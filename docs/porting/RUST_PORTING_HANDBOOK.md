# The Rust Porting Handbook

> **What this is.** A module-by-module scaffold for hand-porting Marginal's
> Go backend to Rust. It is not a translation of the Go code and deliberately
> contains **no finished Rust implementations** — types, signatures,
> invariants, algorithms in pseudocode, and the test list that proves each
> one. `.agents/agents.md` §2 is the format; this document applies it to
> every module in the system at once so the shape of the whole port is
> visible before the first `cargo new`.
>
> **How to read it.** `docs/porting/PORTING_GUIDE.md` is the orientation
> layer — read that first, once. This is the reference you keep open. Each
> part is self-contained enough to be worked in isolation, and **Part 30 is
> the one to follow literally**: it gives the stage-by-stage order with a
> checkpoint after each, chosen so that every stage is verifiable on its own
> and nothing is built before what it depends on.
>
> **The five parts to read before writing any code:** 0 (what moves), 1
> (the workspace), 22 (authorization — where every security bug in this
> system has been), 24 (the bug catalogue — tests that are proven reachable)
> and 30 (the order). Everything else can be read when you reach it.
>
> **Three claims this handbook makes that are worth arguing with, because
> they shape everything else:**
>
> 1. **The frontend never moves.** TypeScript, HTML and CSS stay exactly as
>    they are, and they are the harness the Rust backend is judged against.
>    A screen that renders differently means the port is wrong.
> 2. **The algorithm is always in Rust, never re-implemented in
>    TypeScript** — server-side, or compiled to wasm when it must run
>    against live client state. The view layer only draws what was computed.
> 3. **Where the port can turn a discipline into a type, it should.** The
>    worked example is `Scope` in Part 22.4: a rule that was documented,
>    reviewed, and still forgotten in four places becomes a parameter that
>    cannot be defaulted, and the compiler enumerates the call sites. That
>    is the strongest reason to do this in Rust at all.
>
> **What it is not.** It is not a Rust tutorial, and it does not re-explain
> the product. `RFC-001` (document model), `RFC-002` (operation model),
> `RFC-003` (diagnostics), `DATA_MODEL.md` (schemas) and `docs/api/` are the
> language-agnostic specs; where this handbook disagrees with one of them,
> the RFC wins and this document has a bug.

---

## Contents

| Part | Subject | Depends on |
|---|---|---|
| 0 | The shape of the port | — |
| 1 | Crate layout and the workspace | 0 |
| 2 | The data model, in Rust types | 1 |
| 3 | The document block model | 2 |
| 4 | The grammar, in full | 3 |
| 5 | The parser and the paste pipeline | 4 |
| 6 | The operation model | 3 |
| 7 | Anchors, and why offsets die | 6 |
| 8 | `collaboration-service` — the stateful one | 6, 7 |
| 9 | `document-service` — pages, projections, sagas | 2, 6 |
| 10 | Graph algorithms | 1 |
| 11 | Search: FTS, BK-tree, trie | 1 |
| 12 | Semantics: vectors and HNSW | 10 |
| 13 | Diff, history, and the palimpsest | 6 |
| 14 | Diagnostics and the fact DAG | 3, 10 |
| 15 | Async, concurrency, and backpressure | 8 |
| 16 | Errors, and the Rust idiom table | all |
| 17 | The wasm boundary | 3, 10 |
| 18 | Testing strategy and the order of work | all |
| 19 | Microservices: the parts that are not the algorithm | 1 |
| 20 | gRPC, in Rust | 19 |
| 21 | Persistence: sqlx, transactions, projections | 2 |
| 22 | Identity, authorization, and the rule that gets broken quietly | 19, 21 |
| 23 | Security testing: what to actually test | 22 |
| 24 | The bug catalogue: tests this system has already earned | all |
| 25 | Sketches: HyperLogLog, Count–Min, t-digest | 1 |
| 26 | The markdown compiler, and the lexer | 5 |
| 27 | The network simulator: TP1, Merkle, the causal DAG, LSM | 6, 15 |
| 28 | Benchmarking honestly | 17 |
| 29 | Configuration, deployment, and observability | 19 |
| 30 | The order of work, with checkpoints | all |

---

# Part 0 — The shape of the port

## 0.1 What moves and what does not

**Moves to Rust.** Every Go module in `services/`, without exception:
`documentcore`, `graphalgo`, `textdiff`, `semantic`, `outboxpoll`,
`envconfig`, and the five services that use them.

**Never moves.** `web/` — the TypeScript/HTML/CSS frontend. It is the
permanent visual harness the Rust backend is checked against (`ADR-012`), and
`tools/uidiff` is how that check is run. If a screen renders identically
against the Rust backend and against the Go one, the port of whatever feeds
that screen is done. This is the single most valuable property of the whole
arrangement: **you have a working oracle for every feature, all the way
through the UI, for free.**

**Moves, but rewritten rather than translated.** The wasm entrypoints
(`cmd/wasm`, `cmd/graphwasm`, `cmd/diffwasm`, `cmd/triewasm`). Go's
`syscall/js` and Rust's `wasm-bindgen` are different enough that a
line-by-line port is worse than a re-derivation from the same JSON contract.
The contract is what is stable — see Part 17.

## 0.2 The one thing to internalise before starting

**The op log is the source of truth; everything else is a projection.**

- `collab.ops` is append-only. It is never rewritten, never compacted in
  place, never edited by a migration.
- `docs.blocks`, `docs.page_links`, the FTS vectors, the graph, the search
  index and the semantic index are **all derived**. Any of them can be
  dropped and rebuilt by replaying the log.
- Therefore: **a bug in a projection is recoverable; a bug in the log is
  not.** Port the log and its invariants first, with the most paranoid
  testing you are willing to write, and treat the projections as ordinary
  code afterwards.

This is also what makes the port incremental. You can run a Rust
`collaboration-service` against the Go `document-service` (they share no
database and talk only over NATS and gRPC), or the reverse, and the system
still works.

## 0.3 The invariants that hold across every module

These are numbered because later parts refer to them.

**I0.1 — Every op is invertible.** For every op `o` and every document state
`d` where `o` applies, `apply(apply(d, o), invert(o, d)) == d`. Invertibility
is designed in, not discovered during the undo feature. Note that `invert`
takes the state: `DeleteText`'s inverse needs the text it deleted, which the
op itself therefore has to carry.

**I0.2 — Replay reproduces the projection.** Replaying `collab.ops` for a
page from empty must produce exactly the `docs.blocks` rows the incremental
projection produced. This is a property test, not a comment.

**I0.3 — Every op passes one authorization chokepoint.** `can_apply(op,
actor)` is called in exactly one place. Not one place per service — one place.

**I0.4 — Ordering is deterministic and total.** Lamport timestamp, then
actor id as tiebreak. Two peers that have seen the same ops agree on their
order, always, without communication.

**I0.5 — Anchors, never integer offsets, across a network boundary.** An
offset held for a millisecond is an offset a concurrent edit has already
invalidated. See Part 7.

**I0.6 — Determinism where a human reads the output.** Every algorithm whose
result is drawn on screen breaks ties explicitly (by id, lexicographically).
A component index, a reading order or a hue that changes between two runs
over identical input is one nobody can act on.

## 0.4 Before you start

1. **Rust for Rustaceans**, ch. 2 (Types) and ch. 3 (Designing Interfaces) —
   before designing any public API in this system.
2. **Crust of Rust: "Sorting Algorithms"** and **"Iterators"** — not for the
   algorithms, for the shape of a well-typed generic API in Rust.
3. **Database Internals**, ch. 1–3 — for why the storage decisions below are
   what they are, before you disagree with any of them.

---

# Part 1 — Crate layout and the workspace

## 1.1 The mapping

Go's `go.work` with one module per service becomes one Cargo workspace with
one crate per module. The boundaries are already right — they were drawn to
be portable — so this is mechanical.

| Go module | Rust crate | Kind |
|---|---|---|
| `marginal/documentcore` | `marginal-documentcore` | lib, no I/O |
| `marginal/graphalgo` | `marginal-graphalgo` | lib, no I/O |
| `marginal/textdiff` | `marginal-textdiff` | lib, no I/O |
| `marginal/semantic` | `marginal-semantic` | lib, no I/O |
| `marginal/sketch` | `marginal-sketch` | lib, no I/O |
| `marginal/syntax` | `marginal-syntax` | lib, no I/O |
| `marginal/netsim` | `marginal-netsim` | lib, no I/O |
| `marginal/bench` | `marginal-bench` | lib, no I/O |
| `marginal/mdc` | `marginal-mdc` | lib, no I/O |
| `marginal/authverify` | `marginal-authverify` | lib, one HTTP fetch |
| `marginal/outboxpoll` | `marginal-outbox` | lib, needs a `Pool` |
| `marginal/envconfig` | *delete it* | see 1.3 |
| `document-service` | `marginal-document-service` | bin |
| `collaboration-service` | `marginal-collab-service` | bin |
| `auth-service` | `marginal-auth-service` | bin |
| `notification-service` | `marginal-notify-service` | bin |
| `diagnostics-service` | `marginal-diagnostics-service` | bin |
| `api-gateway` | `marginal-gateway` | bin |

**The nine `no I/O` crates are the port's centre of gravity.** They hold
every algorithm, they have no async, no database, no network — and they are
therefore the crates where Rust's type system buys the most and costs the
least. Port them first (Part 18).

## 1.1b The workspace manifest

```toml
# Cargo.toml at the repo root
[workspace]
resolver = "2"
members = ["crates/*"]

[workspace.package]
edition = "2021"
rust-version = "1.79"          # pin it; MSRV drift is a lead's problem
license = "MIT"

# One version per dependency, for the whole workspace. Two crates on two
# versions of `uuid` is a type mismatch that reads like a compiler bug.
[workspace.dependencies]
uuid       = { version = "1", features = ["v7", "serde"] }
serde      = { version = "1", features = ["derive"] }
serde_json = "1"
thiserror  = "2"
anyhow     = "1"
tokio      = { version = "1", features = ["rt-multi-thread", "macros", "signal", "sync", "time"] }
tonic      = "0.12"
prost      = "0.13"
sqlx       = { version = "0.8", features = ["runtime-tokio", "postgres", "uuid", "time", "json", "macros", "migrate"] }
axum       = "0.7"
tracing    = "0.1"
proptest   = "1"
```

Each crate then writes `uuid = { workspace = true }`. This is not tidiness:
it is what stops `documentcore`'s `Uuid` and `document-service`'s `Uuid`
being different types.

**Lints, workspace-wide, from day one:**

```toml
[workspace.lints.rust]
unsafe_code = "forbid"          # nothing here needs it
missing_docs = "warn"

[workspace.lints.clippy]
pedantic = { level = "warn", priority = -1 }
unwrap_used = "deny"            # in library crates especially — see 17.4
expect_used = "warn"
```

`unsafe_code = "forbid"` is worth stating loudly: **this entire system has
no reason to use `unsafe`.** If a profile later says a hot loop needs it,
that is a deliberate, reviewed exception in one crate, not a workspace
default.

## 1.2 Dependency choices, and the argument for each

| Need | Go | Rust | Why this one |
|---|---|---|---|
| Async runtime | goroutines | `tokio` | The only realistic choice for a service that holds long-lived WebSockets. |
| HTTP server | `net/http` + `chi` | `axum` | Tower middleware composes; `axum`'s extractors make the actor-id boundary a type rather than a header read repeated in every handler. |
| Postgres | `pgx/v5` + `sqlc` | `sqlx` | `sqlx::query!` checks SQL against a live schema at **compile time**, which is closer to `sqlc`'s guarantee than an ORM is. Do not reach for `diesel` here: the schema uses LTREE, JSONB, generated columns and `uuidv7()`, and fighting an ORM about those costs more than writing SQL. |
| gRPC | `grpc-go` + `protoc` | `tonic` + `prost` | Same `.proto` files, unchanged. This is one of the few places the port is genuinely free. |
| WebSocket | `coder/websocket` | `tokio-tungstenite` (or `axum::extract::ws`) | Prefer axum's, so the socket lives in the same router as the health probes. |
| NATS | `nats.go` | `async-nats` | Same subjects, same envelope. |
| UUIDv7 | `google/uuid` | `uuid` with `v7` | Keep generating ids **application-side**, not `gen_random_uuid()` — see 2.2. |
| Serialisation | `encoding/json` | `serde_json` | Tagged enums need `#[serde(tag = "...")]`; see 3.2, this is the single most important serde decision in the port. |
| Errors (libs) | `error` values | `thiserror` | |
| Errors (bins) | wrapped `fmt.Errorf` | `anyhow` | See Part 16. |
| Tests | stdlib + `testify` | stdlib + `pretty_assertions` | |
| Property tests | `pgregory.net/rapid` | `proptest` | The Go side already has property tests; they port almost directly. |
| Containers in tests | `testcontainers-go` | `testcontainers-rs` | Never mock Postgres. |
| Fuzzing | `go test -fuzz` | `cargo-fuzz` (libFuzzer) | The Go side fuzzes the mention parser and the lexers; both port directly. |
| Password hashing | `argon2id` | `argon2` | Same PHC string format, so **existing hashes verify unchanged** — the users table needs no migration. |
| JWT / JWKS | hand-rolled + `jose` | `jsonwebtoken` | Verify locally, never an RPC per request. Part 22.1. |
| Benchmarks (CI) | `testing.B` | `criterion` | Proper statistics. The in-browser benchmark needs its own harness (Part 28). |
| wasm | `syscall/js` | `wasm-bindgen` | Rewrite the entrypoints; do not port them. Part 17.2. |
| Time | `time.Time` | `time::OffsetDateTime` | `chrono` also fine; pick one workspace-wide. **`std::time::Instant` panics under wasm** (17.5). |
| Float ordering | ad hoc | `ordered_float` | `f32` is not `Ord`; the HNSW heaps need a total order (12.1b). |

### Crates deliberately NOT used

