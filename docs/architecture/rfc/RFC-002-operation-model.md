# RFC-002 — Operation Model: The Op Log as Source of Truth

**Status:** Accepted
**Date:** 2026-08-06
**Affects:** collaboration-service, document-service, history-service, editor core
**Related:** ADR-001 (event sourcing), RFC-001 (document model)

---

## 1. The Rule

**The UI never mutates the tree. Every change is an operation.**

```
   keystroke / click / paste / drag
              │
              ▼
      compile to Op(s)          ← the only way anything changes
              │
      ┌───────┴────────┐
      ▼                ▼
  apply locally    append to log
  (instant echo)   (durability + sync)
              │
      ┌───────┴────────┬──────────────┬─────────────┐
      ▼                ▼              ▼             ▼
   undo stack      WebSocket      history      diagnostics
   (invert)        (peers)        (projection) (invalidate)
```

Every downstream capability is a consumer of the op stream. That is why this is the highest-leverage decision in the codebase: undo, collaboration, history, and incremental diagnostics all exist *because* ops exist, and none of them can be retrofitted cleanly if the UI mutates state directly.

**The op log is the source of truth.** Block rows in Postgres are a projection. A test must prove the projection can be rebuilt by replay.

---

## 2. The Instruction Set

Think of it as an ISA: a small, closed, versioned set. Every variant must satisfy §3.

```rust
enum Op {
    // ── text within a block ─────────────────────────────────────
    InsertText  { block: BlockId, at: Anchor, text: String },
    DeleteText  { block: BlockId, range: AnchorRange, /* removed */ text: String },
    SetMark     { block: BlockId, range: AnchorRange, mark: Mark, on: bool },

    // ── block structure ─────────────────────────────────────────
    InsertBlock { id: BlockId, parent: BlockId, after: Option<BlockId>,
                  kind: BlockKind, content: Content },
    DeleteBlock { id: BlockId, /* full subtree for inversion */ tombstone: BlockSubtree },
    MoveBlock   { id: BlockId, from: Location, to: Location },
    SetBlockKind{ id: BlockId, from: BlockKind, to: BlockKind },

    // ── page level ──────────────────────────────────────────────
    SetTitle    { page: PageId, from: String, to: String },
}
```

Two things to notice, both consequences of §3:

- `DeleteText` carries the **removed text**, and `DeleteBlock` carries the **entire subtree**. Deletes are the expensive ops precisely because they must be invertible.
- `MoveBlock` and `SetBlockKind` carry `from` as well as `to`. An op that only records its destination is not invertible.

Positions are **anchors**, never integer offsets — offsets are invalidated by concurrent remote edits. Same mechanism as RFC-001 §2's marks.

---

## 2.1 The ISA arrives in two tiers

**Phase 1 ships block-granular ops. Phase 3 adds character-granular ops.** The split is forced, not
stylistic: `InsertText { at: Anchor }` needs a rope to resolve the anchor, and the rope is Phase 3.

| Tier | Ops | Needs | Phase |
|---|---|---|---|
| **Block-granular** | `InsertBlock` · `RemoveBlock` · `MoveBlock` · `SetBlockKind` · **`SetBlockContent { block, spans, prev_spans }`** | Nothing beyond the block tree. **No rope, no anchors** | **1** |
| **Character-granular** | `InsertText` · `DeleteText` · `SetMark` | Rope + anchors (RFC-001 §9) | **3** |

`SetBlockContent` is what makes Phase 1 work: the browser holds the block tree, edits locally, and
sends the whole block's spans on debounce. Coarse, but **an op like any other** — it carries
`prev_spans`, so it is invertible, and it passes `can_apply` like everything else.

**Both tiers are the same log.** `encoding_version` (§4) is what lets Phase 3 add op kinds without
rewriting history, and replay must decode every tier ever written, forever.

> **Phase 1's editor is single-user.** Two people editing one page is Phase 3 — that is what the
> rope and the CRDT are *for*. Until then a page has one writer and the op log is a plain sequence.

---

## 3. Invertibility Is a Design Constraint, Not a Later Feature

**Every op must define its inverse at the moment it is designed.**

```
   apply(invert(op), apply(op, tree)) == tree        for all op, tree
```

