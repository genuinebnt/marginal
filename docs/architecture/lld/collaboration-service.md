# LLD — `collaboration-service`

**Owns:** the **`collab`** schema — **`collab.ops`** (append-only, the source of truth) and **`collab.outbox`** (ADR-003). Also the live editing session: one rope per open page, the local WAL, and the op fan-out. It **never writes another service's tables** — `document-service` materialises `blocks` by replaying the op events this service publishes.
**Transport:** WebSocket in (proxied by the gateway), gRPC out to `diagnostics-service`. HTTP for probes only.
**Depends on:** PostgreSQL 18 — its own instance locally and in Tier S, its own schema and login role in the resident deployment (ADR-010 §3) — Redis (presence, instance registry, page ownership lease), a local filesystem for the WAL. NATS on flush via its outbox. **No dependency on `document-service`'s database** — that isolation is the point (ADR-003).
**Related:** `RFC-001` (document model, **§9 anchors**) · `RFC-002` (op ISA, WAL, batching, dedup) · `ARCHITECTURE.md` §4 (live-editing flow) · `DATA_MODEL.md` §6 (Redis keys) · `docs/learning/01-track1-mvp.md` § Phase 3

**The hardest service in the project, and the only stateful one.** Rope, sequence CRDT, WAL,
lock-free batching, a hand-written `Future`, `unsafe` with a written contract, vector clocks, and
zero-copy fan-out. Thirteen of the rare concepts in `ROADMAP.md` § Rust, DSA & Concepts Map live here.

> **Read this whole document before writing anything.** Every other service can be built
> bottom-up from its LLD one slice at a time. This one has a load-bearing decision — the anchor
> representation (RFC-001 §9) — that appears in the op payloads, and op payloads are append-only
> forever. Getting it wrong is not a refactor.

---

## 1. Scope — what is hand-written here