| Tempting | Why not |
|---|---|
| `diesel` / `sea-orm` | LTREE, JSONB, generated `tsvector`, partial unique indexes, `FOR UPDATE SKIP LOCKED`. Every one is a fight with an ORM and a line of SQL. |
| `rayon` in the algorithm crates | They must compile to wasm, which has no threads (17.5). |
| `async-trait` on internal traits | Only needed where a trait object crosses an await. Prefer generic bounds (Part 16's PORT-NOTE); `tonic` brings its own. |
| `lazy_static` | `std::sync::OnceLock` / `LazyLock`. |
| A DI framework | The composition happens in `main.rs`, explicitly, in about forty lines. Look at any of the Go `cmd/main.go` files: that wiring is the whole "container". |

## 1.3 Delete `envconfig`

`marginal/envconfig` exists because every Go `main.go` had grown its own
copy of "read this env var, or this default, or fail". In Rust that is
`serde` + `envy`, or `figment`, or twenty lines with `std::env::var`. Do not
port a shim whose reason to exist was a gap the target language does not
have. **This is the general rule for the whole port: port the decisions, not
the workarounds.**

## 1.4 What to do about `outboxpoll`

`marginal/outboxpoll` is a `Poller` parameterised by three closures —
`Claim`, `MarkPublished`, `BuildEnvelope` — because Go's generics could not
express "a repo with these three methods over a type I do not know" without
more ceremony than it saved.

Rust can. The port is a trait:

```rust,ignore
// Sketch only. Signatures, not bodies.
trait Outbox {
    type Event: Serialize;
    type Error;
    async fn claim(&self, limit: i32) -> Result<Vec<Self::Event>, Self::Error>;
    async fn mark_published(&self, ids: &[Uuid]) -> Result<(), Self::Error>;
    fn subject(&self, event: &Self::Event) -> String;
}
```

**Invariant O1** — claim and publish share one transaction, and the claim
uses `FOR UPDATE SKIP LOCKED`. Two pollers must never claim the same row, and
a poller that dies mid-publish must leave the row claimable again.

**Invariant O2** — at-least-once, never at-most-once. Consumers dedupe on
`source_event_id` (this is why `notify.notifications` has that column and a
`UNIQUE` on it). Do not "fix" the duplicate by making delivery exactly-once;
you cannot, and the dedup key is the real fix.

**Test list.**
1. `two_pollers_never_claim_the_same_row` — the hardest; needs two real
   connections against a real Postgres, and a barrier.
2. `a_crash_between_claim_and_mark_leaves_the_row_claimable`
3. `redelivery_creates_no_second_notification` (the consumer half of O2)
4. `an_empty_outbox_polls_without_burning_cpu`

**Before:** *Database Internals* ch. 13 (Distributed Transactions) for why
the outbox pattern exists at all; *DDIA* ch. 11 for the log-as-integration
argument.

**After:** Debezium does this with CDC instead of a table, which removes the
double write at the cost of coupling to the WAL format. The spec chose a
table because a self-hosted single-node deployment should not need a CDC
pipeline running beside it.

---

# Part 2 — The data model, in Rust types

`DATA_MODEL.md` is the authority. This part is only about how its columns
become Rust types, and where that mapping has a trap in it.

## 2.1 Database per service, and what that costs

Five services, five Postgres databases, **no cross-database joins**:

| Schema | Owner | Holds |
|---|---|---|
| `docs` | `document-service` | `pages`, `blocks`, `page_links`, `topics`, `page_tags`, `page_deletions`, `reading_positions`, `outbox` |
| `collab` | `collaboration-service` | `ops`, `outbox` |
| `auth` | `auth-service` | `users`, `sessions`, `outbox` |
| `notify` | `notification-service` | `notifications` |
| — | `diagnostics-service` | *nothing*; it is stateless by design |

The cost is real and should be paid consciously: a page's title lives in
`docs.pages` and its ops live in `collab.ops`, so anything wanting both makes
two calls. The benefit is that `collaboration-service` — the only stateful,
connection-scaled service — can be deployed, restarted and scaled without
touching the database the editor's page tree reads from.

## 2.2 Identifiers

**Every id is a UUIDv7, generated application-side.** Not
`gen_random_uuid()`, not a bigserial.

- **v7, not v4**, because it is time-ordered: index locality on insert, and
  `ORDER BY id` is chronological without a second column.
- **Application-side**, because an op's id is assigned *before* it is
  written — it appears in the ack the client is already waiting on, and in
  the WAL entry that precedes the database write. A database-assigned id
  would mean the client cannot be told what its own edit is called until the
  write commits.

```rust,ignore
// Newtypes, not bare Uuid. PageId and BlockId are both UUIDs and are never
// interchangeable; the compiler should be the one that knows that.
struct PageId(Uuid);
struct BlockId(Uuid);
struct OpId(Uuid);
struct ActorId(Uuid);
```

This is the single cheapest win in the whole port. Go could not do it without
a wrapper type per id and hand-written `Scan`/`Value`; in Rust it is a
`#[derive(sqlx::Type)] #[sqlx(transparent)]`.

## 2.3 The hard columns

**`ltree` (`docs.pages.path`).** Materialised ancestry, `a.b.c`. `sqlx` has
no native LTREE type: read it as `String` with an explicit `path::text` cast
in the query, exactly as the Go side already does. Wrap it:

```rust,ignore
struct LTree(String);          // invariant: labels are [A-Za-z0-9_]+, dot-separated
impl LTree { fn parent(&self) -> Option<LTree>; fn depth(&self) -> usize; }
```

**Invariant D2.1** — `path` is a *cache* of ancestry, not an address. Never
construct a request from it. Reparenting rewrites it for the moved page and
every descendant **in one transaction** — a reader must see all old paths or
all new ones, never a mixture.

**`jsonb` (`docs.blocks.kind`, `docs.blocks.content`, `collab.ops.op`).**
These are tagged unions on the wire. See 3.2 — the serde representation is
the most consequential single decision in this part of the port.

**Generated columns (`search_vector`).** `GENERATED ALWAYS ... STORED`.
`sqlx` must never try to write it. Practically: never `SELECT *` into a
struct that has a field for it, and never include it in an `INSERT`.

**`TEXT[]` (`docs.page_deletions.steps_done`).** `Vec<String>` maps directly.
The saga appends one step name at a time and resumes at the first name *not*
present — see 9.4, and note that this makes appending a new step **reopen
completed sagas**, on purpose.

## 2.4 Sort keys

`docs.pages.sort_key` is a fractional index — an opaque, lexicographically
ordered string. Between any two keys another key always exists, so inserting
between two siblings writes **one row**, not a renumbering of the tail.

```rust,ignore
struct SortKey(String);        // invariant: non-empty, ASCII, never ends in the minimum digit
fn between(lo: Option<&SortKey>, hi: Option<&SortKey>) -> SortKey;
```

**Invariants.**
1. `between(a, b)` is strictly between `a` and `b` in byte order.
2. `between(None, b) < b` and `a < between(a, None)`.
3. The result never ends in the alphabet's minimum digit — otherwise no key
   can ever be generated below it, and the space is closed off silently.
4. Repeated `between(a, x)` grows the key by O(1) characters per call, not
   O(n).

**Test list.**
1. `between_is_strictly_between` (property test, `proptest`, thousands of
   pairs)
2. `repeatedly_inserting_at_the_head_does_not_grow_keys_linearly` — the
   hardest, and the one that catches a naive midpoint implementation
3. `a_key_never_ends_in_the_minimum_digit`
4. `ordering_matches_the_database_collation` — an integration test, because
   Rust's `Ord` on `String` and Postgres's `text` ordering under a non-C
   collation are not the same relation

**DSA.** Fractional indexing / order-maintenance. LeetCode: *1585. Check If
String Is Transformable* (no), better analogues are *"Insert Interval"* (57)
and *"My Calendar I"* (729, **closest** — interval insertion with ordering).

**After:** Figma's fractional-index post is the canonical write-up, and
documents the collation trap in production. The spec chose ASCII-only keys so
that Rust's `Ord`, Postgres's `C` collation and JavaScript's `<` all agree.

## 2.5 `collab.ops` — the append-only log

```sql
-- the shape, from DATA_MODEL.md
id            uuid primary key,   -- v7, assigned client-visibly before write
page_id       uuid not null,
actor_id      uuid not null,
actor_kind    text not null,      -- 'user' | 'assistant' | 'system'
seq           bigint not null,    -- per page, gapless, monotonic
lamport       bigint not null,
undo_group    uuid,               -- null = a gesture of one
op            jsonb not null,
content_version int not null,     -- see 2.6
created_at    timestamptz not null default now()
```

**Invariant D2.2 — `seq` is gapless and per page.** Not global. A gap means
either a lost write or a bug in the sequencer, and the replay must be able to
tell the difference by inspection.

**Invariant D2.3 — `actor_kind` is TEXT with an open set, not an ENUM.**
Extending a `TEXT` column is ordinary DDL; extending an ENUM is a
special-cased transaction that cannot run inside a normal migration on every
Postgres version. Same reasoning for `notify.notifications.kind`.

## 2.6 `content_version`, and the rule that outlives everything

Every persisted op carries the version of the op grammar that wrote it.

**The rule: an old reader must survive a new writer, forever.** Field numbers
are never renumbered and never reused; messages only ever grow. Not because
of protobuf — because a future `history-service` replays events written by
every prior release, and the moment one of them is unreadable the log stops
being the source of truth.

In Rust this means: **never** `#[serde(deny_unknown_fields)]` on an op type.
Unknown fields are a future version speaking, and dropping them is correct
where erroring is not.

---

# Part 3 — The document block model

`RFC-001` is the authority. This part is the Rust shape of it.

## 3.1 The core types

```rust,ignore
struct Page {
    id: PageId,
    title: String,
    root: Vec<BlockId>,              // top-level order
    blocks: HashMap<BlockId, Block>, // flat store, tree by parent pointers
}

struct Block {
    id: BlockId,
    parent: Option<BlockId>,         // None = top level
    kind: BlockKind,
    content: Content,
    children: Vec<BlockId>,          // order within the parent
}

struct Content {
    text: String,                    // the block's own text, never its children's
    marks: Vec<Mark>,
}

struct Mark {
    kind: MarkKind,
    start: usize,                    // BYTE offsets into `text`
    end: usize,
}
```

**The flat-store-plus-parent-pointers choice.** The document is a tree and is
stored as a map plus order vectors, not as `Box<Block>` children. Three
reasons, and they matter more in Rust than they did in Go:

1. **Ops address blocks by id.** `MoveBlock { id, to_parent, after }` needs
   O(1) lookup by id; a recursive tree makes that a search.
2. **Borrowck.** Mutating one block while reading its parent is routine here
   (an op needs both), and with owned children that is two overlapping
   mutable borrows. With a `HashMap` it is two lookups.
3. **The projection is flat.** `docs.blocks` is `(id, page_id, position,
   kind, content)`. Keeping the in-memory shape close to the stored shape
   means the projection is a fold, not a translation.

**Invariant B3.1 — the parent pointer and the children vector agree.**
`blocks[c].parent == Some(p)` iff `blocks[p].children.contains(c)`. This is
the invariant that a `MoveBlock` implementation gets wrong first. Assert it
in debug builds after every op.

**Invariant B3.2 — no cycles.** A block is never its own ancestor. `MoveBlock`
must reject moving a block into its own subtree, with an error, not a panic
and not silently.

**Invariant B3.3 — marks are byte offsets into this block's own `text`,
sorted, non-overlapping per kind, and always on a UTF-8 boundary.** A mark
ending mid-codepoint is a rendering crash waiting to happen. Rust makes this
checkable in a way Go did not: `text.is_char_boundary(start)`.

> **Carried-over bug, ported as a decision.** The TypeScript frontend uses
> **UTF-16** offsets for marks (JS string indices), while the backend
> persists **byte** offsets. Identical for ASCII, wrong for anything else.
> Port it as a stated conversion at the boundary — a `Utf16Offset` newtype
> and an explicit `to_bytes(&str)` — rather than inheriting the ambiguity.

## 3.2 `BlockKind` — the most important serde decision in the port

```rust,ignore
#[derive(Serialize, Deserialize)]
#[serde(tag = "tag", rename_all = "snake_case")]
enum BlockKind {
    Paragraph,
    Heading { level: u8 },           // 1..=3
    Quote,
    CodeBlock { language: Option<String> },
    Divider,
    Callout { tone: CalloutTone },
    Aside,
    Toggle,
    List { list_kind: ListKind },
    ListItem { checked: bool },
    Image { file_id: Uuid },
}
```

`#[serde(tag = "tag")]` — **internally tagged**, matching the Go side's
`{"tag":"heading","level":2}` exactly. Not `#[serde(untagged)]`, which would
parse `Paragraph` and `Aside` interchangeably because both serialise to
`{}`; and not adjacently tagged, which changes the wire format and breaks
every stored row.

**Invariant B3.4 — `Callout.tone` is an enum in the model
(`warn|note|info|tip|danger|success`) and a *hue* only in the design system.**
The document says the tone; the frontend decides the colour. A model that
stores `"amber"` has hard-coded a stylesheet into the op log.

**Invariant B3.5 — `Code` has no `Marks`.** Not "marks are ignored inside
code" — the bubble menu must be *unreachable* there. A representable state
that must never occur is a bug the type system should have prevented; if
`Content` is shared, gate it behind a constructor that refuses.

## 3.3 `MarkKind`

```rust,ignore
#[serde(tag = "kind", rename_all = "snake_case")]
enum MarkKind {
    Bold, Italic, Strike, Code,
    Link { href: String },
    Highlight,                        // exactly ONE highlight colour
    PageLink { target: PageId },      // see the note below
}
```

**One highlight colour, on purpose.** A second would have to *mean*
something, and nothing in the product says what. It is also the one place a
mark could collide with the semantic hues (amber = diagnostic, teal = you).

> **Known gap, carried forward.** `PageLink` is **not implemented** as a mark
> today. Backlinks are produced by `blockproj` scanning plain text for
> `[[Title]]` with a regex. The port should decide deliberately: either
> implement the mark (and then the projection reads marks, not text), or keep
> the scan and delete `PageLink` from the enum. Do not leave a variant that
> nothing produces.

## 3.4 `Page::apply` — the one mutation path

```rust,ignore
impl Page {
    fn apply(&mut self, op: &Op) -> Result<(), ApplyError>;
    fn snapshot(&self) -> Snapshot;                 // the wire shape
    fn replay(ops: &[LoggedOp]) -> Result<Page, ApplyError>;
}

enum ApplyError {
    UnknownBlock(BlockId),
    WouldCycle { block: BlockId, into: BlockId },
    OffsetOutOfRange { block: BlockId, at: usize, len: usize },
    NotACharBoundary { block: BlockId, at: usize },
    KindForbidsMarks(BlockId),
}
```

**Invariant B3.6 — the UI never mutates the tree.** Every change is an op.
There is no `set_text` on `Block` that is not reached through `apply`. In Go
this was a convention; in Rust, make the fields private to the crate and the
convention becomes a compile error.

**Test list for Part 3.**
1. `apply_then_invert_restores_the_document` — over the golden vectors in
   `testdata/document-core/`, then as a `proptest` over random op sequences.
   **The hardest and the most valuable test in the entire port.**
2. `move_block_into_own_subtree_is_rejected`
3. `parent_pointer_and_children_vector_agree_after_every_op` (property)
4. `a_mark_never_ends_mid_codepoint` (property, with a multi-byte generator)
5. `code_block_refuses_marks`
6. `replay_from_empty_equals_incremental_apply` (property — this is I0.2)
7. `unknown_fields_in_a_stored_op_do_not_fail_to_parse` (this is 2.6)

**Before:** *Crafting Interpreters* ch. 5 (Representing Code) for why a
tagged union beats a property bag; *Rust for Rustaceans* ch. 2 for the
newtype and privacy mechanics that make B3.6 enforceable.

**DSA.** Tree manipulation with parent pointers, cycle detection on move.
LeetCode: *1490. Clone N-ary Tree*, *431. Encode N-ary Tree to Binary Tree*,
*[**closest**] 1379 / 863 Nodes at distance K* for the parent-pointer
traversal shape.

**After:** ProseMirror stores a persistent tree with position-based
addressing and rebases positions through steps; Notion stores a flat block
table with parent ids, which is what this model is. The spec chose flat +
ids because ops are the API, and an op that names a position instead of an id
cannot survive a concurrent edit (Part 7).

---

# Part 4 — The grammar, in full

Three grammars, and conflating them is the classic mistake. They are:

1. **The document grammar** — what a block tree may contain. Part 3's types.
2. **The input grammar** — what typing does. §4.2.
3. **The paste grammar** — what arriving text means. §4.3.

They overlap and are not the same, and each has a different failure mode.

## 4.1 The document grammar, as a production

```
Page      := Title Block*
Block     := Leaf | Container
Leaf      := Paragraph | Heading | CodeBlock | Divider | Image
Container := Quote | Callout | Aside | Toggle | List
Quote     := 'quote'    Block*
Callout   := 'callout' Tone Block*
Aside     := 'aside'    Block*
Toggle    := 'toggle' Summary Block*
List      := 'list' ListKind ListItem+
ListItem  := 'list_item' Checked? Block*         -- nesting is a List inside a ListItem
Text      := (Char | Mark)*
Tone      := warn | note | info | tip | danger | success
ListKind  := bulleted | numbered | todo
```

**Rules the production does not show, which are still part of the grammar:**

- **G4.1** — `CodeBlock.content.marks` is always empty (B3.5).
- **G4.2** — `Divider` and `Image` have no children and no text of their own
  beyond a caption.
- **G4.3** — `List` has only `ListItem` children. A paragraph directly inside
  a list is not representable and must be rejected, not normalised silently.
- **G4.4** — `Heading.level ∈ 1..=3`. Not 1..=6. The reason is the outline:
  four levels of indent stop being legible in a 272px rail, and a level
  nobody can see is a level nobody should be able to write.
- **G4.5** — nesting a list means a `List` inside a `ListItem`, never a
  `ListItem` inside a `ListItem`. This is the rule an implementation gets
  wrong when it makes indentation a number instead of structure.

**Why a `List` container at all**, rather than a flat run of `ListItem`s with
a `list_kind` each: a list is a *set*, and its kind belongs to the set. Flat
items let two adjacent items disagree about whether they are the same list,
and rendering then has to guess. It also makes "convert this list to
numbered" one op instead of n.

## 4.2 The input grammar — what typing does

An input rule fires on a **bounded lookbehind** from the caret. Never a
reparse of the block.

| Trigger | Becomes | Lookbehind |
|---|---|---|
| `# ` `## ` `### ` at block start | `Heading{1,2,3}` | 4 bytes |
| `> ` at block start | `Quote` | 2 |
| `- ` `* ` at block start | `List{bulleted}` + item | 2 |
| `1. ` at block start | `List{numbered}` + item | 3 |
| `[] ` `[x] ` at block start | `List{todo}` + item | 4 |
| ` ``` ` at block start | `CodeBlock` | 3 |
| `---` alone | `Divider` | 3 |
| `**x**` `*x*` `~~x~~` `` `x` `` | the mark, inline | ≤ 48 |
| `[[` | opens the page-link autocomplete | 2 |
| `::` | opens the container picker | 2 |
| `/` at block start | opens the slash menu | 1 |

**Invariant G4.6 — the lookbehind is bounded by a constant, and the constant
is stated.** 48 bytes. This is the whole difference between typing being
cheap and typing being a parse: the cost of a keystroke must not grow with
the size of the block, and certainly not with the size of the document.

**Invariant G4.7 — an input rule is one gesture.** `# ` → heading is a
`SetBlockKind` **and** a `DeleteText` of the `"# "`, sharing one
`undo_group`, so one ⌘Z gives back exactly what you typed. Two undo steps
here is the bug users actually report.

**Invariant G4.8 — input rules do not fire inside a `CodeBlock`.** Typing
`# ` in code means `# `. This falls out of G4.1 if the check is "does this
kind admit marks/structure", and has to be written by hand if it is not.

## 4.3 The paste grammar

Pasting is the only place a *parser* in the classical sense exists. See
Part 5.

**Invariant G4.9 — paste is one gesture.** However many ops it emits, one
`undo_group`. Pasting a 200-block document and pressing ⌘Z once must leave
the document exactly as it was.

**Invariant G4.10 — paste never produces a kind the grammar forbids.** The
parser's output goes through the same `Page::apply` as a keystroke does; it
has no privileged path. If the parser can produce something `apply` rejects,
the bug is in the parser and `apply` is right to reject it.

## 4.4 Test list for Part 4

1. `every_input_rule_fires_and_is_one_undo_step` — table-driven over the
   table above.
2. `input_rules_are_inert_inside_a_code_block`
3. `a_list_rejects_a_paragraph_child`
4. `nesting_produces_a_list_inside_an_item_not_an_item_inside_an_item` —
   **the hardest**, because the wrong shape renders identically at one level
   of depth and only breaks at two.
5. `lookbehind_never_exceeds_48_bytes` — instrument the scan and assert the
   bound; a test that only checks the *result* will pass on a full reparse.
6. `heading_level_four_is_not_representable`

**Before:** *Crafting Interpreters* ch. 6 (Parsing Expressions) — for the
distinction between a grammar and its recogniser, which is exactly the
document/input/paste split above.

**After:** Notion's input rules are a fixed table like this one; Obsidian
reparses the whole line and pays for it on long lines. The spec chose the
bounded scan because the editor has to stay at 60fps while someone types into
a 5,000-word block.

---

# Part 5 — The parser and the paste pipeline

This is the part with the most Rust-specific pleasure in it, and the part
where a Go-to-Rust translation is most obviously the wrong move. Write it
from the grammar.

## 5.1 The pipeline

```
bytes  ──lex──▶  tokens  ──parse──▶  AST  ──lower──▶  block tree  ──emit──▶  ops
```

Four passes, each **lossy on purpose**, and the loss is the design:

- **lex** discards whitespace runs and comment syntax, keeps positions.
- **parse** discards token positions unless deliberately retained; produces
  an AST, **not** a parse tree.
- **lower** discards syntax — after this pass nothing can depend on whether
  the author wrote `*x*` or `_x_`.
- **emit** discards the tree shape, producing a flat op sequence.

**Invariant P5.1 — the round trip.** Replaying the emitted ops from an empty
page must produce **exactly** the tree the lowering pass built directly. This
is the only claim on the whole compiler screen worth testing, and it is a
property test, not an example.

## 5.2 The lexer

A DFA with an output tape. That framing is not an analogy — it is the
implementation, and it fixes the cost up front: one pass, one byte at a time,
no backtracking.

```rust,ignore
enum Token<'src> {
    Text(&'src str),
    Hash(u8),                 // run length, for heading level
    Backtick(u8),             // 1 = inline code, 3 = fence
    Star(u8), Underscore(u8), Tilde(u8),
    BracketOpen(u8),          // 1 = link, 2 = page link
    BracketClose(u8),
    Gt, Dash, Digit(u8), Dot,
    Newline, BlankLine,
    Eof,
}

struct Lexer<'src> { src: &'src str, pos: usize, state: State }
impl<'src> Iterator for Lexer<'src> { type Item = Spanned<Token<'src>>; }
```

**`&'src str`, not `String`.** The lexer borrows from the input and allocates
nothing for ordinary text. This is the concrete thing the port buys you over
Go, where every token carrying text either allocated or carried an index pair
by hand. Make the borrow explicit and let the lifetime do the work.

**Invariant P5.2 — the lexer always advances.** Every state transition
consumes at least one byte. A parser that cannot advance past a malformed
token *hangs*, and it hangs in production on someone's pasted document. Emit
a recovery token and continue; the diagnostic is more useful than the halt.
Assert `pos` strictly increases in a debug wrapper.

**Invariant P5.3 — offsets are BYTE offsets, and every span lands on a char
boundary.** Rust makes the second half free (`is_char_boundary`); use it in a
debug assertion rather than trusting it.

## 5.3 The parser

Recursive descent, block-level first, then inline within each block.

```rust,ignore
enum Node {
    Paragraph(Vec<Inline>),
    Heading { level: u8, inline: Vec<Inline> },
    CodeFence { language: Option<String>, body: String },
    Quote(Vec<Node>),
    List { kind: ListKind, items: Vec<Vec<Node>> },
    Divider,
}

enum Inline {
    Text(String),
    Marked { kind: MarkKind, inner: Vec<Inline> },
    PageLink(String),        // the title as written; resolution happens later
}

fn parse(tokens: &[Spanned<Token>]) -> (Vec<Node>, Vec<Diagnostic>);
```

**Note the return type: `(ast, diagnostics)`, never `Result`.** A paste with a
malformed fence must still produce a document. There is no "the paste failed"
state that is useful to a person — there is a document, and a list of things
that were odd about it.

**Invariant P5.4 — the AST is not the parse tree.** No node exists purely to
record that a delimiter was seen. Conflating them is the single fastest way
to make a front end untestable, because every test then asserts on syntax
instead of meaning.

**Invariant P5.5 — inline nesting is bounded.** Cap it (16 is generous). A
crafted input of 50,000 nested `*` is a stack overflow, which in Rust is an
abort, not a catchable error. This is a real denial-of-service on a
paste handler and the bound is the fix.

## 5.4 Lowering

`Node` → `Block` tree. This is where G4.3/G4.5 are enforced: a `List` gets
`ListItem` children and nothing else, and a nested list becomes a `List`
inside a `ListItem`.

**Invariant P5.6 — lowering is total.** Every `Node` maps to something. If
the parser can produce a node that lowering cannot place, the enum has a
variant that should not exist. Prefer making it unrepresentable to handling
it.

## 5.5 Emission

Tree → `Vec<Op>`, in an order where every op is applicable when it is applied:
a block's `InsertBlock` precedes any op naming it, and `after` names a
sibling that already exists.

**Invariant P5.7 — emission order is a valid application order.** A
topological property, and testable as one: apply the ops in order to an empty
page and assert no `UnknownBlock` error.

**Invariant P5.8 — the whole paste is one `undo_group`** (G4.9).

## 5.6 Test list for Part 5

1. `replay_of_emitted_ops_equals_the_lowered_tree` — **the hardest and the
   whole point**. `proptest` generating random markdown-ish documents,
   round-tripping, comparing trees.
2. `the_lexer_always_advances` (property, arbitrary bytes — including
   invalid UTF-8 handling at the boundary)
3. `an_unterminated_code_fence_produces_a_document_and_a_diagnostic`
4. `deeply_nested_emphasis_is_bounded_not_a_stack_overflow`
5. `every_span_is_on_a_char_boundary` (property, multi-byte generator)
6. `emission_order_applies_cleanly_to_an_empty_page`
7. `pasting_then_undoing_once_restores_the_document_exactly`

**Before:** *Crafting Interpreters* ch. 4 (Scanning) and ch. 6 (Parsing
Expressions) — the two chapters this design is a direct application of.
*Rust for Rustaceans* ch. 3 for the `&'src str` borrow shape.

**DSA.** Recursive descent; DFA construction; bounded lookbehind.
LeetCode: *772. Basic Calculator III*, *394. Decode String*, *[**closest**]
1106. Parsing A Boolean Expression*, *20. Valid Parentheses* for the nesting
bound.

**After:** `pulldown-cmark` is a pull parser producing a flat event stream,
which is the right shape when you are rendering; this needs a tree because it
is producing ops, so it parses to an AST first. `comrak` builds a full
CommonMark tree and is a good reference for the block/inline split — but
CommonMark's full grammar is far more than this product needs, and adopting
it would import edge cases (link reference definitions, setext headings,
HTML blocks) that have no block kind to lower into.

---

# Part 6 — The operation model

`RFC-002` is the authority. This is the ISA of the whole system.

## 6.1 Two tiers, one log

```rust,ignore
#[serde(tag = "scope", rename_all = "snake_case")]
enum Op {
    Block(BlockOp),                       // structure
    Text { block: BlockId, op: TextOp },  // characters within one block
}

#[serde(tag = "type")]
enum BlockOp {
    InsertBlock { id: BlockId, parent: Option<BlockId>, after: Option<BlockId>,
                  kind: BlockKind, content: Content },
    DeleteBlock { id: BlockId, subtree: Vec<Block> },   // carries what it removed
    MoveBlock   { id: BlockId,
                  from_parent: Option<BlockId>, from: Option<BlockId>,
                  to_parent:   Option<BlockId>, to:   Option<BlockId> },
    SetBlockKind    { id: BlockId, from: BlockKind, to: BlockKind },
    SetBlockContent { id: BlockId, prev: Content, next: Content },
    SetTitle        { from: String, to: String },
    SetPageTopic    { from: Option<TopicId>, to: Option<TopicId> },
    AddTag          { tag: String },
    RemoveTag       { tag: String },
}

#[serde(tag = "type")]
enum TextOp {
    InsertText { at: Option<Anchor>, text: String },
    DeleteText { range: AnchorRange, text: String },   // carries the removed text
    SetMark    { range: AnchorRange, kind: MarkKind, on: bool },
}
```

**Why two tiers and not one.** A character insertion inside a paragraph and a
block reorder have different concurrency behaviour: two people typing in
different blocks never conflict at all, and merging them as one flat sequence
would make them appear to. Splitting the ISA lets `collaboration-service`
hold one live rope **per block** and a block tree beside it, and lets the
common case — two people in different paragraphs — need no merge logic at all.

**Invariant R6.1 — every destructive op carries what it destroyed.**
`DeleteText` keeps the text. `DeleteBlock` keeps the whole subtree.
`MoveBlock` keeps `from` as well as `to`. `SetBlockKind` and `SetBlockContent`
keep `from`/`prev`.

This is I0.1 made concrete, and it is the invariant that must be designed in
rather than discovered: an op recorded with only its destination makes undo,
the trace screen and the palimpsest all impossible, and by the time you
notice, the log is full of ops that cannot be inverted.

## 6.2 `invert`

```rust,ignore
impl Op { fn invert(&self) -> Op; }
```

Note it takes **no state** — because of R6.1, the op already carries
everything the inverse needs. That is the whole payoff:

| Op | Inverse |
|---|---|
| `InsertBlock{id,..}` | `DeleteBlock{id, subtree: [that block]}` |
| `DeleteBlock{id, subtree}` | a sequence of `InsertBlock` restoring the subtree in order |
| `MoveBlock{from,to,..}` | `MoveBlock` with `from` and `to` swapped |
| `SetBlockKind{from,to}` | `SetBlockKind{from: to, to: from}` |
| `SetBlockContent{prev,next}` | `SetBlockContent{prev: next, next: prev}` |
| `InsertText{at,text}` | `DeleteText{range: at..at+len, text}` |
| `DeleteText{range,text}` | `InsertText{at: range.start, text}` |
| `SetMark{range,kind,on}` | `SetMark{range,kind,on: !on}` |

**Invariant R6.2 — `DeleteBlock`'s inverse restores the subtree in an order
that applies cleanly**, parents before children (P5.7's property again).

**Invariant R6.3 — invert is an involution up to application.**
`invert(invert(o))` need not equal `o` structurally (a `DeleteBlock` inverts
to several `InsertBlock`s), but applying it must restore the same state. Test
the *state*, not the op.

## 6.3 Undo groups

`undo_group: Option<Uuid>`. `None` is a gesture of one — the field's
documented meaning, not a special case.

**Invariant R6.4 — undo pops the newest `undo_group` belonging to THIS
actor**, never the newest op. Interleaved edits by someone else stay exactly
where they are.

**Invariant R6.5 — a new op by an actor clears that actor's redo stack.** The
same rule every editor holds itself to, and the one users notice instantly
when it is missing.

**Invariant R6.6 — undoing a restore reapplies the reverted steps in their
ORIGINAL order**, not the restore's own reverse order. A restore is itself
one gesture; undoing it must land you back at head, with the ops in the order
they were written.

## 6.4 Authorization

```rust,ignore
fn can_apply(op: &Op, actor: &Actor, page: &Page) -> Result<(), Denied>;
```

**Invariant R6.7 — one chokepoint.** Called from exactly one place, on the
path every op takes. Not one per service — one. In Rust, enforce it by making
the "applied" op a distinct type that only `can_apply` can construct:

```rust,ignore
struct Authorized(Op);        // private field; only can_apply returns one
fn apply(&mut self, op: Authorized) -> Result<(), ApplyError>;
```

That turns I0.3 from a code-review rule into a type. It is the single best
argument for this port existing.

## 6.5 The WAL

Every op is written to a local write-ahead log **before** it is acknowledged,
and the flush to Postgres happens after.

**Invariant R6.8 — ack implies durable.** If the client saw an ack, the op
survives a crash. Anything else and "converged" is a lie the status bar is
telling.

**Invariant R6.9 — the WAL is per page and ordered.** Recovery replays it in
order; a gap is a corruption, not a hint.

## 6.6 Test list for Part 6

1. `apply_then_invert_is_identity_for_every_op_kind` — table-driven over
   every variant, then `proptest` over sequences. **The hardest**, and it
   subsumes most of the others.
2. `undo_pops_this_actors_group_not_the_newest_op` — with interleaved ops
   from two actors, which is the case that fails when R6.4 is implemented as
   "pop the last op".
3. `a_new_op_clears_redo`
4. `undoing_a_restore_reapplies_in_original_order`
5. `delete_block_inverse_restores_the_whole_subtree_in_a_valid_order`
6. `an_op_that_fails_can_apply_never_reaches_the_log`
7. `ack_after_a_kill_9_still_finds_the_op_on_restart` (integration)

**Before:** *DDIA* ch. 5 (Replication) for the log-as-truth argument; *DDIA*
ch. 7 (Transactions) for what "durable" has to mean before you promise it.

**DSA.** Undo/redo as two stacks with grouping; command pattern with
inverses. LeetCode: *[**closest**] 1472. Design Browser History*, *155. Min
Stack*, *622. Design Circular Queue*.

**After:** Automerge and Yjs both make every op invertible but store the
inverse implicitly in the CRDT structure; this spec stores it explicitly in
the op, which costs bytes and buys a readable log — the trace screen, the
palimpsest and per-actor undo are all consequences of that one choice.

---

# Part 7 — Anchors, and why offsets die

The shortest part, and the one that decides whether the collaborative editor
works at all.

## 7.1 The problem, precisely

Actor A holds "insert at offset 14". Actor B, concurrently, deletes five
characters at offset 3. By the time A's op arrives, offset 14 names a
different position — and nothing in A's op says so. The document does not
diverge loudly; it diverges *quietly*, one character out, and neither peer
can tell.

**I0.5: an integer offset is only valid in the document version that produced
it, and no op survives to be applied in that version.**

## 7.2 The type

```rust,ignore
/// A position that survives concurrent edits: it names a NEIGHBOUR, not an
/// index. `after: None` means the start of the block.
struct Anchor {
    after: Option<CharId>,     // the character this position follows
    // Resolution is: find `after` in the block's character sequence
    // (including tombstones) and take the position immediately following it.
}

struct AnchorRange { start: Anchor, end: Anchor }

/// Globally unique per inserted character: (actor, lamport, index-in-run).
/// Never reused, never reassigned, and NOT removed on delete — see 13.2.
struct CharId { actor: ActorId, lamport: u64, offset: u32 }
```

**Invariant A7.1 — an anchor resolves in any version that contains its
neighbour, and the neighbour is never removed.** Deleting a character
tombstones it (Part 13); the `CharId` stays in the sequence forever, so an
anchor pointing at it still resolves. This is *why* the palimpsest exists as
a data structure and not only as a screen.

**Invariant A7.2 — an anchor whose neighbour is unknown is an error, not a
guess.** If the character is genuinely absent, the op arrived from a version
this replica has not seen — that is a real fault, and clamping to offset 0 is
how you get silent corruption instead of a loud one.

**Invariant A7.3 — offsets exist only inside one process, one version.** They
appear in the wasm bridge and in the DOM. They never cross a network
boundary, are never stored, and never appear in an op.

## 7.3 Test list

1. `an_anchor_resolves_after_a_concurrent_insert_before_it` — the base case.
2. `an_anchor_resolves_after_its_neighbour_is_deleted` — **the hardest**, and
   the reason tombstones are not optional.
3. `an_anchor_with_an_unknown_neighbour_errors` (A7.2)
4. `two_replicas_applying_the_same_ops_in_different_orders_converge`
   (property, `proptest` shuffling causally-independent ops)

**Before:** *DDIA* ch. 5, "Detecting Concurrent Writes" — the whole
happens-before framing this rests on.

**After:** This is RGA/Causal-Trees in the CRDT literature (Attiya et al.,
"Specification and Complexity of Collaborative Text Editing"). Yjs calls the
same thing an `ID`; Automerge calls it an `OpId`. The spec's version is
deliberately the simplest one that supports per-actor undo — which most CRDT
text implementations do *not*, because their inverses are implicit.

---

# Part 8 — `collaboration-service`, the stateful one

The only service that holds state in memory, and the only one that scales on
connection count rather than request rate. It owns `collab` — the op log and
its outbox — and nothing else may write there.

## 8.1 The shape

```rust,ignore
struct Manager { sessions: DashMap<PageId, Arc<Session>> }

struct Session {
    page: Mutex<Page>,                     // documentcore::Page — block structure
    text: DashMap<BlockId, TextBuffer>,    // one live buffer per block
    log:  Mutex<OpLog>,                    // confirmed ops, in order
    wal:  Wal,                             // durable before ack (R6.8)
    subs: RwLock<Vec<Subscriber>>,         // one per connection
    seq:  AtomicU64,
    lamport: AtomicU64,
}
```

**Invariant C8.1 — one session per page, and every connection to that page
shares it.** The session *is* the serialisation point. This is what makes
I0.4 achievable without a consensus protocol: there is exactly one writer.

**Invariant C8.2 — a session never idle-evicts.** *This is a known gap
carried forward from the Go implementation*, matching this repo's demo scale.
The port should either implement eviction (with a documented policy: evict
when subscriber count is 0 and the WAL is flushed) or state the gap in the
same words. Do not leave it undecided.

## 8.2 The frame flow

```
client op ──▶ can_apply ──▶ WAL append ──▶ apply to session ──▶ ack sender
                                                          └──▶ broadcast others
                                        ──▶ (async) flush to collab.ops
                                        ──▶ (async) outbox ──▶ NATS collab.ops_flushed
```

**Invariant C8.3 — ack after WAL, before Postgres.** R6.8 is satisfied by the
WAL, not by the database. Waiting for Postgres to ack a keystroke is a
round trip per character and the editor stops feeling live.

**Invariant C8.4 — the sender gets an `ack`, everyone else gets a
`broadcast`, and they are the same op.** Not a different shape, not a
different code path. `useCollabPage` needs no special handling for undo/redo
precisely because the server drives them down this same path.

**Invariant C8.5 — presence is join/leave, not inferred from op traffic.** A
person reading a page is present. Inferring presence from edits means a
reader is invisible and someone who stops typing disappears.

## 8.3 The WebSocket protocol

`docs/api/collaboration.md` is the authority; it is one contract with no REST
projection, because a persistent connection is not a request/response
resource.

```rust,ignore
#[serde(tag = "type", rename_all = "snake_case")]
enum ClientMessage {
    Op { op: Op, undo_group: Option<Uuid> },
    Cursor { cursor: CursorWire },
    Undo, Redo,
    Restore { to_step: u64 },
}

#[serde(tag = "type", rename_all = "snake_case")]
enum ServerMessage {
    Snapshot { snapshot: Snapshot, present: Vec<ActorId>, cursors: Vec<CursorWire> },
    Ack { op: LoggedOp, boundaries: Option<AnchorRange> },
    Broadcast { op: LoggedOp, boundaries: Option<AnchorRange> },
    Presence { actor_id: ActorId, joined: bool },
    Cursor { cursor: CursorWire },
    Error { message: String },
}
```

**Invariant C8.6 — `Restore` is repeated undo, not a snapshot swap.** It
writes the inverses as new ops, all under one new `undo_group` for the
requester (R6.6). Restoring to head is a **no-op, not an error**.

**Invariant C8.7 — a `restore` out of range is an error frame, not a dropped
connection.** The client can send another one.

## 8.4 The concurrency shape, and the Rust decision

Go used a mutex per session and a goroutine per connection. The port has two
credible options and they are not equivalent:

| | `Arc<Mutex<Session>>` | actor (`mpsc` + one owning task) |
|---|---|---|
| Ownership | shared, guarded | single owner, messages |
| Deadlock risk | real (two locks: page + log) | none by construction |
| Backpressure | invisible | explicit — the channel has a bound |
| Ordering | whatever the mutex grants | the channel order, deterministically |
| Cost | a lock per frame | a send + a task wake per frame |

**Recommendation: the actor.** Not for performance — for I0.4. The channel
*is* the serialisation point, so "one writer" stops being a thing you
maintain and becomes a thing the structure guarantees. It also gives you
backpressure for free (8.5), which the mutex version has to bolt on.

**Invariant C8.8 — the session task owns the state; nothing else holds a
reference to it.** Readers get snapshots by asking, not by locking.

## 8.5 Backpressure

**Invariant C8.9 — a slow subscriber must not slow the session.** Each
subscriber has a bounded outbound queue. When it fills: **drop the subscriber
and let it reconnect** (it will get a fresh `Snapshot`, which is correct by
construction). Do not block the session, and do not grow the queue without
bound — one stalled browser tab must not be able to stop everyone else's
typing, or exhaust the server's memory.

## 8.6 Test list

1. `two_clients_editing_different_blocks_never_conflict`
2. `two_clients_editing_the_same_block_converge` (property, shuffled
   causally-independent ops) — **the hardest**
3. `an_ack_precedes_the_postgres_write_and_survives_a_kill` (integration,
   testcontainers, SIGKILL between WAL and flush)
4. `undo_by_A_leaves_Bs_interleaved_ops_in_place`
5. `restore_to_head_is_a_noop_not_an_error`
6. `a_marked_block_falls_back_to_whole_block_last_write_wins` — the stated
   tradeoff in `web/src/collab/marks.ts`, ported as a decision
7. `a_stalled_subscriber_is_dropped_and_the_others_keep_typing` (C8.9)
8. `presence_reports_a_reader_who_never_edits` (C8.5)
9. `goleak_equivalent`: no task outlives a closed session — in Rust, assert
   the `JoinSet` drains

**Before:** *Rust Atomics and Locks* ch. 1–4 for why `Mutex<T>` and `RwLock<T>`
mean what they mean here; *Crust of Rust: "Channels"* for the actor shape.

**After:** Figma's multiplayer server is one process per document with an
in-memory authoritative copy — the same shape. The spec chose it for the same
reason: a single serialisation point per document removes the need for
consensus between replicas of the same document.

---

# Part 9 — `document-service`: pages, projections, sagas

Stateless. Owns `docs`. Two gRPC services (`PageService`, `GraphService`,
plus `SearchService` and `DiscoverService`), HTTP for probes only.

## 9.1 Pages

Ordinary CRUD over `docs.pages`, with three things that are not ordinary:

**Reparent is two statements in one transaction.** Move the page (new
`parent_id`, new `path`, new `sort_key`), then rewrite every descendant's
path:

```sql
UPDATE docs.pages
SET path = @new_prefix::ltree || subpath(path, nlevel(@old_prefix::ltree))
WHERE path <@ @old_prefix::ltree AND id != @page_id;
```

**Invariant S9.1 — all old paths or all new ones, never a mixture** (D2.1).

**`ListPages` lists direct children only.** A full tree is built by calling it
once per expanded node. This is deliberate: the rail is lazily loaded, and a
"give me everything" mode would be the query that gets slow first and is
hardest to remove later.

**No owner scoping.** Every page on the instance is visible to every
authenticated actor. This is a *shared workspace, not multi-tenancy* —
`created_by` records who made a page, and is not an access filter. Real
multi-user collaboration needs the second person to see the first person's
pages.

## 9.2 The block projection

`internal/blockproj` consumes `collab.ops_flushed` from NATS and materialises
`docs.blocks` and `docs.page_links`.

**Invariant S9.2 — the projection is never a second writer.** No request
handler writes `docs.blocks`. Ever. It is rebuilt from the event stream, and
a handler that "just fixes up" a row has forked the source of truth.

**Invariant S9.3 — the projection is idempotent.** NATS is at-least-once
(O2), so consuming the same flush twice must produce the same rows. Replace
the page's blocks wholesale rather than patching incrementally: at this scale
a full rewrite per flush is cheaper than making a patch idempotent, and it
makes I0.2 trivially true.

**Backlinks** are extracted by scanning each block's plain text for
`[[Title]]`, resolving against `docs.pages` by lowercased title. Unresolved
targets are stored with `target_page = NULL` — a *dangling* link, which is a
real state the diagnostics engine reports and the graph deliberately excludes
as an edge (there is nothing on the other end to draw a line to).

## 9.3 Classification

Two tables, and the split is a modelling claim:

- **Topic** — singular, owned, indexed. It clusters the graph, colours a
  node, scopes `/discover`. `topic_id` is **nullable**, because *untopiced is
  a real state* the UI reports and offers to fix, not a gap to backfill.
- **Tag** — free-form, many. It facets search. It never boosts rank and never
  picks a hue.

Collapsing them gives you folders: a page genuinely about two things has to
lie about one, and every consumer has to guess which label was load-bearing.

**Invariant S9.4 — `color_key` stores a KEY, never a hex value.** The palette
belongs to the design system. Shipping colours over the wire forks it.

**Invariant S9.5 — `page_tags` has no id and no `tags` table.** A tag has no
properties beyond its own text; a lookup table buys renaming a string at the
cost of a join on every read.

**Invariant S9.6 — tags normalise (lowercase, trim) but REJECT internal
whitespace.** `" CRDT "` → `crdt`; `"two words"` is an error, because it is
almost always two tags typed as one.

## 9.4 The page-delete saga

Deleting a page is a **state, not an event**. `docs.page_deletions` holds
progress:

```rust,ignore
struct PageDeletion {
    page_id: PageId,
    steps_done: Vec<StepName>,   // appended one at a time
    attempts: i32,
    started_at: OffsetDateTime,
}
```

Six steps, resumed at the first name **not** present in `steps_done`.

**Why a separate table.** `docs.pages.lifecycle_state` says what a page *is*;
it structurally cannot say how far a delete got. Progress belongs to the
operation — it has its own retry count, and once the page is purged the row
is history rather than state. It also keeps the table the editor blocks on
from widening for state almost every row has no use for.

**Invariant S9.7 — appending a step reopens completed sagas, on purpose.** A
page deleted before embeddings existed genuinely does have embeddings to
purge once they do. Pin this with a test so a later release cannot quietly
"fix" it.

**Invariant S9.8 — steps that have nothing to do are REAL steps reporting
`not_applicable`, not omissions.** Omitting them means the step list silently
changes shape when the feature lands; faking work means the trash screen
reports progress that never happened.

**Invariant S9.9 — the sweeper is a poller, not an in-process retry.** A
retry loop only survives the process that started it, and the case this
exists for is the one where that process died. Claim and work share one
transaction so `FOR UPDATE SKIP LOCKED` actually holds.

**Invariant S9.10 — waiting on an ack is not a retry.** `AwaitingAck` leaves
the saga in flight without bumping `attempts`, so that counter reads as
instability rather than latency. The ack times out into *proceeding*, never
failing: forward-only means a silent `collaboration-service` delays a purge,
it does not block one.

**Test list.**
1. `a_saga_resumes_at_the_first_incomplete_step`
2. `appending_a_step_reopens_a_completed_saga` (S9.7)
3. `two_sweepers_never_claim_the_same_saga` (integration)
4. `awaiting_an_ack_does_not_increment_attempts` — **the hardest**, because
   it is a test about a counter *not* moving
5. `an_ack_timeout_proceeds_rather_than_failing`

**Before:** *Database Internals* ch. 13 for sagas and compensating actions;
*DDIA* ch. 9 for why "forward-only with timeouts" beats distributed rollback
here.

**After:** Temporal/Cadence externalise exactly this state machine. The spec
chose a table plus a poller because a self-hosted notebook should not need a
workflow engine to delete a page.

---

# Part 10 — Graph algorithms

`marginal-graphalgo`: pure functions, no I/O, no async. The single easiest
crate to port and the best place to start (Part 18).

## 10.1 The types

```rust,ignore
struct NodeId(String);          // a page id as a plain string; zero deps on purpose
struct Edge { from: NodeId, to: NodeId }
struct Graph { nodes: Vec<NodeId>, edges: Vec<Edge> }
```

**`Graph` holds every node, including isolated ones.** Orphan detection has
to see a page sitting alone; an adjacency-list-only representation drops it.

**Adjacency is built per call, not cached on `Graph`.** The package holds no
state, and a graph is cheap to rebuild from `docs.page_links` per request at
this scale. In Rust, return a borrowed view:

```rust,ignore
struct Adjacency<'g> {
    directed:   HashMap<&'g NodeId, Vec<&'g NodeId>>,
    undirected: HashMap<&'g NodeId, Vec<&'g NodeId>>,
}
fn adjacency(g: &Graph) -> Adjacency<'_>;
```

## 10.2 The algorithm inventory

| Function | Algorithm | Answers |
|---|---|---|
| `components` | flood fill, undirected | can I get there at all |
| `orphans` | components ∌ any root | what nothing points into |
| `strongly_connected` | **Tarjan**, directed | can I get there AND back |
| `detect_cycle` | **three-colour DFS**, directed | show me one loop, as a path |
| `topological_sort` | **Kahn** | a reading order |
| `layers` | longest-path levelling over the topo order | what can be read in parallel |
| `bfs` / `shortest_path` | BFS, undirected | link distance |
| `nearest_neighbours` / `ring_sizes` | BFS + ranking | the ranked ring |
| `forward_reachable` | BFS, directed | blast radius |
| `diameter` | all-pairs BFS | the longest shortest path |
| `betweenness` | **Brandes** | bridges that degree does not warn you about |
| `modularity` | **Newman's Q** | does the declared partition match the wiring |
| `betti` | GF(2) rank of the triangle boundary map | real holes vs. mutual citation |
| `voronoi` / `delaunay` | half-plane clipping / dual | territory in the drawn layout |
| `convex_hull` / `territories` | monotone chain | where a topic's pages actually are |
| `neighbour_majority` | vote over the Delaunay dual | what the REGION is about |
| `layout_tick` | force-directed step | the drawing itself |

## 10.2b The algorithms, explained

A table is a checklist, not an explanation. Each of these has a shape worth
understanding before you write it, because the Rust version differs from the
textbook one in a specific way.

### Components (flood fill)

Repeatedly: take an unvisited node, BFS the undirected adjacency, mark
everything reached as one component. `O(V + E)`.

The only subtlety is the one in 10.1 — **isolated nodes are components of
size one**, and they are the interesting answer here (an orphan page). A
version that iterates edges rather than nodes silently reports fewer
components than exist.

### Cycle detection (three-colour DFS)

White = unvisited, grey = on the current stack, black = finished. A grey
node reached again is a **back edge**, and the cycle is the stack slice from
that node to the top.

Returning the *path* rather than a boolean is the whole point: the UI shows
the loop. In Rust the recursive form risks stack overflow on a deep tree, so
write it iteratively with an explicit stack of `(node, child_index)` — the
same shape you need for `topological_sort` anyway.

```rust,ignore
enum Colour { White, Grey, Black }
// stack: Vec<(NodeId, usize)>  — node plus how many of its children are done
```

### Tarjan (strongly connected components)

One DFS, two numbers per node: `index` (discovery order) and `lowlink` (the
smallest index reachable from its subtree, including one back edge). When
`lowlink == index`, pop the stack down to that node — that is one SCC.

Why it is here and `components` is too: **undirected reachability answers
"can I get there", SCC answers "can I get there and back"**. On a wiki those
are very different claims about a set of pages.

### Kahn (topological sort) and layers

Kahn: repeatedly emit any node with in-degree zero, decrement its
successors. If the queue empties with nodes remaining, there is a cycle —
which is why `detect_cycle` and this share a code path in spirit.

`layers` then levels the result: a node's layer is `1 + max(layer of its
predecessors)`. That is a *longest*-path computation, and it is what makes
the answer "what can be read in parallel" rather than "one valid order".

### Brandes (betweenness centrality)

The one algorithm here that is genuinely subtle. Naively, betweenness needs
all-pairs shortest paths and their counts — `O(V³)`. Brandes computes it in
`O(VE)` by running one BFS per source and then accumulating *dependencies*
backwards:

1. BFS from `s`, recording for each `v`: distance, predecessor list, and
   `sigma[v]` = number of shortest paths from `s` to `v`.
2. Walk the nodes in **reverse** BFS order, accumulating
   `delta[v] += (sigma[v]/sigma[w]) * (1 + delta[w])` for each `w` whose
   predecessor is `v`.
3. Add `delta[v]` to `betweenness[v]` for every `v != s`.

The insight is that the dependency of `s` on `v` decomposes recursively over
the shortest-path DAG, so one backward pass replaces the pairwise sum.

Why the product owns it: **degree does not find bridges.** A page with three
links can be the only path between two clusters, and that page is the one
whose deletion splits the workspace.

### Newman's modularity Q

`Q = Σ_c (e_c / m − (d_c / 2m)²)` where `e_c` is edges inside community `c`,
`d_c` the total degree of its members, `m` the edge count.

It scores a partition that is **declared** (the topic each page was tagged
with) against the wiring. High Q means the topics match how pages actually
link; low Q means the taxonomy and the reality disagree — which is a
finding, not a bug.

### Betti numbers over GF(2)

`b0` = connected components. `b1` = independent cycles = `E − V + b0` for a
graph. The interesting one needs the triangle boundary map: build the
1-skeleton, find all 3-cliques, and compute the rank of the boundary matrix
∂₂ over GF(2) by Gaussian elimination with XOR.

`b1 = dim(ker ∂₁) − rank(∂₂)`. In practice: **a mutual-citation triangle is
filled in and does not count as a hole; a genuine ring of four or more pages
does.** That distinction is why this is here rather than a cycle count.

Over GF(2) every operation is a XOR of bit rows, so use `Vec<u64>` bitsets
and `rank` becomes a tight loop. This is one of the few places in the port
where a small amount of bit-fiddling buys a large constant factor.

### Convex hull (monotone chain) and Voronoi/Delaunay

Hull: sort by `(x, y)`, build lower and upper chains, keeping a turn
direction via the cross product sign. `O(n log n)`, and the sort dominates.

Voronoi here is computed by **half-plane clipping**, not Fortune's sweep: for
each site, start with the viewport rectangle and clip it by the perpendicular
bisector against every other site. `O(n²)` per frame, which is fine for
tens of pages and is dramatically easier to get right. Delaunay is taken as
the dual (two sites are Delaunay-adjacent iff their Voronoi cells share an
edge).

**Be honest about the complexity choice in a comment.** A reader who knows
Fortune's algorithm will assume you did not, unless you say you chose not to.

### The force-directed layout tick

One step of Fruchterman–Reingold: repulsion between every pair
(`k²/d`), attraction along edges (`d²/k`), then a cooling factor.

**It must be seeded and deterministic.** A layout that differs per run makes
every screenshot, every diff and every test unreproducible. Seed a small PRNG
explicitly (`rand::rngs::SmallRng::seed_from_u64`) rather than using the
thread RNG.

The tick is a pure function `(positions, graph) -> positions`, which is what
lets it run in wasm and be tested without a browser.


## 10.3 The traps, one per algorithm

**Three-colour DFS, not a visited set.** A diamond (A→B, A→C, B→D, C→D)
visits D twice with no cycle anywhere. A visited set that treats "seen
before" as "found a cycle" false-positives on it. Three colours fix it: D is
BLACK (fully explored, off the stack) the second time, not GRAY. **A cycle
exists only when a directed edge reaches a node that is currently GRAY.**

**Tarjan: the back edge uses the neighbour's INDEX, not its lowlink.** Using
lowlink merges components that merely touch. This is *the* Tarjan bug, and
the test for it is two separate two-cycles joined by one edge.

**Component numbering must be canonical.** Tarjan's discovery order depends
on where the outer loop started. Renumber so the component holding the
smallest node id is 0 (I0.6) — otherwise the colour of a node changes between
two requests over an unchanged graph.

**Kahn returns its PARTIAL order on failure.** `(Vec<NodeId>, bool)`, not
`Result`. "These 40 are orderable, these 6 are tangled" is actionable; a bare
error is not. Ties break on node id.

**Layers use the LONGEST path, not the first prerequisite found.** A node
depending on both a 1-hop and a 3-hop ancestor belongs at level 3. Taking the
first prerequisite seen puts it at level 1 and draws a dependency arrow
pointing backwards.

**Orphans are not `backlinks == 0`.** A mutually-linked pair each has a
nonzero backlink count, and the pair is still unreachable from any root. The
question is about *components*, not degrees.

**Diameter skips disconnected pairs.** Treating them as infinite makes the
diameter infinite the moment one page is unlinked, which is always.

**Betti: `b0`/`b1` are graph facts; `b1_clique`/`b2`/`chi`/`triangles`/`rank2`
are properties of a CHOSEN complex.** Filling every 3-clique as a 2-simplex
is a modelling decision, not a measurement, and the code should say so where
it is made.

**Voronoi is clipped to a finite rectangle.** An unbounded diagram has no
finite polygon for an outermost site. "Exact within these bounds" is the
honest phrasing.

**The force layout must be seeded and deterministic.** Same seed, same
positions, every load. A graph that rearranges itself on refresh destroys the
spatial memory it exists to build.

## 10.4 The Rust-specific gain

Go's `map[NodeID]int` returns the zero value for a missing key, and
"component 0" and "not in the graph" are the same value. Rust's
`Option<usize>` makes them different, and every one of these algorithms has a
place where that distinction was maintained by hand in Go.

**Do not "improve" the signatures into `Result`.** `topological_sort`
returning `(order, is_dag)` is the right shape (see above), and
`neighbour_majority` *omitting* a node with no evidence is the right shape —
inventing a majority for a node with no labelled neighbours is exactly the
lie the SPACE lens exists to avoid.

## 10.5 Test list

1. `scc_splits_a_loop_from_what_only_touches_it` — a→b→c→a plus c→d.
   Undirected flood fill says one component; strong connectivity must not.
2. `scc_does_not_merge_components_that_merely_touch` — **the hardest**, and
   the direct test for the lowlink/index bug.
3. `three_colour_dfs_does_not_false_positive_on_a_diamond`
4. `topological_sort_returns_the_placeable_part_of_a_cyclic_graph`
5. `layers_use_the_longest_path_not_the_first_found`
6. `component_ids_are_stable_across_runs` (I0.6, run it 25 times)
7. `betweenness_ranks_a_bridge_above_a_hub` — a low-degree node on the only
   path between two clusters must outrank a high-degree node inside one
8. `voronoi_cells_tile_the_viewport_with_no_residual_area` (property: areas
   sum to the rectangle)
9. `the_layout_is_identical_for_the_same_seed`

**Before:** *Skiena* ch. 5 (Graph Traversal) and ch. 6 (Weighted Graphs).

**DSA.** LeetCode: *[**closest**] 1192. Critical Connections in a Network*
(Tarjan's bridges — same lowlink machinery), *207/210. Course Schedule I & II*
(Kahn), *547. Number of Provinces* (flood fill), *2360. Longest Cycle in a
Graph* (three-colour DFS).

**After:** `petgraph` implements most of this and is a legitimate choice for
a product. It is deliberately *not* used here: the entire point of the module
is that the algorithm is the thing being ported, and a dependency that
already contains it makes the port an exercise in adapters.

---

# Part 11 — Search: FTS, BK-tree, trie

Three structures answering three different questions. Keeping them separate
is the design.

## 11.1 Full-text search — Postgres, not a library

`docs.blocks.search_vector` is `tsvector GENERATED ALWAYS ... STORED`. The
index is maintained by the database, so it cannot drift from the row.

**Invariant F11.1 — the generated column is never written by the
application.** Never `SELECT *` into a struct with a field for it; never
include it in an `INSERT`.

**Invariant F11.2 — the UI says the index may lag.** It is a projection over
an event stream. A status bar admitting "index may lag" is more trustworthy
than one implying a transaction it does not have.

**Invariant F11.3 — every full-text query takes a `Scope`** (Part 22.4).
This is not a general principle restated; it is the specific bug this
service shipped. `SearchPageTitles` and `SearchBlockText` were unscoped, so
a search returned hits — **with `ts_headline` snippets, which are page
content** — from every space on the instance. Both queries now carry
`AND space_id = ANY($scope)`, and the Rust version should make the scope a
parameter that cannot be omitted rather than a filter that can be forgotten.

## 11.2 BK-tree — "did you mean"

A metric tree over edit distance. Every node's children are bucketed by their
distance to it; a query at radius `r` only descends children in
`[d-r, d+r]`.

```rust,ignore
struct BkTree { root: Option<Node> }
struct Node { term: String, children: BTreeMap<u32, Node> }

impl BkTree {
    fn insert(&mut self, term: &str);
    fn search(&self, query: &str, radius: u32) -> Vec<(String, u32)>;
}
```

**Why this works: the triangle inequality.** Edit distance is a metric, so
`|d(q,n) - d(n,c)| <= d(q,c)`. That bound is what lets whole subtrees be
skipped, and it is the only reason the structure beats a linear scan.

**Invariant F11.4 — the index is instance-wide, so the FILTER is at query
time.** One BK-tree per member would be one index per person. The tree
therefore holds every title and each entry carries its page's space, and
`search` takes the caller's `Scope`. A suggestion *is* a title, so filtering
in the client is already too late.

The Go fix shipped with the space id left at its zero value in one of two
construction sites, and every suggestion was filtered out — a total outage
of the feature. **Test the allow as well as the deny**: a filter that
returns nothing to everybody passes every leak test ever written.

**Invariant F11.5 — the metric must actually be a metric.** Levenshtein is.
Damerau-Levenshtein with unrestricted transpositions is **not** (it violates
the triangle inequality), and using it silently makes the pruning wrong —
results just quietly go missing. If you want transpositions, use the
*optimal string alignment* variant and know that it is a metric only for
adjacent transpositions.

**Invariant F11.6 — results are sorted by distance, then lexicographically.**
Determinism (I0.6): a "did you mean" that reorders between identical queries
is one nobody trusts.

**Test list.**
1. `search_returns_every_term_within_the_radius` — checked against a **brute-
   force scan** over the same term set. **The hardest and the only one that
   matters**: this is the test that catches a wrong pruning bound, which
   otherwise manifests as "sometimes a word is missing".
2. `an_exact_match_has_distance_zero_and_sorts_first`
3. `insertion_order_does_not_change_the_result_set` (property)
4. `a_radius_of_zero_is_an_exact_lookup`

**DSA.** LeetCode: *[**closest**] 72. Edit Distance*, *712. Minimum ASCII
Delete Sum*, *1092. Shortest Common Supersequence*.

**After:** Lucene's fuzzy query uses a Levenshtein automaton, which is faster
and much harder to explain; the BK-tree was chosen because the pruning
argument fits in a paragraph and the structure is worth understanding once.

## 11.3 Trie — `[[` autocomplete

A prefix trie over page titles, compiled to wasm so keystroke latency is not
a round trip.

```rust,ignore
struct Trie { root: TrieNode }
struct TrieNode { children: BTreeMap<char, TrieNode>, ids: Vec<PageId> }

impl Trie {
    fn insert(&mut self, title: &str, id: PageId);
    fn complete(&self, prefix: &str, limit: usize) -> Vec<PageId>;
}
```

**`BTreeMap`, not `HashMap`.** Completions come out in a deterministic order
for free, which is I0.6 and matters when the list is drawn under a caret.

**Invariant F11.7 — matching is case-insensitive and the STORED key is
normalised, but the DISPLAYED title is the original.** Lowercasing on the way
in and rendering the lowercase form is the bug you ship on day one.

**Invariant F11.8 — completion is bounded.** A prefix of `""` must not walk
the whole trie. Cap the traversal, not just the result.

**DSA.** LeetCode: *208. Implement Trie*, *[**closest**] 642. Design Search
Autocomplete System*, *1268. Search Suggestions System*.

---

# Part 12 — Semantics: vectors and HNSW

`marginal-semantic`. Also pure, also no I/O, and the crate where Rust's
numerics are most obviously nicer than Go's.

## 12.1 The vectors, and the honest name for them

**Not neural embeddings.** Hashed, IDF-weighted term frequencies — the
hashing trick — at 256 dimensions, L2-normalised so a dot product *is* the
cosine.

```rust,ignore
const DIM: usize = 256;
struct Vector([f32; DIM]);

struct Corpus { doc_freq: HashMap<String, usize>, docs: usize }
impl Corpus {
    fn new(docs: &[Document]) -> Self;
    fn embed(&self, terms: &[String]) -> Vector;
    fn top_terms(&self, terms: &[String], n: usize) -> Vec<String>;
}
```

This captures **lexical** similarity: two pages using the same uncommon words
score high; "rope" and "cord" are unrelated to it. Say so on the screen. The
whole posture of `/discover` is that its figures can be checked, and the
first uncheckable claim would be about what it is measuring.

**Weighting, and why:**
- **Sublinear TF** (`1 + ln tf`) — a page saying "rope" forty times is about
  ropes, but not ten times more so than one saying it four times. Raw counts
  let long pages dominate every neighbourhood.
- **Smoothed IDF** (`ln(1 + N/df)`) — the textbook `ln(N/df)` is exactly 0
  for a term in every document, silently deleting that dimension, and
  undefined for `df = 0`.
- **No stemmer** — a wrong one merges terms that should not merge. IDF
  already makes "operation" and "operations" both rare and both informative.

**Invariant V12.1 — a zero vector stays zero, never NaN.** A page of pure
stop words normalises by dividing by zero if you are careless; every later
comparison becomes NaN, NaNs sort unpredictably, and a whole result list is
silently corrupted. `Vector::normalize` must check.

**Invariant V12.2 — `Corpus` owns IDF, so `embed` hangs off it.** IDF is a
property of the *collection*. A free `embed(text)` function is a signature
that cannot be correct.

## 12.1b How HNSW actually works

Read this before writing the struct. HNSW is easy to implement *nearly*
correctly, and a nearly-correct one returns plausible neighbours while
quietly having 60% recall.

**The problem.** Given a query vector, find the `k` nearest of `N` vectors
without comparing against all `N`.

**The idea, in three moves.**

1. **A navigable small world.** Build a graph where each vector is a node
   linked to some of its near neighbours. Greedy descent — repeatedly step
   to whichever neighbour is closer to the query — converges to a local
   minimum that is usually the true nearest neighbour, *if* the graph has
   both short-range links (precision) and long-range links (escape from
   local minima).

2. **Layers, for the long-range part.** Assign each node a maximum layer
   from a geometric distribution: layer 0 holds everything, layer 1 holds
   ~1/M of it, layer 2 ~1/M², and so on. Search starts at the top layer,
   greedily descends to the closest node there, drops a layer, repeats. The
   upper layers are a coarse map — a few long hops get you into the right
   region, and layer 0 does the fine-grained work. This is a skip list
   generalised to a metric space, and the analogy is exact enough to be
   worth keeping in mind.

3. **A beam, not a single path.** Pure greedy descent gets stuck. At layer 0
   the search keeps a candidate set of size `ef` (a min-heap by distance)
   and a result set of size `ef` (a max-heap), expanding the closest
   unexplored candidate until the closest remaining candidate is farther
   than the worst result. `ef` is the accuracy dial: larger `ef` costs more
   distance computations and finds more of the true neighbours.

**Insertion** does the same search to find `ef_construction` candidates at
each layer from the new node's top layer down, then connects the node to `M`
of them — and here is the part implementations get wrong.

**The heuristic neighbour selection.** Do *not* simply take the `M` closest
candidates. Doing so makes every node in a dense cluster link only to
others in that cluster, and the graph loses the long edges that make it
navigable. Instead, walk candidates nearest-first and keep a candidate `c`
only if it is closer to the new node than to **any already-selected
neighbour**:

```rust,ignore
fn select_neighbours(&self, base: &Vector, candidates: &[Candidate], m: usize) -> Vec<usize> {
    let mut chosen: Vec<usize> = Vec::with_capacity(m);
    for c in candidates.iter() {            // sorted nearest-first
        if chosen.len() >= m { break }
        // Keep it only if it is not "dominated" by something already chosen:
        // if some chosen neighbour is closer to c than base is, then c is
        // reachable through that neighbour and this edge is redundant.
        let dominated = chosen.iter().any(|&q| dist(&self.nodes[q].vec, &c.vec) < c.dist);
        if !dominated { chosen.push(c.idx) }
    }
    chosen
}
```

This is what preserves diversity of direction, and it is the difference
between 95% recall and 60%.

**Bidirectional linking, and pruning.** After connecting the new node to its
chosen neighbours, add the reverse edges. That can push a neighbour over
`Mmax` (`Mmax0` at layer 0, conventionally `2M`), so re-run the same
heuristic on that neighbour's edge list to prune it back. Skipping this step
makes hub nodes accumulate hundreds of edges and search time degrade toward
linear.

**Filtering.** When the caller restricts to a topic or a tag, filter **during
descent**, not afterwards. Post-filtering asks for `k` and then discards
most of them, so the answer is short or empty; filter-during-descent keeps
walking until it has `k` that pass. The cost is that a very selective filter
can make the graph effectively disconnected — so bound the work and say when
you gave up.

**Complexity.** Search is `O(log N)` hops with `ef` distance computations
per layer, in practice a few hundred comparisons for `N` in the millions.
Build is `N` insertions each costing a search.

**Measure recall on every query.** The crate runs a brute-force scan
alongside and reports recall@k. That is the entire reason to trust the
screen, and it costs `O(N)` on a corpus of this size — trivial. Keep it, and
keep it *on*, not behind a debug flag:

```rust,ignore
pub struct SearchResult {
    pub neighbours: Vec<Neighbour>,
    /// Measured against a brute-force scan, this query, right now.
    pub recall_at_k: f32,
}
```

**Rust-specific notes.**
- Seed the layer-assignment RNG explicitly (`Lcg` in the Go version). An
  index that differs run to run is untestable.
- `Vec<Vec<usize>>` for per-layer neighbours is fine; do not reach for an
  arena until a profile says so.
- The distance function is a dot product over `[f32; 256]`. Write it as a
  plain loop and let LLVM autovectorise; check the assembly before hand-
  writing SIMD, and if you do, keep the scalar version as the test oracle.
- **`f32` comparison in a heap needs a total order.** `f32` is not `Ord`.
  Use `ordered_float::NotNan` or compare with `partial_cmp().unwrap()` only
  after V12.1 guarantees no NaN can exist.

## 12.2 HNSW

```rust,ignore
struct Index {
    m: usize, mmax0: usize, ef_construction: usize,
    nodes: Vec<Node>, by_id: HashMap<String, usize>,
    entry: Option<usize>, max_layer: usize, rng: Lcg,
}
struct Node { id: String, vec: Vector, neighbours: Vec<Vec<usize>> }

impl Index {
    fn add(&mut self, id: &str, v: Vector);
    fn search(&self, q: &Vector, k: usize, ef: usize,
              exclude: Option<&str>, allow: Option<&dyn Fn(&str) -> bool>)
        -> (Vec<Result_>, SearchStats);
    fn exact(&self, q: &Vector, k: usize, ...) -> Vec<Result_>;   // brute force
    fn layer_sizes(&self) -> Vec<usize>;
}
```

**The idea in one paragraph.** Every element is on layer 0. Each is also
promoted to a random number of higher layers, with `P(level >= l)` falling
off exponentially — so the top layer holds a handful spread across the whole
space and each layer down is denser. A search starts at the single entry
point on top, greedily walks to the closest element it can see, drops a
layer, repeats. The upper layers are a coarse map; layer 0 refines. **It is a
skip list in a metric space.**

**Invariant H12.1 — `mL = 1/ln(M)`.** Not arbitrary: it is the value that
makes the expected number of elements examined per layer roughly constant,
which is what makes the structure logarithmic rather than merely
hierarchical.

**Invariant H12.2 — the level draw is SEEDED.** Two builds over identical
input must answer identically, or the recall figure on screen cannot be
compared with the one from a minute ago.

**Invariant H12.3 — neighbour selection is the HEURISTIC, not "keep the M
closest".** This is the thing naive implementations get wrong. Keeping the
closest fills a node's list from one dense cluster and the graph loses every
long edge — which is what made it navigable. The heuristic keeps a candidate
only if it is closer to the query than to any neighbour already kept, so a
distant element in an unrepresented direction beats a nearer duplicate.

**Invariant H12.4 — the filter rides the descent.** Applied inside the layer
search, never to the result list. Post-filtering asks for k=5, throws three
away and ships two, and **recall collapses exactly when the filter is narrow
— which is when someone is relying on it**. Excluded elements are still
traversed (they are the roads) and never kept.

**Invariant H12.5 — recall is measured, not asserted.** Run the exact scan
beside every query. An approximate index that never shows its recall is an
index asking to be trusted.

**Invariant H12.6 — report the comparison count against the exact count, even
when it is unflattering.** At twenty pages the tower is one layer and HNSW
buys nothing; the screen says so. A structure has to justify itself at the
size it is actually running at, not the size its paper was written for.

**The two queues.** `search_layer` needs a min-heap of candidates to expand
and a max-heap of results to keep. In Rust, `BinaryHeap` with `Reverse` for
one of them, and a tie-break on node index in `Ord` so results are
deterministic.

**Test list.**
1. `search_matches_brute_force_on_a_small_corpus` — recall 1.0, same order.
   **The one that matters.**
2. `a_narrow_filter_still_returns_k_results` — only every 7th element
   eligible; a post-filter returns 0–1. **The hardest**, and the direct test
   for H12.4.
3. `the_index_is_deterministic_across_builds` (H12.2)
4. `layer_sizes_shrink_going_up` — a tower that does not narrow is a set of
   duplicate flat graphs
5. `search_never_returns_the_query_itself`
6. `an_empty_index_answers_nothing_rather_than_panicking`
7. `a_zero_vector_never_produces_nan` (V12.1)

**Before:** Malkov & Yashunin, "Efficient and robust approximate nearest
neighbor search using HNSW" — §4 (the algorithm) and §4.2 (the heuristic).
Read the heuristic section twice; it is the part everyone skips.

**DSA.** Skip lists; k-NN; priority queues. LeetCode: *[**closest**] 973. K
Closest Points to Origin*, *1206. Design Skiplist*, *703. Kth Largest Element
in a Stream*.

**After:** `hnswlib`, FAISS and `qdrant` all implement this; Postgres has
`pgvector` with both HNSW and IVFFlat. The spec implements it because the
algorithm is the thing being learned — and because the recall-beside-every-
query property is much easier to keep when the index is yours.

---

# Part 13 — Diff, history, and the palimpsest

## 13.1 LCS by the full DP table

`marginal-textdiff`. Deliberately the O(n·m) table, not Myers.

```rust,ignore
fn lcs_table(a: &[Token], b: &[Token]) -> Vec<Vec<u32>>;
fn traceback(table: &[Vec<u32>], a: &[Token], b: &[Token]) -> Vec<DiffOp>;

enum DiffOp { Equal(Range), Delete(Range), Insert(Range) }
```

**Why the full table.** The screen exposes it. The point of `/lab/diff` is to
show the DP table with the traceback outlined, and Myers' O(nd) has no table
to show. **The screen also makes the counter-argument**: O(n·m) is fine for a
block and absurd for a document, and the page says so rather than pretending
the choice was free.

**Invariant T13.1 — tokenisation is the caller's job.** Word-level and
character-level diffs are the same algorithm over different token streams.
Splitting text into tokens is the only non-Go code in `web/src/diff-core/` —
and that is the boundary: the *split* is a view concern, the *diff* is not.

**Invariant T13.2 — the traceback is deterministic.** At a tie in the table,
always prefer the same direction. Otherwise two runs produce two different
(both valid) minimal diffs, and the screen flickers.

**DSA.** LeetCode: *1143. Longest Common Subsequence*, *[**closest**] 583.
Delete Operation for Two Strings*, *72. Edit Distance*.

## 13.2 The palimpsest — one array, every version

The claim: **history costs storage, never time, and never a second copy of
the text.**

```rust,ignore
struct Char {
    id: CharId,
    ch: char,
    insert_step: u64,
    insert_actor: ActorId,
    delete_step: Option<u64>,      // None = still live
    delete_actor: Option<ActorId>,
}
struct Palimpsest { chars: Vec<Char> }   // insertion order, never compacted
```

**Reading version `v` is the filter `insert_step <= v < delete_step`.** That
is the whole mechanism. There is no snapshot per revision, no copy, no
reconstruction — scrubbing the version slider is a predicate over one array.

**Invariant P13.1 — a delete writes a stamp and never removes.** This is
what makes A7.1 true (anchors keep resolving), what makes the scrubber free,
and why `COPIES` reads 0 on § 17.

**Invariant P13.2 — the array is never compacted while any anchor could
reference a tombstone.** Which, given the log is permanent, is never. Garbage
collection here needs a causal-stability argument (every replica has seen
every op that could reference the tombstone) and is out of scope; state that
rather than leaving a TODO.

**Invariant P13.3 — `TEXT` and `PALIMPSEST` are two RENDERINGS of one filter,
not two stored forms.** § 17's toggle draws the same array with tombstones
shown or hidden.

**Test list.**
1. `reading_version_v_equals_replaying_ops_up_to_v` — **the hardest and the
   central claim**: the filter and a full replay must agree, for every `v`,
   over a property-generated op sequence.
2. `a_delete_never_shortens_the_array`
3. `an_anchor_into_a_tombstone_still_resolves` (A7.1, from the other side)
4. `stored_minus_live_equals_tombstoned` (the three readouts on § 17)

**Before:** *DDIA* ch. 3 (Storage and Retrieval) on log-structured storage —
the tombstone-and-filter shape is the same idea as an LSM's deletion marker.

**After:** Yjs and Automerge both keep tombstones for exactly this reason and
both have a garbage-collection story gated on causal stability. The spec has
neither, deliberately, at this scope.

---

# Part 14 — Diagnostics and the fact DAG

`RFC-003`. `diagnostics-service` is **stateless**, has no database, no NATS,
and is a gRPC client of `document-service`. It exists as a separate service
for exactly one reason: *it can die without touching editing*, and a service
sharing `document-service`'s deployable could not tell that story.

## 14.1 The analyzers

Nine, pure, over one page's block tree.

```rust,ignore
trait Analyzer {
    fn id(&self) -> &'static str;
    fn analyze(&self, page: &Page, symbols: &SymbolTable) -> Vec<Diagnostic>;
}

struct Diagnostic {
    analyzer: &'static str,
    severity: Severity,          // error | warning | info
    block: BlockId,
    range: Option<AnchorRange>,  // an ANCHOR, so it survives an edit
    message: String,
    fix: Option<Fix>,            // e.g. CreatePage { title }
}
```

**Invariant N14.1 — a diagnostic anchors, it does not carry an offset.** Open
it a minute later, after three edits, and it still points at the right words.

**Invariant N14.2 — analyzers are pure and independent.** No analyzer reads
another's output. Composition happens in the caller, so any one of them can
be disabled without a cascade.

**Invariant N14.3 — a diagnostic that cannot be acted on should not exist.**
Every one either has a `Fix` or names the thing to look at. "Consider
rewriting this" is noise wearing a badge.

## 14.2 The fact DAG

Facts (`{{name}}`) are defined on one page and transcluded on others.
Changing a definition makes every transclusion **stale**, and "what goes
stale when this changes" is a reachability query.

**This reuses `graphalgo` directly** — `detect_cycle` and `forward_reachable`
over a graph whose `NodeId` is a fact name rather than a page id. That is why
`NodeId` is a plain `String` and not a page-specific type: the same algorithm
answers both questions, and a second copy specialised to facts would be a
second implementation of one thing.

**Invariant N14.4 — a fact cycle is an error the UI reports, not a hang.**
`A` defined in terms of `B` defined in terms of `A` must be detected before
evaluation, by the same three-colour DFS.

**Invariant N14.5 — staleness is computed, never stored.** Storing it makes
a cache that must be invalidated by exactly the thing that already computes
the answer.

---

# Part 15 — Async, concurrency, and backpressure

## 15.1 The shape of each service

| Service | Concurrency shape |
|---|---|
| `document-service` | stateless; `tokio` + `tonic`, one task per request, a `PgPool` |
| `auth-service` | same |
| `diagnostics-service` | same, plus a gRPC client pool |
| `notification-service` | a NATS subscriber task + an axum server |
| `api-gateway` | pure fan-out; `axum` → `tonic` clients |
| `collaboration-service` | **one actor task per page**, one task per connection, plus WAL and flush tasks (Part 8) |

## 15.2 Rules

**A15.1 — every unbounded channel is a bug.** Bounded, always, with a
documented policy for what happens when it fills. `collaboration-service`'s
answer is 8.5: drop the subscriber, let it reconnect, it gets a fresh
snapshot.

**A15.2 — no blocking call in an async task.** `spawn_blocking` for anything
CPU-bound that could exceed a few hundred microseconds. The candidates here
are real: HNSW index construction, the LCS table, and Betti's GF(2)
elimination.

**A15.3 — every spawned task is owned.** A `JoinSet` or a stored
`JoinHandle`, cancelled when the owner drops. Go's equivalent discipline was
`goleak` in tests; in Rust, structure it so leaking is not expressible, and
assert the set drains.

**A15.4 — cancellation safety at every `select!`.** A future dropped mid-poll
must not have half-applied an op. This is the single sharpest edge in porting
a Go server: Go's goroutines are not cancelled at arbitrary await points, and
Rust's futures are. Any `select!` arm that mutates shared state must complete
its mutation synchronously after the await, never across one.

**A15.5 — shutdown is graceful and bounded.** Stop accepting, drain the WAL,
flush, then exit. With a timeout, after which exit anyway — an unbounded
drain is a service that cannot be restarted.

## 15.3 Cancellation safety, in detail

A15.4 deserves its own section, because it is the rule with no Go equivalent
and the one that produces bugs that survive code review.

**What actually happens.** A Rust future makes progress only when polled. If
the thing polling it stops — a `select!` branch loses the race, a timeout
fires, the caller drops the future — the future is **dropped where it
stands**, at its last `.await`. Everything after that point never runs.

Go has no analogue. A goroutine blocked on a channel receive inside
`select` is not unwound when another case wins; it simply stays blocked
until it is cancelled explicitly, and the code after the receive still runs
when it eventually returns.

**The dangerous shape:**

```rust,ignore
// WRONG. If `shutdown` wins, `apply` has already mutated the session
// but `journal` never runs, and the WAL is now behind the memory state.
loop {
    tokio::select! {
        Some(op) = rx.recv() => {
            session.apply(op).await;          // ← mutation
            wal.journal(&op).await;           // ← may never run
        }
        _ = shutdown.cancelled() => break,
    }
}
```

**The fix is structural, not careful.** Do the awaits *before* the mutation,
and make the mutation synchronous:

```rust,ignore
loop {
    tokio::select! {
        Some(op) = rx.recv() => {
            // Journal first: an op in the WAL that was never applied is
            // recoverable by replay; an op applied but never journalled is
            // lost state. Order the failure so the survivable one is
            // possible and the unsurvivable one is not.
            wal.journal(&op).await?;
            session.apply(op);                 // ← no .await, cannot be torn
        }
        _ = shutdown.cancelled() => break,
    }
}
```

**The rule to write on the wall:** *between the first mutation of shared
state and the end of the operation, there must be no `.await`.* If you
cannot arrange that, hold the state in a task that owns it and send it a
message instead (15.4).

**Which primitives are cancellation-safe** matters and is documented per
API. Safe: `tokio::sync::mpsc::Receiver::recv`, `broadcast::recv`,
`Notify::notified`. **Not** safe in general: anything you wrote yourself
that mutates before awaiting, and `AsyncReadExt::read_exact` (it can consume
bytes and then be dropped, losing them). When in doubt, wrap the operation
in `tokio::spawn` and await the `JoinHandle` — a spawned task is not
cancelled by the caller's drop, which converts a correctness problem into a
resource problem you can bound.

## 15.4 Shared mutable state: the three options, and when each is right

This decision recurs throughout the port, so make it deliberately.

**Option A — `Arc<Mutex<T>>`.** Simple, and correct when the critical
section is short and does no I/O.

```rust,ignore
// Fine: a pure in-memory lookup.
let dir = Arc::new(Mutex::new(RoleDirectory::default()));
```

Use `parking_lot::Mutex` or `std::sync::Mutex` for these, **not**
`tokio::sync::Mutex` — an async mutex is for holding a lock across an
`.await`, which the previous section just told you not to do. If you find
yourself needing `tokio::sync::Mutex`, that is usually the signal to use
Option C.

**Option B — `RwLock`, only with evidence.** Reads must genuinely dominate
and the critical section must be long enough for the extra bookkeeping to
pay. For a `HashMap` lookup it is usually slower than a `Mutex`. Measure.

**Option C — an actor task that owns the state.** No lock at all: one task
owns the value, everyone else sends messages.

```rust,ignore
enum SessionMsg {
    Apply { op: Op, reply: oneshot::Sender<Result<Ack, DomainError>> },
    Snapshot { reply: oneshot::Sender<Snapshot> },
    Close,
}

// One task per page. The Page is not Send-shared; it is OWNED here.
tokio::spawn(async move {
    let mut page = Page::new();
    while let Some(msg) = rx.recv().await { /* ... */ }
});
```

**This is the right shape for `collaboration-service`'s sessions**, and it is
a better fit than the Go original's `Manager` + mutex, because:

- The invariant "one writer per page" becomes structural rather than
  conventional.
- Ops for one page are serialised by the channel, which is exactly the
  ordering the op log needs.
- `Page` never has to be `Sync`, so it can hold `Rc`-flavoured internals if
  the rope wants them.
- Backpressure is expressible: a bounded channel that is full is a client
  that is sending faster than the document can absorb, and that is
  information, not an error.

The cost is that every read becomes a message round-trip. For the snapshot
path that is fine; for a hot read you would keep an `Arc<ArcSwap<Snapshot>>`
the actor publishes into.

## 15.5 Backpressure, concretely

Three queues in this system, three policies, and each must be *chosen*:

| Queue | Bound | When full |
|---|---|---|
| Per-connection outbound broadcast | 64 messages | **Drop the subscriber.** It reconnects and gets a fresh snapshot — cheaper and more correct than buffering unboundedly for a client that has stalled |
| Per-page inbound ops | 256 | Apply backpressure to the socket read: stop reading, let TCP's window do the work |
| Outbox poller batch | 100 rows/tick | Not a queue — a claim size. Tune by measuring lag |

**The general rule: never buffer on behalf of a consumer that is not
keeping up.** Either drop with a defined recovery (the first row) or push
back to the producer (the second). Growing a buffer converts a slow consumer
into an OOM.

## 15.6 The `Send` bound problem

`#[tonic::async_trait]` requires futures to be `Send`, which means every
value held across an `.await` in a handler must be `Send`. This bites in two
predictable places:

1. **`Rc` or `RefCell` in a domain type.** Keep them out of anything a
   handler touches; use them only inside a single-threaded actor
   (`tokio::task::spawn_local` on a `LocalSet`) or not at all.
2. **`MutexGuard` held across an await.** The compiler error names the
   guard, and the fix is always the same: end the scope before the await.

```rust,ignore
// The idiom that fixes it, and reads better anyway:
let role = { dir.lock().role_for(page, actor) };   // guard dropped here
send_ack(role).await;
```

## 15.7 Testing concurrency

- `tokio::test` with `start_paused = true` for anything time-dependent —
  it makes `sleep` instantaneous and deterministic, which is the only way
  to test a poller's cadence without a slow test.
- `loom` for the genuinely lock-free parts. There are few here; do not use
  it where a `Mutex` is the design.
- **The Go project ran `-race` and `goleak` on every concurrent test.** The
  Rust equivalents are: the type system for the first (mostly), and for the
  second, a `JoinSet` whose `is_empty()` you assert after shutdown.
- Property-test the transform (Part 27.1) with thousands of seeded
  interleavings. That is where concurrency bugs in this system actually
  live — not in the locking, but in the merge.

**Before:** *Rust Atomics and Locks* ch. 1–3; *Crust of Rust: "Channels"* and
*"Atomics"*. For A15.4 specifically, read the `tokio::select!` documentation
on cancellation safety in full — it is short and it is load-bearing.

---

# Part 16 — Errors, and the Rust idiom table

## 16.1 The rule

**Libraries: `thiserror`, one enum per module, variants that name the
failure.** **Binaries: `anyhow`, with `.context()` at each layer boundary.**
A library that returns `anyhow::Error` has told its caller nothing.

At the gRPC boundary, one function maps the domain error to a
`tonic::Status`. One place, not scattered `map_err` calls — the same
chokepoint argument as R6.7.

## 16.2 The idiom table

| Go | Rust | Note |
|---|---|---|
| `error` return | `Result<T, E>` | |
| `errors.Is` | `matches!` on the enum | Nicer: the variants are exhaustive |
| `errors.As` | pattern match | |
| `fmt.Errorf("...: %w", err)` | `#[from]` / `.context()` | |
| zero value on missing map key | `Option<T>` | **The biggest correctness win.** See 10.4 |
| `interface{}` / `any` | a generic or an enum | Almost never `Box<dyn Any>` |
| small interface at point of use | a trait, generic bound | Not `dyn` unless you actually swap at runtime |
| `sync.Mutex` guarding a struct | `Mutex<T>` guarding the data | Rust names *what* is guarded; Go only names the lock |
| `context.Context` cancellation | `CancellationToken` / drop | |
| `context.Context` deadline | `tokio::time::timeout` | |
| goroutine + channel | task + `mpsc` | |
| `defer` | `Drop` | |
| struct tags (`json:"x"`) | `#[serde(rename = "x")]` | |
| tagged union by `Tag string` field | `enum` + `#[serde(tag)]` | The single largest type-safety gain in the port |
| `nil` slice vs empty slice | `Vec::new()` | The distinction disappears; **check every JSON boundary that relied on it** — several converters ship `[]` rather than `null` on purpose |
| table-driven test | `#[test]` + a loop, or `rstest` | |

**PORT-NOTE, carried from the Go source:** wherever a Go call site resolves an
interface at compile time and never swaps implementations at runtime, the
Rust version should be a **generic bound**, not `dyn Trait`. `notify.Repo` is
the worked example: one production impl, one fake, both known statically. Go
pays an allocation and an indirection for that under the GC; Rust need not
inherit the cost just because the Go version looks like an interface.

---

# Part 17 — The wasm boundary

`ADR-004`'s boundary, unchanged. Four entrypoints today:
`documentcore` (the editor core), `graphalgo` (layout, Voronoi, hulls,
spatial majority), `textdiff` (LCS), and the trie (`[[` autocomplete).

## 17.1 The contract, which is what is stable

**JSON string in, JSON string out**, every function, with exactly one of
`{value, error}` set. The TypeScript side checks `.error` before touching
`.value`; there is no exception to catch across the boundary.

```rust,ignore
#[wasm_bindgen]
pub fn graph_layout_tick(req_json: &str) -> String;   // serialises {value|error}
```

**Invariant W17.1 — the JSON shape is the same shape a network call would
use.** The view layer must not be able to tell "local wasm call" from
"network call". This is what allows an algorithm to move server-side later
without touching a screen.

**Invariant W17.2 — nothing crosses the boundary except JSON.** No shared
memory views, no borrowed slices. It costs a serialisation per call and buys
a boundary that cannot rot.

**Invariant W17.3 — every wasm entrypoint lives in one crate by
convention**, even when the algorithm belongs to another (`textdiff` has
nothing to do with pages, and its entrypoint still lives beside the others).
One place to look for "what does the browser run".

## 17.2 What changes from Go

Go's `syscall/js` needs `main()` to block in `select {}` and installs
functions on `globalThis`. `wasm-bindgen` exports directly and has no
runtime to keep alive. **Rewrite the entrypoints; do not port them.** Also:
the Go wasm binaries are ~3.5 MB; `wasm-bindgen` output plus `wasm-opt`
should be an order of magnitude smaller, which is worth measuring and
recording in `BENCHMARKS.md`.

**Invariant W17.4 — the TypeScript bridge files change only in their loader.**
`web/src/*-core/wasm.ts` should keep the same exported functions with the
same signatures. If a screen needs editing to accommodate the Rust build,
the boundary was not the boundary.

## 17.3 The nine modules, and what each one costs

The Go build ships nine wasm modules totalling roughly 28 MB uncompressed.
That is the single worst number in the project, and it is a straightforward
consequence of Go's runtime being linked into every one of them.

| Module | Crate | What the browser needs it for |
|---|---|---|
| `documentcore` | `documentcore` | the editor core — apply, invert, history |
| `graphwasm` | `graphalgo` | seeded layout, Voronoi, hulls, territories |
| `diffwasm` | `textdiff` | LCS table + traceback for the revision diff |
| `triewasm` | `trie` | `[[` autocomplete |
| `syntaxwasm` | `syntax` | code-block highlighting, nine languages |
| `sketchwasm` | `sketch` | HLL / Count–Min / t-digest, recomputed per keystroke |
| `netsimwasm` | `netsim` | the OT simulation and its four lenses |
| `benchwasm` | `bench` | the in-browser benchmark |
| `mdcwasm` | `mdc` | the markdown compiler |

In Rust each of these is a `cdylib` with `wasm-bindgen`, and there is no
runtime to carry. Expect single-digit megabytes total, and **record the
before/after in `BENCHMARKS.md`** — this is one of the few places the port
produces a headline number.

```toml
[lib]
crate-type = ["cdylib", "rlib"]     # rlib so the native tests still compile

[profile.release]
opt-level = "z"
lto = true
codegen-units = 1
panic = "abort"                     # no unwinding tables in the browser
strip = true
```

Then `wasm-opt -Oz` on the output. `wasm-pack build --target web` does most
of this for you; the manual `wasm-bindgen` + `wasm-opt` path is worth
knowing because it is what a build script ends up doing.

## 17.4 Panics, and why `panic = "abort"` needs a decision

With `panic = "abort"`, a Rust panic in wasm traps and the module is
**poisoned** — every subsequent call fails. That is worse than the Go
behaviour, where a panicking goroutine could be recovered.

So: **no panics across the boundary.** Every exported function returns the
`{value, error}` envelope, and the internal code returns `Result`. The
entrypoint is the place where `Result` becomes JSON:

```rust,ignore
#[wasm_bindgen]
pub fn diff_lines(req_json: &str) -> String {
    match run(req_json) {
        Ok(v)  => serde_json::json!({ "value": v }).to_string(),
        Err(e) => serde_json::json!({ "error": e.to_string() }).to_string(),
    }
}

fn run(req_json: &str) -> anyhow::Result<DiffResponse> { /* no unwrap, no expect */ }
```

Add `console_error_panic_hook` in debug builds only — it makes a
development panic legible, and it is dead weight in release.

**Grep the wasm crates for `unwrap`, `expect`, indexing, and integer
division before shipping.** A slice index out of range is a panic, and in
wasm a panic is an outage of that feature until the page reloads.

## 17.5 What wasm cannot do, and where that bites here

- **No threads** (without `SharedArrayBuffer` and cross-origin isolation,
  which this app does not have). Everything is single-threaded; do not
  reach for `rayon` in a crate that must compile to wasm.
- **No sockets, no filesystem.** Which is fine: nothing in these nine crates
  does I/O, by design (19.2). If one starts to, the wasm build breaks first
  and the error will be confusing.
- **No sampling profiler.** Which is why `bench` walks instrumented spans
  rather than sampling (Part 28.3).
- **A coarsened clock.** `performance.now()` is quantised, so single
  operations cannot be timed directly — hence batch calibration (28.2).
- **`std::time::Instant` panics on `wasm32-unknown-unknown`.** Use
  `web_sys::window().performance()` behind a `#[cfg(target_arch = "wasm32")]`
  shim, and keep the native path for tests. This is the single most common
  "it compiled and then exploded in the browser" cause.

```rust,ignore
#[cfg(target_arch = "wasm32")]
fn now_ms() -> f64 { web_sys::window().unwrap().performance().unwrap().now() }
#[cfg(not(target_arch = "wasm32"))]
fn now_ms() -> f64 { std::time::Instant::now().elapsed().as_secs_f64() * 1000.0 }
```

## 17.6 Testing the wasm crates

Test them **natively**. `cargo test` on `x86_64` exercises the same code, is
faster, and gives you a debugger. Use `wasm-bindgen-test` only for the thin
entrypoint layer — the JSON envelope, the error path — and run it in
headless Chrome in CI.

The harness already checks the thing that matters most, and the port
inherits it: `verify.js` asserts that **every module the SPA loads is really
wasm** (nine checked). A screen that silently fell back to a JavaScript
reimplementation would pass every visual diff, and that check is what makes
"the algorithm is Go/Rust, never a second implementation in TypeScript" an
enforced rule rather than an aspiration.

---

# Part 18 — Testing strategy and the order of work

## 18.1 The testing rules

1. **Never mock infrastructure.** Integration tests hit real Postgres, real
   NATS, real Redis via `testcontainers-rs`.
2. **Property tests where a law exists.** Invertibility (I0.1), replay
   equality (I0.2), convergence (Part 8), anchor resolution (Part 7), LCS
   minimality, BK-tree completeness. `proptest`, and shrink the failures.
3. **Golden vectors where a format exists.** `testdata/document-core/*.json`
   already works this way and is the model. Extend it: the highest-value
   extraction is the CRDT core's scenarios.
4. **`-race` has no equivalent, and needs none** for data races — but
   `loom` is the equivalent for the *logic* races in Part 8, and `miri` for
   any unsafe (there should be none).
5. **Fuzz the parser.** `cargo-fuzz` over `parse()`, with the two invariants
   P5.2 (always advances) and P5.5 (bounded nesting) as the oracle. This is
   the one place untrusted input reaches an algorithm.

## 18.2 The order

Bottom-up, because every step is verifiable against the running Go system.

**Phase 1 — the pure crates (no I/O, no async, immediately testable).**
1. `graphalgo` — the easiest, the most self-contained, and the one whose
   output the UI can be diffed against immediately.
2. `textdiff` — smallest.
3. `semantic` — self-contained; recall against brute force is its own oracle.
4. `documentcore` — the hardest and the most important. Golden vectors exist.

**Phase 2 — the boundary.**
5. The wasm entrypoints. **At this point the frontend runs half on Rust**,
   and `tools/uidiff` tells you whether it worked, screen by screen.

**Phase 3 — the stateless services.**
6. `document-service` (pages, then the projection, then the saga).
7. `auth-service`, `notification-service`.
8. `api-gateway` — trivial once the protos are compiled.

**Phase 4 — the stateful one.**
9. `collaboration-service`. Last, deliberately: it is the only one with
   in-memory state, the only one with a WAL, and the one where every earlier
   crate's invariants get exercised at once. Attempting it first is the
   classic way to spend three weeks debugging a `documentcore` bug through a
   WebSocket.

**Phase 5 — `diagnostics-service`**, which depends on `document-service`'s
gRPC surface being stable.

## 18.3 How you know a module is done

Not "the tests pass". **The screen it feeds renders identically.**

```
node tools/uidiff/uidiff.js <section> <route>
```

against the Rust backend, with `missing 0`. That is the acceptance bar the Go
implementation is already held to, and it is the reason the frontend is never
ported: it is the oracle.

Plus the second half of that gate: **click every control the screen draws.**
uidiff compares markup and computed style, so a control that renders and does
nothing passes it.

## 18.4 What to write down as you go

`docs/porting/PROGRESS.md`'s discipline, continued in the new repo: every
decision, dated, with the argument. Especially the ones where the Rust
version *diverges* — every divergence is either an improvement worth naming
or a bug worth catching, and six months later you cannot tell which from the
code alone.

---

# Part 19 — Microservices: the parts that are not the algorithm

Parts 3–14 are the interesting half of the port. This part is the half that
decides whether the interesting half ever runs. It is deliberately blunt:
almost none of it is a design question, and treating it as one is how a port
spends three weeks on a service registry it does not need.

## 19.1 What a service is here, and what it is not

`ADR-001`'s rule, unchanged by the port: **a service exists only if it
differs in scaling profile, state, failure mode, or deploy cadence.** Owning
a different noun is not sufficient. Six services survive that test:

| Service | Why it is separate | Port difficulty |
|---|---|---|
| `document-service` | Stateless, read-heavy, owns `docs` | Medium — most surface area |
| `collaboration-service` | **Stateful.** Scales on connection count, not request rate | **Hardest.** Part 8 |
| `auth-service` | Distinct security surface; different deploy cadence | Medium |
| `notification-service` | Fails independently; nothing degrades if it dies | Easy — do this first |
| `diagnostics-service` | Can die without touching editing | Easy, and no database |
| `api-gateway` | The only process that knows where everything is | Easy, but do it early |

The one that matters: **`collaboration-service` is the only stateful one.**
Everything else can be killed and restarted mid-request and lose nothing but
that request. That asymmetry should be visible in the Rust code — it is the
only crate that owns a long-lived in-memory structure keyed by document, and
the only one where "which instance handles this connection" is a question.

## 19.2 One binary per service, one crate per binary

```
crates/
  document-service/     src/main.rs  + src/{pages,blocks,graph,search,discover}/
  collaboration-service/
  auth-service/
  notification-service/
  diagnostics-service/
  api-gateway/
  documentcore/         lib only
  graphalgo/  textdiff/  semantic/  sketch/  syntax/  netsim/  bench/
```

The library crates have no `main.rs` and no `tokio` dependency. That is not
tidiness — it is what keeps them compilable to wasm (Part 17). The moment
`graphalgo` grows a `tokio::spawn`, the browser build dies, and the error
will be about `mio` rather than about the mistake.

**Enforce it in `Cargo.toml`, not by intention:**

```toml
# crates/graphalgo/Cargo.toml
[dependencies]
serde = { version = "1", features = ["derive"] }
# No tokio. No sqlx. No tonic. If you need one of those here,
# the code is in the wrong crate.
```

## 19.3 Configuration

Go has `envconfig` (`EnvOr`, `RequiredEnv`). Rust should **delete it** and
use `figment` or plain `envy` with a `#[derive(Deserialize)]` struct per
service:

```rust
#[derive(Debug, serde::Deserialize)]
pub struct Config {
    pub database_url: String,
    #[serde(default = "default_grpc_addr")]
    pub grpc_addr: SocketAddr,
    #[serde(default = "default_http_addr")]
    pub http_addr: SocketAddr,
    pub nats_url: Option<String>,
    #[serde(default = "default_jwks_url")]
    pub auth_jwks_url: String,
}

fn default_grpc_addr() -> SocketAddr { "0.0.0.0:9001".parse().unwrap() }
```

Why a struct rather than scattered `env::var` calls: **the service fails at
startup with one message naming every missing variable**, instead of failing
on the first request that happens to touch the third one. The Go version
reads them one at a time in `run()` and is slightly worse for it.

Secrets come from the environment only. Never a file in the repo, never a
default. `database_url` has no default on purpose: a service that silently
starts against `localhost` in production is worse than one that refuses.

## 19.4 Startup order and graceful shutdown

Every `main.rs` has the same shape. Write it once, copy it five times —
this is TEDIUM-RULE territory, not a design exercise:

```rust
#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt().with_env_filter(EnvFilter::from_default_env()).init();
    let cfg: Config = envy::from_env().context("reading configuration")?;

    // 1. Migrations BEFORE anything serves. A service that accepts a
    //    request against an un-migrated schema returns a confusing 500
    //    rather than failing to start, which is the wrong failure.
    let pool = PgPoolOptions::new().max_connections(16).connect(&cfg.database_url).await?;
    sqlx::migrate!("./migrations").run(&pool).await.context("migrating")?;

    // 2. Long-lived background work, each with its own cancellation.
    let shutdown = CancellationToken::new();
    let outbox = tokio::spawn(outbox::run(pool.clone(), nats.clone(), shutdown.clone()));

    // 3. Servers.
    let grpc = tonic::transport::Server::builder()
        .add_service(PageServiceServer::new(pages))
        .serve_with_shutdown(cfg.grpc_addr, shutdown.clone().cancelled_owned());
    let http = axum::serve(listener, health_router())
        .with_graceful_shutdown(shutdown.clone().cancelled_owned());

    // 4. One signal handler cancels everything.
    tokio::select! {
        r = grpc => r?,
        r = http => r?,
        _ = signal::ctrl_c() => shutdown.cancel(),
    }
    outbox.await??;
    Ok(())
}
```

**The ordering is load-bearing.** Migrations before listeners; listeners
before the process reports healthy; cancellation token shared by everything
so one `Ctrl-C` unwinds all of it.

`collaboration-service` has one extra step, and it is the one that will bite:
**flush every open session's WAL before the process exits.** The Go version
does this in a `defer manager.CloseAll()`. In Rust it belongs *after* the
`select!`, not in a `Drop` impl — `Drop` cannot be async, and the flush needs
the pool.

## 19.5 Health probes, and what "healthy" is allowed to mean

Two endpoints, both plain HTTP, never gRPC:

- `GET /health` — the process is up and can serve. **Does not touch the
  database.** A liveness probe that checks Postgres restarts every service in
  the cluster when Postgres blips, which converts a degraded system into an
  outage.
- `GET /ready` — the dependencies this service cannot work without are
  reachable. This one *may* touch the database.

The Go version has only `/health` and returns `ok` unconditionally. That is
honest at this repo's scale and the port may keep it, but the distinction
above is worth writing down before somebody "improves" the probe.

`api-gateway`'s `GET /admin/health` is a different thing entirely: it fans
out to every service's probe concurrently and reports up/down/timeout with
latency. In Rust that is a `futures::future::join_all` over a `Vec<Future>`
with a per-probe `tokio::time::timeout`, and **the timeout must be shorter
than the caller's own** or the admin screen hangs on the slowest dead
service.

## 19.6 Timeouts, retries, and the two things that make retries dangerous

Every outbound call gets a deadline. In `tonic` that is per-request:

```rust
let mut req = tonic::Request::new(GetPageRequest { id: id.to_string() });
req.set_timeout(Duration::from_secs(2));
```

**Retry only what is idempotent.** In this system:

| Call | Retryable? | Why |
|---|---|---|
| `GetPage`, `ListPages`, any read | Yes | Pure |
| `CreatePage` | **No** — unless the id is client-generated | Two pages, one intent |
| `Append` (op log) | **Yes** | Deduplicated on op id — see below |
| `GrantRole` | Yes | Upsert by (user, space) |
| `RespondToInvitation` | **No** | The second answer is refused by design |

The op-log append is the interesting one, and it is the pattern to copy
everywhere else: **the client generates the id, the server deduplicates on
it.** `opstore.Append` writes `ON CONFLICT (id) DO NOTHING` and reports
"already there" as success. That is what makes an at-least-once transport
safe, and it is a property of the *schema*, not of the retry loop.

## 19.7 The outbox, and why it is not a queue

Every service that must publish an event writes it **in the same
transaction as the state change**:

```rust
let mut tx = pool.begin().await?;
sqlx::query!("INSERT INTO collab.ops (...) VALUES (...)").execute(&mut *tx).await?;
sqlx::query!("INSERT INTO collab.outbox (id, aggregate_id, event_type, payload)
              VALUES ($1, $2, $3, $4)", ...).execute(&mut *tx).await?;
tx.commit().await?;
```

A separate poller claims rows with `FOR UPDATE SKIP LOCKED`, publishes, and
marks them published. `SKIP LOCKED` is what lets two pollers run without
either blocking or double-publishing.

**The property this buys:** there is no interleaving in which the state
change is durable and the event is lost. There *is* an interleaving where
the event is published twice (publish succeeded, mark failed) — which is why
every consumer deduplicates on the outbox row id.

**The property it does not buy:** delivery. Core NATS has no redelivery and
delivers nothing to a subscriber that is down. The outbox row survives, so
the evidence exists, but nothing replays it. The Go code says this in
`internal/notify`'s doc comment and `docs/api/notifications.md` § 5 gives the
concrete case where it now matters (a lost mention is somebody never
learning they were asked a question). **The Rust port is the right moment to
fix it** — `async-nats` supports JetStream, and the change is a durable
stream plus an explicit `ack()` per message. If you do fix it, delete the
apologetic comments rather than leaving them to confuse the next reader.

## 19.8 Service-to-service identity

Today every east-west call carries `actor-id` in gRPC metadata, set by
whichever service is acting on a user's behalf, and **the receiving service
trusts it**. That is safe only because nothing but the gateway and the other
services can reach those ports.

Two things to preserve exactly:

1. **`created_by` is never a request field.** It comes from the metadata.
   A client that can name its own author can forge authorship.
2. **`ListAllMemberships` is the one call with no per-space authorization**,
   because there is no single space to authorize it against. It is
   service-to-service. If the port ever exposes it through the gateway, that
   is a breach, not a feature.

If the port hardens this, the shape is mTLS or a service token — but it is
out of scope for the port itself, and doing it halfway (a shared secret in
an env var, checked in three of six services) is worse than the current
honest "the network is the boundary".

---

# Part 20 — gRPC, in Rust

## 20.1 The toolchain

| Go | Rust |
|---|---|
| `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` | `tonic-build` in `build.rs` |
| `buf` (unused here; there is a `scripts/gen-proto.sh`) | `tonic-build`, or `buf` with the `prost` plugin |
| generated into `genproto/` and committed | generated into `OUT_DIR`, **not** committed |

```rust
// crates/document-service/build.rs
fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure()
        .build_server(true)
        .build_client(true)
        // The gateway is a separate crate and needs the client stubs.
        .compile_protos(&["proto/document.proto", "proto/graph.proto"], &["proto"])?;
    Ok(())
}
```

**The one structural difference from Go:** the Go repo puts generated code
at `genproto/` *outside* `internal/`, specifically so `api-gateway` (a
separate module) can import the client stubs. Rust has no `internal/` rule,
so this problem disappears — but you still have to decide who owns the
`.proto` files. Keep them where they are, in the owning service's crate, and
let dependents depend on that crate:

```toml
# crates/api-gateway/Cargo.toml
[dependencies]
document-service = { path = "../document-service", default-features = false, features = ["client"] }
```

Gate the server half behind a feature so the gateway does not compile
`sqlx`, the migrations, and the whole service to get a client stub.

## 20.2 Implementing a service

```rust
#[tonic::async_trait]
impl PageService for PageServer {
    async fn get_page(
        &self,
        request: Request<GetPageRequest>,
    ) -> Result<Response<Page>, Status> {
        let actor = actor_id(&request)?;              // metadata, never a field
        let id: PageId = request.get_ref().id.parse()
            .map_err(|_| Status::invalid_argument("pages: invalid id"))?;
        self.visible(actor, id).await?;               // authorization, one place
        let page = self.repo.get(id).await.map_err(to_status)?;
        Ok(Response::new(page.into()))
    }
}
```

Three rules, all of them lifted from bugs this codebase actually had:

1. **Parse into a newtype at the boundary.** `PageId(Uuid)`, not `String`,
   and not `Uuid`. The proto is stringly-typed; the domain must not be.
2. **Authorization is a separate, named call, not an inlined condition.**
   `visible()` and `may_write()` are the only two, and every RPC calls one of
   them. Part 22 explains why this is worth being rigid about.
3. **One `to_status` function.** Scattered `map_err(|e| Status::internal(..))`
   is how `PermissionDenied` became a 500 in the Go version — "you may not"
   rendered as "the server is broken".

## 20.3 Status codes, and the 404-vs-403 decision

This is not a style question; it is an information-disclosure decision, and
the Go code has it right:

```rust
fn to_status(err: DomainError) -> Status {
    match err {
        // Not a member of the space: NOT_FOUND. A 403 on a resource you
        // cannot see confirms it exists, and space and page names say what
        // people are working on.
        DomainError::NotAMember   => Status::not_found("not found"),
        // A member without the rank: PERMISSION_DENIED is safe here,
        // because "it exists" is already known to you.
        DomainError::InsufficientRole => Status::permission_denied("your role does not allow that"),
        DomainError::NotFound     => Status::not_found("not found"),
        DomainError::InvalidTitle(_) => Status::invalid_argument(err.to_string()),
        DomainError::LastAdmin    => Status::failed_precondition("a space must keep at least one admin"),
        DomainError::AlreadyMember => Status::already_exists("already a member"),
        DomainError::Conflict     => Status::aborted("version conflict"),
        // Everything else is internal, and the message must not leak the
        // underlying error.
        e => { tracing::error!(error = ?e, "internal"); Status::internal("internal error") }
    }
}
```

And at the gateway, one mapping from `Status` to HTTP:

| `tonic::Code` | HTTP |
|---|---|
| `InvalidArgument` | 400 |
| `Unauthenticated` | 401 |
| `PermissionDenied` | 403 |
| `NotFound` | 404 |
| `AlreadyExists`, `Aborted`, `FailedPrecondition` | 409 |
| `Unavailable`, `DeadlineExceeded` | 503 / 504 |
| everything else | 500 |

**Test this table.** The Go port shipped a gateway that mapped five codes
and fell through to 500 for the sixth; the symptom was a working permission
check that looked like a crash.

## 20.4 Metadata and interceptors

```rust
/// The actor, from metadata. Never from a request field.
fn actor_id<T>(req: &Request<T>) -> Result<UserId, Status> {
    req.metadata()
        .get("actor-id")
        .and_then(|v| v.to_str().ok())
        .and_then(|s| s.parse().ok())
        .map(UserId)
        .ok_or_else(|| Status::unauthenticated("an actor id is required"))
}
```

Resist the urge to make this an interceptor that stuffs the actor into
`Extensions`. It reads better, but it makes "which RPCs require an actor"
invisible, and this codebase has already shipped one RPC that forgot to
check. An explicit call at the top of each handler is four characters longer
and impossible to omit silently.

**Outbound**, the actor travels the same way:

```rust
fn with_actor<T>(mut req: Request<T>, actor: UserId) -> Request<T> {
    req.metadata_mut().insert("actor-id", actor.to_string().parse().unwrap());
    req
}
```

The Go version had a real bug here worth not repeating: a two-hop call set
the actor on the second hop only, and every join failed `UNAUTHENTICATED` —
correctly. **Set it on every hop.**

## 20.5 What stays REST

`api-gateway` translates REST to gRPC and does nothing else, with three
deliberate exceptions:

- `GET /admin/health` — the fan-out (19.5). Not a shim; the gateway is the
  only process that knows where everything lives.
- **`collaboration-service`'s WebSocket is reached directly.** A persistent
  connection is not a request/response resource, and proxying it would put a
  stateful hop in front of a stateful service.
- **`collaboration-service`'s debug reads** (`/trace`, `/palimpsest`,
  `/diff`, `/collab/stats`) and `notification-service`'s `/notifications`
  are served directly too — they are instance facts, not resources.

Keep this. The temptation in a rewrite is to put everything behind the
gateway "for consistency", which adds a hop, a translation, and a second
place for the auth decision to be made differently.

---

# Part 21 — Persistence: sqlx, transactions, and the projection rule

## 21.1 The choice, and why it is not an ORM

Go uses `pgx` + `sqlc`: hand-written SQL, generated types, compile-time
checking against a real schema. **The Rust equivalent is `sqlx` with
`query!`/`query_as!`, and the reasoning transfers exactly** — both check the
SQL against the database at build time, and neither invents queries.

```rust
let row = sqlx::query_as!(
    PageRow,
    r#"SELECT id, title, parent_id, space_id, path, sort_key, deleted_at
       FROM docs.pages WHERE id = $1 AND deleted_at IS NULL"#,
    id.0,
)
.fetch_optional(&self.pool)
.await?;
```

Do **not** reach for SeaORM or Diesel here. The schema uses `LTREE`, `JSONB`,
generated `tsvector` columns, `websearch_to_tsquery`, partial unique
indexes, and `FOR UPDATE SKIP LOCKED`. Every one of those is a fight with an
ORM and a one-liner in SQL.

`sqlx` needs either a live database at build time or a committed
`.sqlx/` offline cache. **Commit the cache** — CI should not need Postgres to
type-check, and `cargo sqlx prepare` is one command.

## 21.2 The hard columns

| Column | Postgres | Rust |
|---|---|---|
| `id` | `UUID` (v7, app-generated) | `Uuid`, wrapped in a newtype |
| `path` | `LTREE` | `String` — sqlx has no LTREE type; the Go side overrides it to `string` too |
| `content` | `JSONB` | `sqlx::types::Json<BlockContent>` |
| `sort_key` | `TEXT` | `SortKey(String)` — see 2.4; **never** a float |
| `search_vector` | generated `tsvector` | never selected; only matched |
| `anchor_start` / `anchor_end` | `JSONB` | `Json<Anchor>` |
| `read_at`, `deleted_at` | `TIMESTAMPTZ NULL` | `Option<OffsetDateTime>` |
| `accepted` | `BOOLEAN NULL` | `Option<bool>` — three states, and the third is "unanswered" |

The last row is a trap worth naming. `space_invitations.accepted` is
`Option<bool>`: `None` = pending, `Some(false)` = declined, `Some(true)` =
accepted. A port that models it as `bool` silently turns every pending
invitation into a declined one.

## 21.3 Identifiers: v7, application-side, always

Every id in this system is generated by the application, never by
`DEFAULT uuidv7()`. Two reasons, both still true in Rust:

1. The code needs the id **before** the insert — to write the outbox row in
   the same transaction, to return it, to log it.
2. PG18's native `uuidv7()` is not available on Cloud SQL.

```rust
// uuid = { version = "1", features = ["v7", "serde"] }
let id = Uuid::now_v7();
```

v7 rather than v4 because it is time-ordered, which keeps B-tree inserts
sequential and makes `ORDER BY id` a usable proxy for creation order.

## 21.4 Transactions, and the shape that keeps them short

```rust
pub async fn respond(&self, caller: UserId, id: InvitationId, accept: bool)
    -> Result<Invitation, DomainError>
{
    // Everything that needs the network happens BEFORE the transaction.
    let members = self.spaces.members(space).await?;

    let mut tx = self.pool.begin().await?;
    let inv = answer(&mut tx, id, caller, accept).await?;
    if accept {
        upsert_membership(&mut tx, inv.user, inv.space, inv.role).await?;
        write_role_event(&mut tx, &inv).await?;
    }
    tx.commit().await?;
    Ok(inv)
}
```

**The rule the Go version learned the hard way: never hold a transaction
across a network call.** Two gRPC calls inside an open transaction pin a
connection for the duration of somebody else's network, and a comment box
becomes a way to exhaust the pool. Resolve first, then open.

Rust makes the discipline easier to state: if a `&mut Transaction` is in
scope, no `.await` on anything but the database is allowed. There is no
compiler check for that — write it in a comment where it matters.

## 21.5 The projection rule

Three tables in this system are **projections**, not sources of truth:

| Projection | Source of truth | Fed by |
|---|---|---|
| `docs.blocks` | `collab.ops` | `collab.ops_flushed` |
| `docs.page_links` | `collab.ops` (block text) | same consumer |
| `docs.space_members` | `auth.memberships` | `auth.role_granted` / `_revoked`, plus a periodic reconcile |

Rules that must survive the port:

1. **A projection is never a second writer.** If `docs.blocks` disagrees
   with a replay of `collab.ops`, the ops win and the projection is wrong.
2. **Replay must reproduce it.** This is a testable property, and Part 18's
   test list has it: replay the log into an empty page, compare to the
   projection, assert equality.
3. **The write path does not read the projection.** `can_apply` reads the
   role directory, not `docs.space_members`, precisely because the
   projection can be stale. A stale read on the read path is a slightly old
   list; a stale read on the write path is an authorization decision made
   from out-of-date facts.
4. **An event bus with no redelivery needs a floor.** Hence the periodic
   reconcile against `ListAllMemberships`. Keep it; it is the only thing
   bounding how wrong the projection can get.

## 21.6 Migrations

`sqlx::migrate!` embeds them in the binary and runs them at startup, which
matches the Go/goose behaviour. Keep the numbering and keep them additive.

Two lessons already paid for:

- **A migration can create a state the rules forbid.** `00002` created the
  default space and its memberships; the rule "a space must keep at least
  one admin" was written, tested, and correct, and the data still reached a
  state it forbids — because the migration ran before anything checked.
  `00003` exists to promote the founder. **When you write a migration that
  creates rows the domain has invariants about, assert the invariant at the
  end of the migration.**
- **Down migrations are for development.** Nothing in production runs them.
  Write them anyway; they document what the up migration did.

## 21.7 Pooling and the numbers that matter

```rust
PgPoolOptions::new()
    .max_connections(16)
    .acquire_timeout(Duration::from_secs(3))
    .connect(&url).await?
```

`acquire_timeout` is the one people forget. Without it, pool exhaustion
manifests as requests that hang forever rather than as an error you can see
in a metric. Three seconds turns a resource problem into a visible 503.

---

# Part 22 — Identity, authorization, and the rule that gets broken quietly

This part is longer than its subject looks, because **every security bug
this codebase has had was in this area, and none of them were in the
cryptography.**

## 22.1 The identity chain

```
browser ──JWT (RS256)──▶ api-gateway ──actor-id metadata──▶ services
                    │
                    └──▶ collaboration-service (WebSocket, NOT proxied)
```

- `auth-service` signs RS256 and publishes JWKS at
  `/.well-known/jwks.json`.
- **Every entry point verifies locally against JWKS.** Never an RPC per
  request to ask "is this token good".
- The actor is the token's `sub` claim. Never a header the client controls,
  never a query parameter, never a request field.

In Rust: `jsonwebtoken` for verification, plus a small cache in front of the
JWKS fetch. The Go implementation (`services/authverify`) is 200 lines and
worth reading before writing the Rust one, because its cache has a subtlety:

```rust
struct Cache {
    keys: HashMap<String, DecodingKey>,
    fetched_at: Instant,    // bounds how STALE the cache may be
    attempted_at: Instant,  // bounds how OFTEN an unknown kid may provoke a fetch
}
const MIN_REFRESH_INTERVAL: Duration = Duration::from_secs(30);
```

**Two timestamps, not one.** With one, either rotated keys are rejected
until the TTL expires, or an attacker floods you with unknown `kid` values
and each one triggers a fetch. The Go version shipped the first bug and the
fix is the second field.

## 22.2 The adversarial test list — write these first

This is the one place in the port where tests come before the
implementation. All twelve exist in Go and all twelve must pass in Rust:

1. `alg: none` — rejected.
2. **HS256 forged using the RSA public key as the HMAC secret** — rejected.
   This is the classic JWT attack and a naive verifier accepts it.
3. A token signed by a foreign key that claims a known `kid` — rejected.
4. Expired — rejected.
5. **No `exp` claim at all** — rejected. (A library that treats a missing
   claim as "no expiry" is a library that issues immortal tokens.)
6. `sub` that is not a UUID — rejected.
7. Unknown `kid` flood — at most one fetch per `MIN_REFRESH_INTERVAL`.
8. JWKS unreachable at startup — the verifier **fails closed**, refusing
   every token, rather than starting permissive.
9. Empty JWKS — same.
10. Rotated key — accepted after at most one refresh interval.
11. Valid token — accepted, and yields the right `sub`.
12. `X-Actor-Id` header present alongside a valid token — **the header is
    ignored**, not merely deprioritised.

Run them under `-race`/`loom`-style concurrency and twice (`-count=2` in Go;
in Rust, a test that exercises the cache twice) so cache state carries.

## 22.3 The authorization model

Two questions, two functions, and they are not interchangeable:

```rust
/// READ: is this page inside any space the actor is in?
/// A page outside every one of them is NOT_FOUND, never 403.
async fn visible(&self, actor: UserId, page: PageId) -> Result<(), DomainError>;

/// WRITE: does the actor hold at least `min` in this page's space?
/// Non-member → NOT_FOUND. Member without the rank → PERMISSION_DENIED.
async fn may_write(&self, actor: UserId, space: SpaceId, min: Role) -> Result<(), DomainError>;
```

Roles are ranked (`viewer < editor < admin`) and the rank comparison lives
in one place. A space must keep at least one admin, and **that rule applies
to demotion as much as to removal** — an admin demoting themselves while
they are the only one leaves the same unadministrable space.

For the write path *inside a live session*, `can_apply(page, op, actor)` is
the single chokepoint (RFC-002 §5). It reads a role directory resolved once
per join, not per op. That is a deliberate staleness window, and it must be
stated rather than discovered: a role revoked mid-session takes effect on
the next join.

## 22.4 The failure mode to design against: the reader nobody scoped

**This is the most valuable page in this part.** In September 2026 an audit
of this codebase found that `PageService` was correctly space-scoped and
**four cross-page readers were not**:

| Reader | What leaked |
|---|---|
| `GetLinkGraph` / `AnalyzeGraph` / `GraphNeighborhood` | every page title on the instance |
| `Search` | full-text hits **with `ts_headline` snippets** — page content |
| `Discover` | ranked and named related pages from every space |
| `SuggestTitles` | fuzzy title matches from every space |

None of these was a bug in a permission check. **Each was the absence of
one**, in code written before spaces existed and never revisited when they
arrived. `pages.Server` had `visible()` from the start and the other three
packages simply never grew one.

Three structural lessons for the port:

1. **A per-entity check does not scope a list.** "Can this actor read page
   X" and "which pages may this actor see" are different questions, and a
   codebase that only answers the first will leak through every aggregate.
2. **Make the scope a required parameter, not an optional filter.** The Rust
   fix should be a type:

   ```rust
   /// Proof that a caller's visible space set has been resolved.
   /// Every cross-page query takes one. There is no Default.
   pub struct Scope(Vec<SpaceId>);

   impl Scope {
       pub async fn for_actor(spaces: &impl SpaceReader, actor: UserId) -> Result<Self, DomainError>;
       pub fn as_slice(&self) -> &[SpaceId] { &self.0 }
   }

   pub async fn load_graph(&self, scope: &Scope) -> Result<LinkGraph, Error>;
   pub async fn search(&self, q: &str, scope: &Scope) -> Result<Vec<Hit>, Error>;
   ```

   With `Scope` un-constructible except through `for_actor`, a query that
   forgets to scope **does not compile**. That is the single biggest
   security win available in this port, and it is free.
3. **An empty scope must yield nothing, not everything.** `space_id =
   ANY($1)` with an empty array returns zero rows, which is the safe
   direction. A hand-rolled "if the list is empty, skip the filter"
   optimisation inverts it.

## 22.5 Instance-wide indexes need query-time filtering

`SuggestTitles` reads a BK-tree rebuilt on a cadence. **One index per member
would be one index per person**, so the index stays instance-wide and the
filter is applied when it is queried:

```rust
pub fn suggest(&self, q: &str, max_distance: usize, scope: &Scope) -> Vec<Suggestion> {
    self.tree.query(q, max_distance).into_iter()
        .flat_map(|m| self.by_title[&m.word].iter().map(move |&id| (id, m.clone())))
        .filter(|(id, _)| scope.contains(self.space_of[id]))   // ← the whole point
        .map(|(id, m)| Suggestion { page: id, title: m.word, distance: m.distance })
        .collect()
}
```

The index therefore has to carry each page's space. The Go fix shipped with
that field left at its zero value in one of two construction sites, and
every suggestion was filtered out — a total outage of the feature, caught by
the browser-driving test rather than by any unit test, because the unit test
built the index by hand and the bug was in the loader.

**Test both the deny and the allow.** A filter that rejects everything
passes every "does not leak" test ever written.

## 22.6 Cross-cutting: what an error message may say

- A resource you cannot see: `NOT_FOUND`, and the message must not
  distinguish "does not exist" from "not yours".
- Three separate conditions on invitations — already answered, not yours,
  no such id — return **one** error, deliberately, so the response cannot be
  used as an oracle for whether an invitation exists.
- Internal errors log the detail and return "internal error". The Go code
  does this; a port that helpfully includes the SQL in the response has
  handed over the schema.

---

# Part 23 — Security testing: what to actually test

Part 22 says what the model is. This part says how to attack it, because a
port that re-implements the model and never tests the attacks has copied the
shape of the security and none of it.

## 23.1 The threat model, in one table

| Actor | Can reach | Should be able to |
|---|---|---|
| Anonymous | `POST /auth/{register,login,refresh}`, `GET /health`, `§ 02` home, `GET /collab/stats` | Nothing else. Not one page title. |
| Authenticated, no space | everything requiring a token | See nothing. Empty lists, not errors. |
| Viewer | their spaces | Read. **Not** write, not delete, not invite. |
| Editor | their spaces | Read + write. **Not** manage members. |
| Admin | their spaces | Everything in *those spaces*. Not other spaces. |
| Another service | east-west ports | Whatever it needs; the network is the boundary. |

`GET /collab/stats` being public is deliberate and worth re-deciding
consciously in the port: it reports sessions open, ops flushed, queue depth.
"How busy is this server" is not a secret; "what does this page say" is.

## 23.2 The tests that must exist

Group them by the property, not by the endpoint.

**A. Authentication (Part 22.2's twelve).** Non-negotiable, and they belong
in the `authverify` crate's own test module so they run without a server.

**B. Every entry point requires a token.**

```rust
#[tokio::test]
async fn every_route_refuses_an_anonymous_caller() {
    for (method, path) in ROUTES_REQUIRING_AUTH {
        let res = call(method, path, NoToken).await;
        assert!(matches!(res.status(), 401 | 404), "{method} {path} answered {}", res.status());
    }
}
```

Maintain `ROUTES_REQUIRING_AUTH` as a list next to the router, and make the
allowlist of public routes explicit and short. The Go gateway does exactly
this: a method+path allowlist, everything else authenticated. **An
allowlist, never a denylist** — a route added tomorrow is protected by
default.

**C. Horizontal access control — the one that actually breaks.** For every
read that can return more than one entity:

```rust
#[tokio::test]
async fn a_cross_page_reader_never_names_a_page_outside_your_spaces() {
    let (ivy, ivy_space) = register_with_own_space().await;
    let page = create_page(&ivy, ivy_space, "Secret rope internals").await;
    let other = register().await;               // no shared space

    assert!( graph(&ivy).await.contains_title("Secret rope internals"));
    assert!(!graph(&other).await.contains_title("Secret rope internals"));
    assert!(!search(&other, "Secret rope").await.contains_title(..));
    assert!(!suggest(&other, "Secret rop").await.contains_title(..));
    assert!(!discover(&other).await.contains_title(..));
}
```

Note the first assertion. **A filter that returns nothing to everybody
passes every leak test**, and that is precisely the bug the Go fix shipped
with (Part 22.5). The positive case is half the test.

Write this once per *reader*, not once per endpoint, and add a line the day
a new cross-page reader appears.

**D. Vertical access control.** For each role, the boundary of what it may
do:

- viewer may READ a page in their space
- viewer may **not** create
- viewer may **not** delete a page they can see
- non-member gets **404, not 403** — a 403 confirms it exists
- admin may delete it

The Go version has these five as browser-driven checks, and they caught a
real hole: `can_apply` only sees *ops*, and page lifecycle RPCs (delete,
rename, reparent) are not ops — so a viewer could `DELETE` a page and get
`204`. **Any authorization chokepoint has a blind spot shaped like the calls
that do not go through it.** Enumerate them.

**E. Ownership of an answerable thing.** Invitations are the model: only the
invited may answer, only once, and a wrong id / somebody else's id / an
already-answered id are indistinguishable in the response.

**F. Input that is not text.** Fuzz every parser that touches untrusted
input:

```rust
// cargo-fuzz, or `proptest` for the cheaper version
fuzz_target!(|data: &str| {
    let _ = mention::parse(data);        // must not panic, must stay normalised
    let _ = markdown::compile(data);     // must round-trip: concat(tokens) == source
    let _ = syntax::lex(data);           // same invariant
});
```

The mention parser is small and adversarial in an interesting way: the
grammar decides who gets notified, so a sloppy one is a notification-spam
primitive. 21 million executions found nothing in the Go version; that is a
reasonable bar.

**G. Resource exhaustion.**

- Body size limits on every POST. `axum::extract::DefaultBodyLimit`.
- A cap on ops per WebSocket message and messages per second per connection.
- `acquire_timeout` on the pool (21.7), so exhaustion is a 503 not a hang.
- **The unbounded thing in this system is `Manager`'s session map** — it
  keeps every session open indefinitely, with no idle eviction. That is
  stated as a known limit at demo scale. If the port is meant to run
  anywhere real, this is where to fix it, and the fix has a subtlety:
  evicting a session must flush its WAL first.

**H. Injection.** `sqlx`'s macros parameterise everything, so classic SQLi
is structurally excluded — but `ts_headline` output is **HTML with `<b>`
tags in it**, rendered into the search results page. That is the one place
where a page's own content becomes markup. Test it:

```rust
#[tokio::test]
async fn a_snippet_cannot_inject_markup() {
    create_page_containing("<img src=x onerror=alert(1)> rope").await;
    let hit = search("rope").await.first();
    assert!(!hit.snippet.contains("onerror"));   // escaped, or stripped
}
```

The frontend renders snippets with `dangerouslySetInnerHTML`-equivalent
behaviour to get the bold spans. Either escape everything except the tags
`ts_headline` itself inserts, or render the match spans structurally instead
of as HTML. **Decide this consciously in the port.**

## 23.3 How to run them

- Unit + property + fuzz: `cargo test`, `cargo fuzz`, no infrastructure.
- Integration: `testcontainers` for Postgres and NATS. **Never mock
  infrastructure** — the same rule the Go side has. A mocked Postgres cannot
  tell you that `FOR UPDATE SKIP LOCKED` does what you think.
- End-to-end authorization: drive the real HTTP surface with two or three
  real accounts. The Go repo's `tools/uidiff/verify.js` does this from a
  browser and found holes that no unit test could, because **both services
  were individually correct and the gap was between them.**

## 23.4 What a review should look for

A short list, ordered by how often it has actually been wrong here:

1. A new cross-page read with no `Scope` parameter.
2. A new RPC that does not call `visible()` or `may_write()`.
3. A new write path that does not go through `can_apply`.
4. An error that distinguishes "does not exist" from "not yours".
5. A `403` where the caller cannot see the resource at all.
6. An `unwrap()` on anything derived from a request.
7. A token check that is skipped for a "debug" endpoint.
8. A projection consulted on the write path.

---

# Part 24 — The bug catalogue: test cases this system has already earned

Every entry below is a defect that shipped in the Go implementation and was
found afterwards. They are the highest-value tests in the port, because they
are proven to be reachable. Write them **before** the code they test.

## 24.1 Document model and ops

| # | Bug | The test |
|---|---|---|
| 1 | Converting a block kind lost its text | Convert paragraph→heading→quote; assert content survives each hop |
| 2 | A restore replayed the log but skipped the unrecorded seed block, so the editor inserted one and the log grew (`1/1 → 0/2`) | `restore_to(v)` then assert op count is exactly `v` |
| 3 | Marks are whole-block last-write-wins; concurrent edits to a marked block silently lose one | Assert the tradeoff explicitly so it cannot regress into a surprise |
| 4 | Mark offsets are UTF-16 (JS) while the core persists bytes | A mark over `"café"` and over an emoji; assert the resolved range |
| 5 | `content_version` not bumped on a schema change | Load a v1 document with a v2 binary; assert refusal, not silent misparse |

## 24.2 Anchors and history

| # | Bug | The test |
|---|---|---|
| 6 | **`serverActor` was generated fresh per process start.** Every replay after a restart produced different `ItemId`s, and every anchor resolution against persisted history failed with "anchor refers to an item this text never saw" | Restart the service between writing and resolving an anchor. This is *the* highest-value test in the whole system |
| 7 | `palimpsest::build` replayed only the character tier, but sessions seed ropes from block ops → 500 | Build a palimpsest for a block whose text arrived inside its `InsertBlock` |
| 8 | A `SetBlockContent` reseed must tombstone every live char and insert the new ones | Assert `stored == live + tombstoned` after a reseed |
| 9 | An orphaned anchor is an *answer*, not an error | Delete the anchored text; assert `orphaned: true` and a 200 |

## 24.3 Authorization

| # | Bug | The test |
|---|---|---|
| 10 | A scoped list beside an unscoped `GetPage` | Fetch a page by id from outside your spaces; assert 404 |
| 11 | `can_apply` only sees ops, so a viewer could `DELETE` a page (204) | Viewer deletes; assert refusal |
| 12 | Four cross-page readers unscoped (graph, search, discover, suggest) | Part 23.2 C, with its positive half |
| 13 | The space filter shipped with the space id left at its zero value in one construction site, so the feature returned nothing at all | The positive assertion in the same test |
| 14 | `ListMembers` is admin-only, so mention resolution silently worked for admins only | Resolve a mention **as a viewer** |
| 15 | Migration created a default space with no admin | Assert the invariant at the end of the migration |
| 16 | A new registration joins as viewer, so a fresh install had nobody who could write | First registration bootstraps as admin; assert on an empty database |

## 24.4 Services, transport, and the gateway

| # | Bug | The test |
|---|---|---|
| 17 | `PermissionDenied` unmapped at the gateway → 500 | A table test over every `Code` → HTTP status |
| 18 | **CORS preflight rejected `Authorization`**, so an authenticated fetch never reached the handler — and the symptom looked like a data problem | Assert `OPTIONS` returns the header; in the browser, listen for `requestfailed`, not `response` |
| 19 | The actor was set on the second gRPC hop only; every join failed `UNAUTHENTICATED` | A two-hop call under a non-admin actor |
| 20 | A comment's mention resolution held two gRPC calls inside an open transaction | Assert no `await` on a non-database future while a `Transaction` is live (review rule; also visible as pool exhaustion under load) |

## 24.5 Frontend and the harness

These are not Rust bugs, but the *harness* stays, so the lessons do:

| # | Bug | The lesson |
|---|---|---|
| 21 | A rail section never rendered; eyeballing missed it repeatedly | Diff the DOM against the mockup; `missing` must be 0 |
| 22 | A control rendered but did nothing | Click every control; assert it changes something |
| 23 | Hidden toolbars still took clicks (`opacity: 0` is still a hit target) | `pointer-events: none` with the opacity, and a probe that clicks through |
| 24 | A menu's backdrop sat above the menu (`z-index` 29 vs 20) | Measure with `elementFromPoint`, do not reason about stacking |
| 25 | A flex column with `max-height` shrank its children instead of scrolling | `flex: none` on the rows; measure the row height |
| 26 | Peer carets rendered at the end of the article | Viewport coordinates need a positioned ancestor |
| 27 | A check waited on `networkidle` in an app that polls every 30s | **Wait on what you assert on**, never on a proxy for it |
| 28 | A check asserted on a corpus fact ("some page has a thread") that a reseed removed | Produce your own precondition |
| 29 | Two test sweeps running concurrently produced four phantom failures | One sweep at a time; a racing run's output is not evidence |

## 24.6 The meta-lesson

Sort the list above by cause and almost all of it falls into three buckets:

1. **A check that waits on, or asserts on, something other than the thing it
   is checking.** (27, 28, 18, 21, 22)
2. **A rule that exists but is not applied at one of its call sites.**
   (10, 11, 12, 14, 15, 19)
3. **A value that must be stable and was not.** (6, 13, 2)

Bucket 2 is the one a type system can fix, and Part 22.4's `Scope` newtype is
the worked example: make the rule a parameter that cannot be defaulted, and
the compiler enumerates the call sites for you. **Where the port can convert
a discipline into a type, it should.** That is the strongest argument for
doing this in Rust at all.

---

# Part 25 — Sketches: HyperLogLog, Count–Min, t-digest

`marginal-sketch` is a small crate with an unusually strong claim attached
to it: **every sketch returns its exact answer beside its estimate.** A
sketch that hides its error is indistinguishable from a wrong number, and
the screen built on this crate (`§ 12`) exists to make that visible.

Port this crate early. It is self-contained, has no I/O, compiles to wasm,
and every one of its properties is testable without a server.

## 25.1 HyperLogLog — counting distinct things in a fixed space

**The problem.** How many distinct actors edited this workspace? An exact
answer needs a set, and a set grows with the data.

**The idea.** Hash each item to a uniform 64-bit value. Use the first `p`
bits to pick one of `m = 2^p` registers; count the leading zeros of the rest
and keep the maximum per register. A register whose maximum run of leading
zeros is `k` suggests roughly `2^k` distinct items landed in it — because
seeing `k` zeros has probability `2^-k`. Averaging over `m` registers with
the *harmonic* mean damps the outliers that would otherwise dominate.

```rust,ignore
pub struct HyperLogLog { p: u8, registers: Vec<u8> }   // m = 1 << p

impl HyperLogLog {
    pub fn add(&mut self, item: &str) {
        let h = hash64(item);
        let idx = (h >> (64 - self.p)) as usize;
        let rest = (h << self.p) | (1 << (self.p - 1));   // guard the shift
        let rank = rest.leading_zeros() as u8 + 1;
        self.registers[idx] = self.registers[idx].max(rank);
    }

    pub fn estimate(&self) -> f64 {
        let m = self.registers.len() as f64;
        let sum: f64 = self.registers.iter().map(|&r| 2f64.powi(-(r as i32))).sum();
        let raw = ALPHA_M * m * m / sum;
        // Small-range correction: with few items most registers are 0, and
        // the raw estimator is badly biased there. Linear counting is exact
        // enough in that regime and the crossover is the standard 2.5m.
        let zeros = self.registers.iter().filter(|&&r| r == 0).count() as f64;
        if raw <= 2.5 * m && zeros > 0.0 { m * (m / zeros).ln() } else { raw }
    }
}
```

**Standard error is `1.04 / sqrt(m)`.** With 64 registers that is ±13%,
which is why the screen draws the estimate *against its own bound* rather
than as a number.

**Port notes.**
- `leading_zeros()` on `0` is 64, which would make one unlucky hash claim an
  absurd rank. The `| (1 << (p-1))` guard above is not decoration.
- Registers are `u8`; a rank cannot exceed 64.
- Merging two HLLs is a per-register `max`. That is the property that makes
  them useful across shards, and it is worth a test even if nothing merges
  yet.
- **Test that a duplicate does not move the estimate** and a new item does.
  Those two assertions catch almost every implementation error.

## 25.2 Count–Min — frequency, never underestimated

**The problem.** How many times did page X appear in the stream, in fixed
space, with no per-key allocation?

**The idea.** `d` independent hash functions, each indexing a row of `w`
counters. To add, increment one counter per row. To query, take the
**minimum** across rows. Collisions can only ever add to a counter, so the
minimum is an over-estimate — never an under-estimate.

```rust,ignore
pub struct CountMin { w: usize, d: usize, counts: Vec<u32> }   // d rows × w cols

impl CountMin {
    pub fn add(&mut self, key: &str, n: u32) {
        for row in 0..self.d {
            let i = self.index(row, key);
            self.counts[row * self.w + i] += n;
        }
    }
    pub fn estimate(&self, key: &str) -> u32 {
        (0..self.d).map(|row| self.counts[row * self.w + self.index(row, key)])
                   .min().unwrap_or(0)
    }
}
```

**The guarantee**: with `w = ⌈e/ε⌉` and `d = ⌈ln(1/δ)⌉`, the estimate
exceeds the truth by more than `ε·N` with probability at most `δ`.

**The property to test, and to display**: `estimate(k) >= exact(k)` for every
`k`. One-sided error is the whole contract, and a test that only checks
"close enough" would pass an implementation that sometimes undercounts —
which would be a different, useless data structure.

The `4 × 24` shape in the Go version is deliberately small so the screen can
*draw every counter*. Keep it configurable, keep the default small, and say
why in a comment.

## 25.3 t-digest — quantiles without keeping the data

**The problem.** What is the p99 of these latencies? Exact quantiles need
every sample; a histogram needs its buckets chosen in advance, and the
interesting part of a latency distribution is exactly where fixed buckets
are worst.

**The idea.** Keep a set of *centroids* (mean, count). Merge new points into
the nearest centroid, but bound each centroid's size by a **scale function**
of its quantile position: centroids near the median may be large, centroids
near 0 and 1 must stay tiny. So the tails keep resolution and the middle
compresses.

```rust,ignore
pub struct TDigest { centroids: Vec<Centroid>, count: u64, compression: f64 }
struct Centroid { mean: f64, count: u64 }

// k(q) = compression * (asin(2q - 1) / π + 0.5) is the usual scale;
// what matters is that it is steep at the tails and flat in the middle.
```

Merging: sort the buffered points, walk the centroids in order accumulating
`q`, and fold a point into the current centroid while its resulting count
stays under the size the scale function permits at that `q`; otherwise start
a new centroid.

**Why this and not a fixed histogram**: the p99 of a latency series is the
number people act on, and a fixed histogram either wastes buckets where
nothing happens or bins the tail so coarsely the answer is a bucket
boundary.

**Port notes.**
- The accuracy claim is relative to the *quantile*, not the value, and it is
  strongest at the extremes. Test with a known distribution and assert the
  error bound, not an exact value.
- Keep a `Vec<f64>` of raw samples alongside in debug/test builds so the
  exact quantile is computable and the screen can show both. That is the
  crate's whole editorial stance.

## 25.4 The `analyze` entry point

One function takes an event stream and returns all three answers plus the
exact ones. Its signature is the crate's contract with the wasm bridge:

```rust,ignore
#[derive(serde::Serialize)]
pub struct Analysis {
    pub distinct_actors: Estimate<u64>,      // { estimate, exact, error_bound }
    pub page_counts: Vec<Estimate<u32>>,
    pub latency_quantiles: Vec<Quantile>,    // { q, estimate, exact }
    pub malformed_lines: usize,              // counted, never fatal
}
```

**A malformed line is counted, not fatal.** The screen feeds this from an
editable text box, so a half-typed line is the *normal* state, and a parser
that throws makes the panel flicker to an error on every keystroke.

---

# Part 26 — The markdown compiler, and the lexer

Two crates, one invariant, and it is the most useful invariant in the whole
system to state as a test.

## 26.1 The invariant

```
concat(tokens) == source
```

For the lexer (`marginal-syntax`, nine languages) this is literal: every
byte of the input appears in exactly one token, in order, including
whitespace and unrecognised characters. For the markdown compiler
(`mdc`) the equivalent is that the token stream round-trips.

**Why this rather than "it highlights correctly":** "correctly" needs a
reference implementation to compare against, and there is none. Round-trip
is checkable in one line, catches every dropped-character bug, and is
exactly the property a syntax highlighter must have to be safe to render.

```rust,ignore
proptest! {
    #[test]
    fn lexing_never_loses_a_byte(src in ".{0,4000}") {
        let joined: String = lex(&src, Lang::Rust).iter().map(|t| t.text.as_str()).collect();
        prop_assert_eq!(joined, src);
    }
}
```

## 26.2 The lexer's shape

A lexer is a state machine, so it is Go today and Rust tomorrow — never
TypeScript. Per language: a set of keyword sets, comment syntaxes, string
delimiters (with their escape rules), and number formats.

```rust,ignore
pub enum TokenKind { Plain, Keyword, Type, Str, Number, Comment, Punct }
pub struct Token { pub kind: TokenKind, pub text: String }
```

In Rust, prefer `&'a str` slices over owned `String`s in the token — the
whole point is that they partition the source:

```rust,ignore
pub struct Token<'a> { pub kind: TokenKind, pub text: &'a str }
```

This makes the round-trip invariant *structural*: the tokens are literally
slices of the input, so losing a byte requires losing a slice. Only own the
strings at the wasm boundary, where they must be serialised anyway.

**Traps, all of them real:**
- Nested block comments (Rust allows them; C does not). Track depth.
- Raw strings with variable hash counts (`r##"…"##`).
- A string that never closes, and a comment that never closes. Both must
  terminate at EOF and produce a token, not loop.
- Multi-byte characters. Index by `char_indices()`, never by `usize` on a
  `&[u8]` you then slice.

## 26.3 The markdown compiler

`mdc` is the paste pipeline of Part 5 in library form: lex → parse → lower →
emit. Two properties beyond the round-trip:

1. **An unclosed fence reports rather than failing.** The screen's input is
   a live editor, so an incomplete document is the normal state. Return a
   diagnostic alongside a best-effort tree; never `Err`.
2. **Chars and bytes diverge, and the divergence is named.** The compiler
   reports both counts, because a port that conflates them is the source of
   the offset bugs in Part 24.1. Rust makes this easier to get right and
   just as easy to get wrong: `s.len()` is bytes, `s.chars().count()` is
   chars, and the editor's marks are UTF-16 code units. **Three different
   numbers. Name which one every API means.**

---

# Part 27 — The network simulator: TP1, Merkle, the causal DAG, LSM

`marginal-netsim` is a *simulation*, and the screen says so. The real engine
is `collaboration-service`; this crate is the deterministic, re-runnable
version of one bad network, so the algorithms can be watched rather than
asserted about.

Port it after Part 8, because it re-uses the same op types, and port it at
all because it is the single best test harness for the transform.

## 27.1 TP1, and what convergence actually requires

Operational transformation's first transport property:

> For any two concurrent operations `a` and `b` on the same state,
> `apply(apply(s, a), transform(b, a)) == apply(apply(s, b), transform(a, b))`.

That is: two replicas that receive the same pair of operations in different
orders end up in the same state.

For insert/delete on a linear sequence, the transform is a position
adjustment:

```rust,ignore
fn transform(op: Op, against: Op) -> Op {
    match (op, against) {
        (Insert { pos: p, .. }, Insert { pos: q, .. }) if q < p => Insert { pos: p + 1, .. },
        // The tie. Both inserted at the same position; SOMETHING must break
        // it, and it must be the same something on both replicas.
        (Insert { pos: p, actor: a, .. }, Insert { pos: q, actor: b, .. }) if q == p && b < a =>
            Insert { pos: p + 1, .. },
        (Insert { pos: p, .. }, Delete { pos: q, .. }) if q < p => Insert { pos: p - 1, .. },
        (Delete { pos: p, .. }, Insert { pos: q, .. }) if q <= p => Delete { pos: p + 1, .. },
        (Delete { pos: p, .. }, Delete { pos: q, .. }) if q < p => Delete { pos: p - 1, .. },
        // Both deleted the same character. The second is a no-op, not an error.
        (Delete { pos: p, .. }, Delete { pos: q, .. }) if q == p => Op::Noop,
        (op, _) => op,
    }
}
```

**The tiebreak is the whole algorithm.** Two inserts at the same position
must be ordered by something total and agreed — actor id, here. Get that
wrong and the replicas diverge by one character, forever, and the symptom
appears minutes later as unrelated corruption.

**The test that matters** is not "transform returns the right number". It is
the property:

```rust,ignore
proptest! {
    #[test]
    fn tp1_holds(ops in vec(any_op(), 0..40), seed: u64) {
        let (a, b) = two_replicas_receiving_in_different_orders(ops, seed);
        prop_assert_eq!(a.text(), b.text());
    }
}
```

Run it with thousands of seeded interleavings. This is the single highest-
value property test in the port.

## 27.2 Prediction and rollback

The client applies its own op immediately (prediction), then reconciles when
the server's ordering arrives: rewind to the last confirmed state, replay the
server's ops, then replay its own pending ops transformed against them.

The simulation makes the wire visible — RTT and loss are sliders, and the
intent ledger records what each op *meant* so that "converged" and "did what
the person wanted" can be told apart. **Turn the transform off and the
replicas still converge structurally** (same length, same tree) while the
ledger flags that the intent was violated. That distinction is the reason the
screen exists, and it is worth preserving exactly.

## 27.3 Merkle comparison (AHU-flavoured)

To ask "do these two replicas agree" without shipping the document: hash
each node bottom-up, `hash(node) = H(kind ‖ content ‖ hash(child₁) ‖ …)`, and
compare roots. Equal roots ⇒ equal trees (to the collision bound). Unequal
⇒ descend into the children whose hashes differ, so a diff costs `O(depth ×
branching)` rather than `O(size)`.

The AHU part is canonical *labelling*: children must be hashed in a
canonical order so that two structurally identical trees hash identically
regardless of construction history. For an ordered document tree, document
order *is* the canonical order — which makes this simpler here than in the
general case, and worth a comment saying so.

## 27.4 The causal DAG and its longest chain

Each op names the ops it saw (`parents`). That is a DAG, and:

- **Concurrency** is exactly "neither is an ancestor of the other".
- The **longest chain** is the critical path of causality, computed by DP
  over a topological order: `depth(v) = 1 + max(depth(parents))`.

The number is interesting because it separates "forty edits happened" from
"forty edits happened *in sequence*, each having seen the last".

## 27.5 The LSM shape model

Not a storage engine — a model of one, over the op log: a memtable that
fills, flushes to a level-0 file, and compacts into larger levels. It reports
**write amplification**: total bytes written to disk ÷ bytes of logical data.

The reason it is in this product at all: the op log *is* an LSM-shaped
workload (append-only, never updated in place, read by replay), and drawing
it that way explains why the WAL and the flush pipeline are shaped as they
are. Port it as a pure function over `Vec<Op>` returning a level structure,
and keep it deterministic.

---

# Part 28 — Benchmarking honestly

`marginal-bench` exists because "fast" is a claim, and the screen that shows
it (`§ 16`) must be able to say how it knows.

## 28.1 Clock resolution comes first

Before timing anything, measure the clock:

```rust,ignore
fn clock_resolution() -> Duration {
    let mut min = Duration::MAX;
    for _ in 0..1000 {
        let a = Instant::now();
        let b = loop { let b = Instant::now(); if b > a { break b } };
        min = min.min(b - a);
    }
    min
}
```

In a browser this matters enormously: `performance.now()` is deliberately
coarsened (and jittered) as a Spectre mitigation, so single-operation timings
are meaningless. **The screen states the clock it was quantised by**, which
is the honest version of a benchmark number.

## 28.2 Batch calibration — `testing.B`'s trick

If one operation is faster than the clock, time `n` of them and divide.
Choose `n` by doubling until the batch takes comfortably longer than the
resolution:

```rust,ignore
let mut n = 1;
while time_batch(n) < resolution * 100 { n *= 2; }
```

This is exactly what Go's `testing.B` does and for exactly the same reason.
Say so in a comment; a reader who recognises it will trust the rest.

## 28.3 Percentiles, buckets, and the flame graph

- **Log-spaced buckets.** Latency spans orders of magnitude; linear buckets
  put every interesting value in the first one.
- **Nearest-rank percentiles**, not interpolation. `p99` of 100 samples is
  the 99th sorted sample — a value that actually occurred. Interpolating
  invents a number nothing measured.
- **The flame graph is walked from instrumented spans, not sampled.** There
  is no sampling profiler in wasm; inventing stacks would be exactly the
  dishonesty the screen argues against. So the crate carries explicit span
  enter/exit, and the graph is only as detailed as the instrumentation.

## 28.4 The workloads must be real paths

Four workloads, each a real function from this codebase: `Page::apply`,
`mdc::compile`, `netsim::run`, and the semantic tokenise+embed. Not
synthetic loops. The number is only interesting because it is *about this
codebase*, measured where the reader is sitting.

**Port note.** `criterion` is the right tool for the developer-facing
benchmarks (`cargo bench`), and it does the statistics properly. But the
in-browser benchmark cannot use it — it needs the crate's own harness
compiled to wasm. Keep both: `criterion` for the CI number, this crate for
the screen.

---

# Part 29 — Configuration, deployment, and observability

## 29.1 The ports-and-adapters rule, restated for Rust

`CLOUD_PORTABILITY.md`'s rule: **every external dependency sits behind a
small interface declared at its point of use.** In Go that is a
three-method interface in the consuming package. In Rust it is a trait in
the consuming module, and — per Part 16's PORT-NOTE — a **generic bound**,
not `dyn`, wherever the implementation is known statically.

```rust,ignore
// In the module that USES it, not in a `ports` module.
pub trait EventBus {
    async fn publish(&self, subject: &str, payload: &[u8]) -> Result<(), BusError>;
}

pub struct Poller<B: EventBus> { bus: B, pool: PgPool }
```

The adapters that exist: Postgres (sqlx), NATS (`async-nats`), Redis,
object storage. Each has exactly one production implementation and one test
double. That is the shape a generic bound is for.

**What this buys, concretely**: local dev runs NATS and MinIO in Docker;
cloud runs Pub/Sub and Cloud Storage. Neither the algorithm crates nor the
domain modules know which.

## 29.2 The compose stack

Six services, one database each, plus Redis and NATS. That is not
microservice theatre — **database-per-service is what makes the boundaries
real**, and the moment two services share a schema the boundary is a
convention rather than a constraint.

Keep in the port:
- One Postgres per service, each with its own credentials.
- The frontend dev server in the same compose file, so `docker compose up
  --build` is the entire local setup.
- **The production compose file is standalone, not an override.** Merging
  the base and an override "looks right and breaks the site": the base's web
  service is a Vite dev server whose `image:` and `command:` survive the
  merge and get applied to the built static image, which has no npm. It
  restart-loops on `sh: npm: not found`, every HTML route 502s, and the wasm
  files keep returning 200 — the confusing part, since the reverse proxy is
  up and only the container behind it is gone.

## 29.3 Deployment, and the failure that matters

A deploy that reports success and did nothing is worse than one that fails
loudly. This has happened here: an `ssh 'nohup … & echo started'` printed a
success banner while two `Connection closed` lines scrolled past, and
production ran the vulnerable commit for another hour.

**The rule: verify from the far side.** After deploying, ask the remote what
it is running, and check the artefact, not the process:

```sh
ssh deploy@host 'cd ~/app && git log --oneline -1 && docker compose ps'
curl -s https://app.example/assets/index-*.js | grep -c 'a-string-only-the-new-code-has'
```

Three related traps, all paid for:
- `git checkout -qb X || git checkout X` silently lands on a stale
  pre-existing branch when the first form fails.
- `docker compose up --build` builds an image but does **not** necessarily
  recreate a running container; add `--force-recreate` after a rebase.
- Building with a merged compose file retags the base image
  (`node:22-alpine`) with your build, which then breaks something unrelated
  hours later.

## 29.4 Observability

Deferred in the Go repo; the port should not defer it, because it is cheaper
to add while the code is being written than afterwards.

```rust,ignore
tracing_subscriber::registry()
    .with(EnvFilter::from_default_env())
    .with(tracing_subscriber::fmt::layer().json())
    .with(tracing_opentelemetry::layer().with_tracer(tracer))
    .init();
```

- **`tracing`, not `log`.** Spans, not lines: a request that crosses four
  services is only legible as a trace.
- **Propagate the trace context through gRPC metadata**, and through the
  outbox payload — an event published now and consumed later belongs to the
  request that caused it, and losing that link is the usual reason async
  systems are hard to debug.
- **Cardinality discipline.** Never a page id or a user id as a metric
  label. They belong in span attributes, which are sampled; a metric label
  is multiplied by every series forever.
- The four numbers this system already knows how to report, and should
  export as metrics: outbox depth, op-log lag, sessions open, ops flushed
  per second.

## 29.5 Cost posture

The Go repo's cloud plan is two-tier and deliberately cheap: scale-to-zero
services, one small Postgres per service, no always-on cluster. Keep that
shape in the port and keep the reasoning visible, because the natural
gravity of a rewrite is toward a managed everything.

The one exception is `collaboration-service`: **it is stateful and holds
WebSockets, so it cannot scale to zero** and its instance count is a
function of concurrent connections. That asymmetry drives the whole
deployment topology and should be stated wherever the topology is described.

---

# Part 30 — The order of work, with checkpoints

This is the part to follow literally. The ordering is not arbitrary: each
step is checkable on its own, and each one's dependencies are already done.

## Stage 0 — Scaffolding (a day)

Cargo workspace, one crate per Part-19 row, `rustfmt.toml`, `clippy.toml`
with `-D warnings` in CI, `cargo-deny` for licences and advisories.
`docker-compose.yml` copied across unchanged — the databases and the bus do
not care what language talks to them.

**Checkpoint:** `cargo build --workspace` succeeds and produces six binaries
that start, serve `/health`, and exit cleanly on `Ctrl-C`.

## Stage 1 — The pure crates (a week)

In this order, because each is independently testable and none has I/O:

1. `graphalgo` (Part 10) — the easiest, and the algorithms are self-checking.
2. `textdiff` (Part 13) — LCS table + traceback, one property test.
3. `syntax` (Part 26) — the round-trip invariant is the test.
4. `sketch` (Part 25) — estimate-vs-exact is the test.
5. `semantic` (Part 12) — recall measured against brute force is the test.

**Checkpoint:** every crate has its property tests, and `cargo test
--workspace` is green with no service running. Port the Go golden vectors in
`testdata/` verbatim and make them pass — they are the contract.

## Stage 2 — `documentcore` (a week)

The block tree, the op ISA, `apply`, `invert`. Part 3, Part 6.

**Checkpoint:** the golden vectors in `testdata/document-core/*.json` pass
unchanged, and the round-trip property (`apply` then `invert` restores the
tree) holds under `proptest` for random op sequences.

This is the crate everything else depends on. Do not move on while any
golden vector is skipped.

## Stage 3 — `notification-service` (two days)

The smallest complete service: one table, one NATS consumer, three HTTP
routes. It exercises the whole vertical — config, pool, migrations, outbox
consumption, HTTP, JWT verification — at the smallest possible size.

**Checkpoint:** register a user through the real `auth-service` (still Go!)
and see a welcome notification appear in the Rust service. **Cross-language
running is the point** — it proves the wire contracts are actually
compatible, and it lets you port one service at a time.

## Stage 4 — `auth-service` (a week)

Identity first, per `ADR-013`. Argon2id (`argon2` crate), RS256 + JWKS
(`jsonwebtoken`), sessions, spaces, roles, invitations.

**Checkpoint:** Part 22.2's twelve adversarial tests pass, and the Go
services accept tokens issued by the Rust `auth-service`. That is the real
proof: JWKS is a contract, not an implementation detail.

## Stage 5 — `document-service` (two to three weeks)

The largest surface. Sub-order: pages → blocks projection → graph → search →
discover → trash/saga.

**Checkpoint, and do not skip it:** the `Scope` newtype from Part 22.4
exists before the first cross-page reader is written, and Part 23.2's
horizontal-access test (with its positive half) passes.

## Stage 6 — `collaboration-service` (three weeks, and the hard one)

Rope, anchors, the session, the WAL, the flush pipeline, the WebSocket, the
role directory, comments.

Sub-order matters here:
1. `doctext` rope + anchors, tested alone (Part 7).
2. Session state machine, driven by a script of ops, no network.
3. WAL and flush, with a crash injected between write and flush.
4. The WebSocket, last.

**Checkpoints:** the `serverActor` stability test (Part 24.2 #6) —
restart the process between writing and resolving an anchor. The TP1
property test (Part 27.1). A replay of `collab.ops` reproduces
`docs.blocks` exactly (Part 21.5 rule 2).

## Stage 7 — `api-gateway` and `diagnostics-service` (a week)

Both thin. The gateway is REST↔gRPC translation plus the health fan-out;
diagnostics is nine pure analyzers over a gRPC client.

**Checkpoint:** the frontend — unchanged, still TypeScript — runs against
the all-Rust backend, and `tools/uidiff/verify.js` passes in full.

## Stage 8 — wasm (a week)

`wasm-bindgen` for the four entrypoints, `wasm-opt -Oz`, and a measurement
of the resulting bundle. The Go build ships ~28 MB across nine modules;
Rust should be dramatically smaller, and **measuring it is part of the
deliverable** — it is one of the few places where the port has a number to
show for itself.

## The final checkpoint

The acceptance bar is not "the tests pass". It is:

> **`tools/uidiff/uidiff.js` reports `missing 0 · property 0 · chrome text 0`
> on every screen, and `tools/uidiff/verify.js` passes, against the Rust
> backend, with the TypeScript frontend unchanged.**

The frontend is the permanent visual harness precisely so this comparison is
possible. If a screen differs, the port is wrong — not the screen.