This is a `proptest` law, not a comment. It is stated here — in the phase that designs ops — rather than in the undo phase, because discovering it late means revisiting every variant.

| Op | Inverse |
|---|---|
| `InsertText` | `DeleteText` over the inserted range |
| `DeleteText` | `InsertText` with the carried text |
| `SetMark{on: true}` | `SetMark{on: false}` over the same range |
| `InsertBlock` | `DeleteBlock` |
| `DeleteBlock` | `InsertBlock` reconstructing the carried subtree |
| `MoveBlock` | `MoveBlock` with `from`/`to` swapped |
| `SetBlockKind` | `SetBlockKind` with `from`/`to` swapped |
| `SetTitle` | `SetTitle` with `from`/`to` swapped |

**The failure mode this prevents:** shipping `DeleteBlock { id }` because it is all the *forward* path needs, then discovering undo requires the deleted content and having to reach into history to reconstruct it.

### Per-user undo

Naive undo — revert the last N ops globally — is broken in a collaborative session: it reverts other people's work.

Undo is **scoped to the actor's own ops**, filtered from the interleaved log. Inverting your op may require transforming it against ops that landed afterwards; that is the genuinely hard part and it is what makes the CRDT foundation necessary rather than convenient.

**Undo pops the newest `undo_group` belonging to this actor, never the newest op.** One user
gesture is many ops: a paste is dozens, the input rule for `## ` is `SetBlockKind` plus
`DeleteText`, and accepting one assistant proposal is a whole batch. Without grouping, ⌘Z
undoes one twentieth of a paste and the document looks corrupted — the single most common
editor bug, and structural here because the log is append-only.