| Copy from `document-service` | Designed for this service |
|---|---|
| Startup path, copied from `document-service` | `crates/document-core` — rope, anchors, marks, block tree (`wasm32`-clean) |
| `blocks` / `pages` DDL (`document-service`'s database) | `session/` — the doc-actor, one per open page |
| | `ops/` — `collab.ops`, the log **this service owns**, plus `collab.outbox` |
| `AppError` shape | `wal/` — framing, `sync_data`, recovering reader |
| Probe router | `transport/` — WebSocket, frame decode, fan-out |
| | `ownership/` — lease, fencing token, handoff |
| | `flush/` — `ArrayQueue`, the hand-written `Stream`, rope → spans |

**`crates/document-core` is the centre of gravity, and it has its own LLD** —
[`document-core.md`](document-core.md) covers the crate as a whole, including the front end that Phase 1
builds. §3–§5 below remain the authority for the rope, anchors, and ops; everything else about the
crate lives there.

It is `wasm32`-clean and infrastructure-free by rule (`CLAUDE.md`), which is what lets the same rope
run in the browser and keeps it Miri-reachable and fuzzable. **Nothing in `crates/document-core` may touch
tokio, sqlx, or the filesystem.**

---

## 2. Module map

```
crates/document-core/                        ← wasm32-clean. No tokio, no sqlx, no fs. CI-enforced
├── anchor.rs         Anchor, AnchorRange, ItemId, Bias, Resolved            RFC-001 §9
├── rope/
│   ├── mod.rs        Rope — insert, delete, slice, resolve
│   ├── node.rs       B-tree leaves. THE unsafe module, if any
│   └── summary.rs    per-subtree totals: bytes, chars, items, tombstones
├── marks.rs          mark intervals over anchors; span coalescing
├── ops.rs            Op enum, apply, invert                                 RFC-002 §2
└── tree.rs           block tree, projection to/from `blocks` content

crates/collaboration-service/src/
├── (startup path, copied)
├── session/
│   ├── mod.rs        SessionRegistry — page_id → actor handle
│   ├── actor.rs      THE doc-actor. Owns one Rope. Single-threaded by construction
│   ├── handle.rs     the mpsc sender other tasks hold
│   └── presence.rs   Redis presence hash, cursor broadcast
├── ownership/
│   └── mod.rs        lease acquire/renew/release, fencing token
├── wal/
│   ├── mod.rs        trait Wal + FileWal (same file)
│   ├── writer.rs     [len][payload][crc32], O_APPEND, sync_data, rotation
│   └── recovery.rs   RECOVERING reader — skips a torn tail
├── ops/
│   ├── mod.rs        trait OpLog + PostgresOpLog (same file) — collab.ops
│   └── outbox.rs     same-transaction publish; its own poller
├── flush/
│   ├── mod.rs        ArrayQueue + the batch policy
│   └── stream.rs     hand-written Stream: poll_next, Pin, Waker
├── transport/
│   ├── ws.rs         upgrade, per-connection read/write halves
│   └── frame.rs      rkyv decode with validation; Bytes fan-out
└── undo/             Phase 5 lands here — §7
```

**Why an actor and not `Arc<RwLock<Rope>>`.** One page has exactly one owner
(`ARCHITECTURE.md` §4), so the rope is never contended — it is *owned*. An actor makes that a
structural fact rather than a discipline, and it removes the lock from the hot path entirely.
Alice Ryhl's [Actors with Tokio](https://ryhl.io/blog/actors-with-tokio/) is the design.

---

## 3. `crates/document-core/anchor.rs` — the decision everything rests on

Resolved in **RFC-001 §9**: Yjs/Peritext-style item ids, not offset-plus-origin. Read that
section before this one; it carries the reasoning and the rejected alternative.

```rust
/// Identity for one inserted run, assigned once and never reused.
/// The Lamport pair every sequence CRDT uses.
pub struct ItemId { /* replica: ReplicaId, counter: u32 */ }

/// Which side of the item the anchor binds to — decides whether text typed at
/// the boundary lands inside or outside the anchored range.
pub enum Bias { Before, After }

pub struct Anchor      { /* ItemId + Bias */ }
pub struct AnchorRange { /* start, end */ }

/// Three-state on purpose. `Detached` is a NORMAL outcome — the anchored text
/// was deleted — which is exactly why this is not `Option<usize>`.
pub enum Resolved {
    At(usize),                        // byte offset in the current rope
    Detached { nearest_live: usize }, // render a comment beside where it was
    Unknown,                          // id absent from this document: bad client
}

impl Rope { pub fn resolve(&self, a: Anchor) -> Resolved; }
```

**Three consumers, ten phases apart:** marks (3), diagnostic spans (4), comments (14).
`Detached` exists for the third one. Build it now or retrofit a type that is baked into every
historical op.

Fields stay private. **Nothing may do arithmetic on an anchor** — integer offsets are precisely
the bug this design prevents.

---

## 4. `crates/document-core/rope/` — the text structure

A B-tree of UTF-8 chunks with per-subtree summaries. The summary is what makes
`resolve(anchor) -> byte offset` an O(log n) descent rather than a scan.

```rust
/// Per-subtree totals, combined on the way up. Adding a dimension here is how
/// a new O(log n) query gets added without touching the tree code.
pub struct Summary {
    bytes: usize,       // UTF-8 length — marks are byte ranges
    chars: usize,       // grapheme-aware cursor movement needs this separately
    items: usize,       // live items, for anchor → offset
    tombstones: usize,  // deleted-but-retained, for GC pressure
}
```

**Read [Zed's Rope & SumTree post](https://zed.dev/blog/zed-decoded-rope-sumtree) before designing
this.** The summary/dimension abstraction is the part that matters and it is easy to under-design.

### On `unsafe`

`node.rs` is the only file that may contain it, and only if a benchmark justifies it.
`MaybeUninit<[Node; N]>` to avoid zeroing a leaf array is the candidate. The contract
(`agents.md`, *Rust for Rustaceans* Ch. Unsafe):

1. A type invariant stated on the struct.
2. `// SAFETY:` on every block, naming which invariant makes it sound.
3. A public API where misuse is impossible.
4. Miri in CI, and `cargo-fuzz` on the public surface.

**Start safe.** `Vec<Node>` first, benchmark with `criterion`, then decide. `unsafe` without a
measurement is decoration.

---

## 5. `session/actor.rs` — the doc-actor

One task, one page, one rope. Everything else sends it messages.

```rust
enum Message {
    Join    { actor: ActorId, kind: ActorKind, reply: oneshot::Sender<Snapshot> },
    Apply   { op: Op<Unchecked>, reply: oneshot::Sender<Result<OpId, ApplyError>> },
    Leave   { actor: ActorId },
    Flush,                       // from the timer or a full queue
    Release { fence: FenceToken, reply: oneshot::Sender<()> },  // lease lost — §6
}
```

### The apply path, in order

```
   1. can_apply(op, actor)          ← ONE authorization chokepoint (RFC-002 §5)
   2. dedup: Bloom filter → Redis   ← RFC-002 §10; no false negatives is what makes it safe
   3. apply to rope, assign OpId + vector clock
   4. WAL append + sync_data        ← durability
   5. ack the originating client    ← ONLY here. Not before 4, not after 6
   6. broadcast to other clients    ← Bytes, one allocation, N refcount bumps
   7. push to ArrayQueue            ← batched flush to collab.ops + collab.outbox, ~20:1
   8. notify diagnostics-service    ← gRPC server stream, changed blocks only
```

**Step 5's position is the design.** Acknowledging before the WAL sync is a lie about durability;
acknowledging after Postgres pays database latency per keystroke. `ARCHITECTURE.md` §4 fixes it
between them.

### `can_apply` is the only authorization point

Every op passes through it — user, assistant, and plugin alike (`CLAUDE.md`). Phase 13 makes it a
typestate: `Op<Unchecked>` → `Op<Authorized>`, so *forgetting* to check is a compile error rather
than a review finding. **Design the signature that way now** even though the check is trivial in
Phase 3.

---

## 6. `ownership/` — the lease

Redis holds `collab:page:{page_id}` (`DATA_MODEL.md` §6). Two rules from
`ROADMAP.md` § *Ownership must be a lease, not a record*:

| Rule | Why |
|---|---|
| **A lease, not a record** | A recorded owner that pauses and resumes causes split-brain. A lease with a TTL expires on its own |
| **A fencing token, not a heartbeat** | The token is a monotonically increasing number issued with the lease. Every write carries it; a stale owner's writes are rejected by number, not by liveness guess |

**A Redis TTL is not a lock.** Read Kleppmann's [How to do distributed locking](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html)
before implementing this. The fencing token is what makes it safe *despite* Redis not being
linearizable — do not cargo-cult it, understand why.

Losing the lease mid-session is not an error path to bolt on later: the actor must flush what it
can, close sockets with a reconnect hint, and drop the rope. `Message::Release` carries the fence
so a late release from a superseded owner is ignored.

---

## 7. `undo/` — Phase 5 lands here

Not a separate service and not a separate LLD. Undo is `apply(invert(op))` plus a stack, and it is
small **only if invertibility was designed in at Phase 3** (RFC-002 §3).

| Contract | Where it comes from |
|---|---|
| Undo pops the newest **`undo_group`** for this actor, never the newest op | `DATA_MODEL.md` § Two columns that must exist from op #1 |
| The group is assigned by whoever originates the gesture — client for paste and input rules, assistant for a proposal batch. **Never the server** | RFC-002 §3 |
| Inverting your op may require transforming it against ops that landed after it | RFC-002 §3 — the genuinely hard part |
| A Treiber stack is the exercise; a `Mutex<Vec<_>>` is the correct default until a benchmark says otherwise | `ROADMAP.md` § Concurrency |

**If Phase 5 feels large, something in Phase 3 was wrong.** That is the diagnostic value of
putting undo here rather than in its own service.

---

## 8. Error mapping

WebSocket, not gRPC, so the vocabulary differs: most failures are a close frame or an in-band
error message rather than a status code.

| Condition | Response | Client behaviour | Logged |
|---|---|---|---|
| `can_apply` denies | in-band `OpRejected { op_id, reason }` | keep the session; roll back the optimistic local apply | `info` |
| Duplicate `OpId` | in-band ack, **no re-apply** | none — idempotent by design | `debug` |
| Malformed frame | close `1002` (protocol error) | reconnect with backoff **and jitter** | `warn` |
| Frame exceeds limit | close `1009` | reconnect | `warn` |
| Lease lost / page moved | close `4001` + `{ retry_after, hint }` | reconnect; the gateway rehashes to the new owner | `info` |
| WAL write fails | **do not ack**; close `1011` | reconnect and resync from the server snapshot | `error` |
| Rope invariant violated | `panic!` → caught by `CatchPanicLayer`, session dies | reconnect and resync | `error` |

**A rope invariant violation must not be recovered from.** It means the in-memory state is
already wrong, and continuing risks flushing corruption to `docs.blocks`. Fail the session; the
op log is the source of truth and a resync is cheap.

**A malformed frame is rejected, a torn WAL record is repaired.** Same byte framing, opposite
correct answer (RFC-002 §6) — the wire is attacker-controlled, the WAL tail is your own crash.

---

## 9. Algorithms — named, not written

The densest §9 in the project. Each row is a `ROADMAP.md` § Concepts Map item.

### The document model

| Algorithm | Invariant that must hold | Reference |
|---|---|---|
| **Rope insert / delete** | O(log n) at an arbitrary position; the tree stays balanced; every leaf is a valid UTF-8 boundary — **never split a multi-byte character** | [Zed SumTree](https://zed.dev/blog/zed-decoded-rope-sumtree) · [`ropey`](https://github.com/cessen/ropey) |
| **Anchor resolution** | Stable under any concurrent remote insert or delete; `Detached` when the item is a tombstone; two anchors never cross | RFC-001 §9 |
| **Bias at a boundary** | Text typed exactly at a mark edge lands inside iff the anchor's `Bias` says so, deterministically on every replica | Peritext |
| **Span coalescing** | Adjacent identical mark sets merge in one left-to-right pass; the result is **idempotent** — coalescing twice equals coalescing once | RFC-001 §2 |
| **Rope → spans projection** | Replaying the op log reproduces exactly the flushed `blocks.content`. This is `DATA_MODEL.md` §1's central rule, as a test | RFC-001 §2 |
| **Op invertibility** | `apply(invert(op), apply(op, doc)) == doc` for **every** op, proptested, including ops that carry deleted subtrees | RFC-002 §3 · `ui-mockups/trace.html` |
| **CRDT convergence** | Any two replicas that have seen the same set of ops, in any order, hold identical documents. **Proptest with a random interleaving generator** | Peritext · [CRDTs go brrr](https://josephg.com/blog/crdts-go-brrr/) |
| **Tombstone GC** | An item is reclaimable only when every replica has seen its deletion; reclaiming early breaks anchor resolution for a lagging peer | RFC-002 |

### Durability and ordering

| Algorithm | Invariant that must hold | Reference |
|---|---|---|
| **WAL framing + recovery** | Every acknowledged op is replayed after a crash; a torn tail is skipped, not fatal. **Test by sending real `SIGKILL` mid-write** | RFC-002 §6 |
| **`flush()` vs `sync_data()`** | The ack happens after the data is on the device, not in a buffer. `sync_data` vs `sync_all` is a measured choice, not a guess | *Database Internals* Ch. Recovery |
| **CRC32 per record** | A partial record is always detected. Benchmark `crc32fast`'s SSE4.2/CLMUL path against a table implementation | RFC-002 §6 |
| **Vector clocks** | Causal order is preserved across instances; concurrent ops are *detected* as concurrent — which a Lamport timestamp alone cannot do | `ROADMAP.md` § Distributed systems |
| **Bloom filter dedup** | **No false negatives** — that is what makes it safe to skip the Redis round trip on a miss | RFC-002 §10 |
| **Merkle anti-entropy** | Finds the divergence point in O(log n) instead of shipping the whole log | RFC-002 §9 |

### Concurrency and throughput

| Algorithm | Invariant that must hold | Reference |
|---|---|---|
| **`ArrayQueue` batching** | Bounded, so back-pressure is real: a full queue **slows producers and never drops an op** | RFC-002 §7 |
| **Hand-written `Stream` flush** | `poll_next` returns `Pending` only after the `Waker` is stored; no lost wakeups; correct under `Pin<&mut Self>` | *Rust for Rustaceans* Ch. Async |
| **Atomics + memory ordering** | The op-sequence counter is monotonic under contention, and every `Ordering` weaker than `SeqCst` is **justified in a comment and loom-tested** | *Atomics and Locks* Ch. 3 |
| **Epoch-based reclamation** | A shared op-log node is freed only after every reader that could observe it has left its epoch | KAIST cs431 |
| **Cancellation safety** | A `select!` branch dropped mid-`await` never loses an op between "received" and "applied" | [tokio `select!` docs](https://docs.rs/tokio/latest/tokio/macro.select.html#cancellation-safety) |
| **`Bytes` fan-out** | Broadcasting to N subscribers is one allocation and N atomic increments, never N copies | `ROADMAP.md` § Memory & layout |
| **`rkyv` zero-copy decode** | **Validation before access.** The wire is attacker-controlled, so an unvalidated cast is a memory-safety hole | RFC-002 §8 |

---

## 10. Test map

```
crates/document-core/tests/                 ← no infrastructure. Pure cargo test, wasm32-clean
├── rope.rs                     insert/delete/slice, UTF-8 boundaries, huge inputs
├── anchor.rs                   the eight laws from RFC-001 §9
├── marks.rs                    coalescing idempotence, bias at boundaries
├── invertibility.rs            proptest: apply∘invert == identity, every op kind
└── convergence.rs              proptest: random interleavings converge

crates/collaboration-service/tests/
├── wal.rs                      round trip, torn tail, SIGKILL recovery, rotation
├── session.rs                  join/apply/leave, ack ordering, rejection path
├── ownership.rs                lease acquire/renew/expire, fencing rejects a stale write
├── flush.rs                    batching, back-pressure under a full queue
├── cancellation.rs             drop a read mid-await; assert no op is lost
└── partition.rs                turmoil: partition, delay, drop — split-brain must not occur
```

### The four that carry the phase

| Test | What it proves |
|---|---|
| `invertibility.rs` | Undo (Phase 5) is a consequence rather than a project. `trace.html` already runs this law live |
| `convergence.rs` | The CRDT claim. Generate two op sequences, apply in both orders, assert identical documents |
| `wal.rs` — the `SIGKILL` case | Durability is real. Spawn a child, kill it mid-write, recover, assert every acked op is present |
| `partition.rs` — turmoil | The CP claim. Partition an instance from Redis but **not** from its clients: fencing tokens must prevent two owners |

**`loom` for the atomics, Miri for any `unsafe`, `cargo-fuzz` for the frame decoder.** Tests alone
do not cover this phase (`CLAUDE.md`).

---

## 11. Build order

Strictly bottom-up. Steps 1–5 need no database, no network, and no Docker — they are pure
`cargo test`, and they are most of the difficulty.

1. **`crates/document-core/anchor.rs`** — types and the eight laws as failing tests. No rope yet; a `Vec<char>` stub is enough to make the laws meaningful.
2. **`crates/document-core/rope/`** — safe implementation, `Summary` with bytes and chars. Activate `rope.rs`, `anchor.rs`.
3. **`crates/document-core/marks.rs`** — intervals over anchors, coalescing. Activate `marks.rs`.
4. **`crates/document-core/ops.rs`** — the `Op` enum, `apply`, `invert`. Activate `invertibility.rs`. **This is the gate: do not proceed until the law holds under proptest.**
5. **Convergence** — vector clocks and the merge rule. Activate `convergence.rs`.
6. **`wal/`** — framing, `sync_data`, the recovering reader. Activate `wal.rs` including `SIGKILL`.
7. **`session/actor.rs`** — the actor with an in-memory rope, no persistence. Activate `session.rs`.
8. **`transport/`** — WebSocket, `rkyv` decode with validation, `Bytes` fan-out. Fuzz the decoder.
9. **`flush/`** — `ArrayQueue`, then the hand-written `Stream`. Activate `flush.rs`.
10. **`ownership/`** — lease and fencing token. Activate `ownership.rs`.
11. **`turmoil`** — partition tests. Activate `partition.rs`.
12. **`loom`** on the counter, **Miri** on anything `unsafe`, **`tokio-console`** on the flush task.

**Step 4 is the checkpoint.** If invertibility does not hold under proptest, everything after it
inherits the defect — and Phases 5 and 6 are built on it.

### 11.1 The cloud increment for this phase

**GKE arrives here** (`CLOUD_ROADMAP.md` §2). Cloud Run cannot host this service: it is stateful,
holds long-lived WebSocket connections, and needs stable identity for consistent-hash routing.

| Terraform resource | Why this phase forces it |
|---|---|
| `google_container_cluster` + node pool | First workload Cloud Run cannot serve |
| `StatefulSet` (not `Deployment`) | Stable network identity per instance for the routing ring |
| `google_redis_instance` (if Phase 2 did not) | Presence, instance registry, ownership lease |
| PersistentVolume per pod | The WAL. **Ephemeral is acceptable** — correctness needs it flushed before shutdown, not surviving the pod (`CLOUD_PORTABILITY.md` §6) |
| Gateway API `HTTPRoute` with WebSocket support | The proxy path |
| `terminationGracePeriodSeconds` **>** drain timeout | Otherwise Kubernetes kills the pod mid-flush |
| Pub/Sub topic + subscription (ADR-010 §2) | The outbox gets its first subscriber |

---

## 12. Implementation notes — the things that will bite

### `select!` will lose an op, and the test for it is not obvious

The canonical shape is a `select!` over "socket read" and "shutdown". If the read branch is
cancelled after the frame arrives but before the op is applied, **the op is gone and the client
already believes it is in flight.** `tokio::sync::mpsc::Receiver::recv` is cancel-safe;
`AsyncReadExt::read` composed with your own decode loop generally is **not**.

Read the [cancellation-safety section](https://docs.rs/tokio/latest/tokio/macro.select.html#cancellation-safety)
and [sunshowers on cancelling async Rust](https://sunshowers.io/posts/cancelling-async-rust/), then
write `cancellation.rs` **before** the transport, not after.

### `flush()` is not durability

`BufWriter::flush` moves bytes from your buffer to the kernel. A power cut still loses them.
`sync_data()` is the syscall that matters, and it costs milliseconds — which is exactly why the
ack is placed after it and Postgres is not. Measure `sync_data` against `sync_all`; the latter
also flushes inode metadata and is usually unnecessary for an append-only file.

### Byte offsets and char offsets diverge, and the sample data must prove it

Marks are **byte** ranges (RFC-001 §2). Cursor movement is by **grapheme cluster**. These are
three different indices into the same text and conflating any two of them breaks on the first
emoji or combining character.

`ui-mockups/compiler.html`'s default text contains an em dash and `café` on purpose. **Your test
fixtures must too** — an all-ASCII fixture passes every version of this bug.

### Tombstones only grow, and the GC is a distributed problem

Deleted items are retained so anchors resolve (§3). At single-user scale this is invisible; over a
long-lived document it is unbounded growth. The reclamation condition is *every replica has seen
the deletion*, which needs the vector clock — so **GC is not a local cleanup task**, and building
it as one is a correctness bug that only appears when a peer is offline.

### A full `ArrayQueue` must slow producers, never drop

Bounded is the point. The tempting failure mode — drop the op when the queue is full — silently
violates durability for an op you already acknowledged. Back-pressure has to reach the WebSocket
read loop, which means the apply path must be able to `await`.

### `rkyv` without validation is a memory-safety hole

Zero-copy decode casts into the buffer. The buffer came from the network. Use the validating API
(`check_archived_root`) on **every** inbound frame — the performance you are protecting is
worthless against an attacker-supplied pointer. Fuzz `frame.rs` from day one.

### `#[repr(align(64))]` on per-session counters, or two cores serialise

Two atomics in one cache line make unrelated sessions contend on the same line — false sharing.
It does not show up on one core, or in a single-session benchmark. `ROADMAP.md` § Memory & layout
has the row; *Atomics and Locks* Ch. 7 has the explanation.

### `SeqCst` everywhere is not "safe by default", it is untested

It is *correct* and slow, which is a fine starting point. The failure is shipping a weaker
`Ordering` because a benchmark improved, without a `loom` test. **An `Ordering` that has not been
loom-tested is a guess** — write the loom test in the same commit that weakens the ordering.

### The WAL segment cannot be deleted when Postgres acks — only when the *flush* commits

Segments rotate at 64 MB and are deleted once Postgres confirms durability (RFC-002 §6). The trap
is deleting on the outbox *publish* rather than on the transaction *commit*: the flush and the
outbox row share one transaction, so a rollback after a publish would drop ops still needed for
recovery.

### A lost lease during a flush is the ugly case

The actor may be mid-flush when the lease expires. Writing after that risks two owners writing the
same page. The fencing token must be checked **inside** the flush transaction — a check before
`BEGIN` is a TOCTOU window. Carry the token into the SQL as a predicate, not as an assertion in
Rust.

### `crates/document-core` will acquire an infrastructure dependency by accident

Someone adds `tokio::time::Instant` for a timestamp, or `rand` without `getrandom`'s `js` feature,
and the browser build dies months later. The CI gate exists for this
(`ROADMAP.md` § The wasm32 rule needs a gate) — **add it before writing `crates/document-core`, not after.**

```
cargo check -p domain -p doc -p diagnostics --target wasm32-unknown-unknown
```

### Reconnect storms after a deploy

Every client of every page reconnects at once when a pod rolls. Exponential backoff **with
jitter** — without jitter the retries stay synchronised and the thundering herd repeats at each
interval ([AWS Builders' Library](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/)).
The close frame's `retry_after` hint should be randomised server-side too, since a client library
may honour it literally.