The group is assigned by **whoever originates the gesture**: the client for paste and input
rules, the assistant for a proposal batch. Never the server, which cannot know where a gesture
began. `undo_group IS NULL` means a group of one, so single-op edits need nothing
(`DATA_MODEL.md` § Two columns that must exist from op #1).

Redo is the same machinery in reverse and inherits grouping for free — which is the point of
putting it in the log rather than in a client-side stack.

---

## 4. The Log Is a Permanent Wire Format

The op log is persisted, replayed for history, and shipped over the network. **You can never break its encoding.**

```rust
struct LoggedOp {
    id:        OpId,          // UUIDv7 — time-ordered, dedup key
    version:   u16,           // encoding version, from op #1
    actor:     ActorId,
    clock:     VectorClock,   // causal ordering
    op:        Op,
}
```

Rules, committed from the first op:

1. **A `version` field exists from day one.** Retrofitting a version onto unversioned records requires guessing which format each row is in.
2. **Explicit enum discriminants**, never derived from declaration order. Reordering variants must not change the encoding.
3. **Additive-only evolution.** New variants get new discriminants; existing ones never change meaning. A field may be added with a default, never removed or repurposed.
4. **Old versions decode forever.** History replay reads ops written by every prior release. Deleting decode support deletes users' history.
5. **`OpId` is the dedup key.** NATS delivers at-least-once, so every consumer dedupes on it.

> Figma and Notion both hit this. "New block types are just new op variants and everything keeps working" is true of *code* and false of *stored logs*.

---

## 5. Authorization: One Chokepoint

```rust
fn can_apply(op: &Op, actor: &Actor) -> bool { true }   // today
```

Every op passes through this. It returns `true` at the current single-tenant scope.

It exists now for two reasons. **Extensibility:** workspaces and RBAC later become "make this function actually check something," rather than threading authorization through every mutation path retroactively. **Security:** one auditable place, versus checks scattered across handlers where you can never prove coverage.

---

## 6. Durability: WAL Before Acknowledgement

An op is acknowledged to the client only after it is durable locally — not after Postgres confirms.

```
   Record framing:  [4-byte len][op bytes][4-byte crc32]
```

- `O_APPEND` + `sync_data()` — measure against `sync_all()`, which also flushes inode metadata
- **CRC32 per record** detects torn writes: a partial record left by a `SIGKILL`
- **Recovery skips torn records and replays all acknowledged ops.** Test it by actually sending `SIGKILL` mid-write
- Segments rotate at 64 MB; deleted once Postgres confirms durability

**This is a recovering parser.** A torn tail is *expected* after a crash, so the reader resynchronises to the next valid record boundary rather than failing. Contrast the gRPC wire protocol, where a malformed frame means a bug or an attack and the correct response is to reject and close. Same byte-level framing, opposite correct answer — knowing which a given decoder needs is the skill.

---

## 7. Batching and Back-Pressure

15k concurrent editors at 2 keystrokes/second is 30k ops/second. Batched ~20:1 that is ~1.5k Postgres writes/second, which one primary handles comfortably. **Batching is what makes the write volume survivable** — not a micro-optimisation.

- `crossbeam::queue::ArrayQueue` — bounded lock-free ring buffer, ops accumulate between flushes
- The flush is a **hand-written `Stream`**: implement `poll_next`, handle `Pin<&mut Self>`, store and wake the `Waker` yourself (ADR-002)
- Bounded means back-pressure is real: a full queue must slow producers, not drop ops

---

## 8. Wire Format and Zero-Copy Fan-Out

Ops cross three boundaries: client ↔ gateway (WebSocket), gateway ↔ doc-actor, and doc-actor → history. JSON on any of them is a hot-path allocation per hop.

### Framing

```
   [4-byte length][op bytes][4-byte crc32]
```

The same framing as the WAL (§6), deliberately — one encoder, one decoder, one set of tests.

### Zero-copy decode with `rkyv`

`rkyv` deserialises by **casting into the buffer** rather than allocating a new structure. For an op that is forwarded and then discarded, that is the difference between one allocation and none.

- Validate before access (`rkyv`'s `CheckBytes`) — an unvalidated archive read from the network is unsound, and this is a boundary an attacker controls
- Owned conversion only where the op is retained (the rope, the WAL); forwarding paths read the archive in place
- Benchmark against `bincode`. Zero-copy wins on read, and `rkyv`'s stricter layout rules cost flexibility — measure before committing

### `Bytes`, not `Vec<u8>`, for fan-out

A doc-actor relaying one op to N subscribers must not clone the payload N times.

```
   BAD                              GOOD
   ───                              ────
   for sub in subs {                let buf = Bytes::from(encoded);   // one alloc
     sub.send(encoded.clone())      for sub in subs {
   }                                  sub.send(buf.clone())           // refcount bump
   // N allocations, N copies      }
                                    // 1 allocation, N atomic increments
```

`Bytes::clone` increments an `AtomicUsize` and shares the underlying buffer. With 20 people on a page that is 1 allocation instead of 20, per keystroke.

This is also where the atomics work in §7 pays off twice: `Bytes` is a refcounted buffer over `AtomicUsize`, so reading its implementation is a short, useful exercise in exactly the ordering questions the op counter raises.

---

## 9. Reconciliation: Merkle Diffing, Not Full Replay

Two replicas that diverged — a client offline for a day, or a doc-actor that lost its lease and rejoined — must converge. Naively that means shipping the whole op log and letting the CRDT sort it out. For a page with 100k ops that is unacceptable on a reconnect.

**Build a Merkle tree over the op log** so the divergence point is found in `O(log n)` comparisons instead of `O(n)`.

```
                    root hash
                   /         \
              h(0..512)      h(512..1024)
              /      \        /       \
        h(0..256) h(256..512) …        …
           │
        leaves = hash(OpId) over a fixed-size chunk

   Exchange root hashes. Equal ⇒ converged, one round trip, no data.
   Unequal ⇒ descend only the differing subtree.
```

- Leaves are chunks of the op sequence, not individual ops — a per-op leaf makes the tree as large as the log
- Chunk boundaries must be **content-defined or index-defined, not time-defined**, or two replicas chunk differently and every comparison fails
- The result is a set of chunk ranges to exchange; the CRDT merge itself is unchanged
- Automerge and Dynamo-style stores both do this. It is the standard answer to "how do you sync without shipping everything"

**Vector clocks answer a different question.** They establish *causality between ops* (did A see B?). Merkle diffing establishes *which ops each side is missing*. Both are needed: causality for ordering, Merkle for efficient discovery.

---

## 10. Deduplication: Bloom Filter, Then Redis

At-least-once delivery (§4) means every consumer must recognise an op it has already applied. A full seen-set in memory is unbounded; a Redis round trip per op is a network hop per keystroke.

**Two tiers:**

```
   op arrives
       │
       ▼
   Bloom filter (in-memory, per doc-actor)
       │
       ├── definitely NOT seen  ──▶  apply, insert into filter    (the common case, zero I/O)
       │
       └── MAYBE seen  ──────────▶  Redis SETNX dedup:op:{id}     (rare; the filter's false-positive rate)
                                        ├── new    ⇒ apply
                                        └── exists ⇒ discard
```

A Bloom filter has **no false negatives**, which is the property that makes this safe: "not seen" is always true, so the fast path can never wrongly discard an op. False positives only cost a Redis check.

- Size for the expected ops-per-session and a ~1% false-positive rate; log the observed rate as a metric
- Reset per doc-actor session, not globally — the filter guards a live session, and Redis with a TTL remains the durable guard
- `OpId` is UUIDv7, so hashing is cheap and well distributed

---

## 11. Correctness Tooling

This is the most `unsafe` and most concurrent code in the project, and tests alone validate neither.

- **`loom`** — model-check the `ArrayQueue` usage, the op-sequence CAS loop, and epoch reclamation. Behind `#[cfg(loom)]` in a dedicated CI job. Rule of thumb: if you chose an `Ordering` by reasoning rather than copying, it needs a loom test
- **Miri** — `cargo +nightly miri test` over rope internals using `MaybeUninit`, `#[repr(align(64))]` structs, and epoch reclamation. Keep these in `libs/` with pure unit tests so they stay Miri-reachable
- **`proptest`** — the invertibility law (§3), convergence under arbitrary interleavings, and log round-trip
- **`cargo-fuzz`** — the WAL reader **and the `rkyv` archive validator** over arbitrary bytes: never panic, never accept a corrupt record. The wire decoder is attacker-controlled input, so this is security work
- **`criterion`** — `rkyv` vs `bincode` decode, and `Bytes` vs `Vec<u8>` fan-out at 5/20/100 subscribers. Publish the numbers; the point is that you measured

---

## 12. Open Questions

1. **Op granularity** — one op per keystroke, or coalesce a typing run into one `InsertText`? Affects log size, undo granularity, and history scrubber resolution.
2. **Undo transformation** — full operational transformation against subsequent ops, or the simpler "undo only if no causally-dependent op followed"? The latter is much easier and occasionally surprises the user.
3. **Tombstone GC** — deleted CRDT elements must be retained for merge correctness until all peers acknowledge. When is it safe to collect?
4. **Vector clock size** — grows with actor count. Prune departed actors, or accept growth at this scope?
5. **Merkle chunk size** — larger chunks mean a smaller tree and coarser diffs (more redundant data exchanged); smaller chunks invert both. Where is the knee?
6. **Does the client also maintain a Merkle tree**, or only servers? A client-side tree makes offline reconnect cheap and adds real complexity to the WASM core.

---

## Resources

| Resource | For |
|---|---|
| [Rust Atomics and Locks — Mara Bos](https://marabos.nl/atomics/) | **Read before §7.** Memory ordering |
| [Crust of Rust — Jon Gjengset](https://www.youtube.com/playlist?list=PLqbS7AVVErFiWDOAVrPt7aYmnuuOLYvOa) | Hand-written `Future`, `Pin`, atomics |
| [Event Sourcing — Fowler](https://martinfowler.com/eaaDev/EventSourcing.html) | Why the log is truth and rows are a projection |
| [Undo Support in Cooperative Work (Prakash & Knister, 1994)](https://dl.acm.org/doi/10.1145/193233.193247) | Why collaborative undo differs fundamentally from single-user undo |
| [Stripe on idempotency keys](https://stripe.com/blog/idempotency) | §4's dedup contract |
| [rkyv docs](https://rkyv.org/) | Zero-copy deserialisation, and the validation requirement |
| [`bytes` crate](https://docs.rs/bytes/) | Read the `Bytes` source — refcounting over `AtomicUsize` |
| [Automerge sync protocol](https://automerge.org/docs/repositories/synchronization/) | Merkle-based reconciliation in a real CRDT library |
| [Dynamo paper (2007), §4.7](https://www.allthingsdistributed.com/files/amazon-dynamo-sosp2007.pdf) | Merkle trees for anti-entropy between replicas |
