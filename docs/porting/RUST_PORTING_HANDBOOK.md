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
> part is self-contained enough to be worked in isolation, and Part 18 gives
> the order that minimises rework.
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
| `marginal/outboxpoll` | `marginal-outbox` | lib, needs a `Pool` |
| `marginal/envconfig` | *delete it* | see 1.3 |
| `document-service` | `marginal-document-service` | bin |
| `collaboration-service` | `marginal-collab-service` | bin |
| `auth-service` | `marginal-auth-service` | bin |
| `notification-service` | `marginal-notify-service` | bin |
| `diagnostics-service` | `marginal-diagnostics-service` | bin |
| `api-gateway` | `marginal-gateway` | bin |

**The four `no I/O` crates are the port's centre of gravity.** They hold
every algorithm, they have no async, no database, no network — and they are
therefore the crates where Rust's type system buys the most and costs the
least. Port them first (Part 18).

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

**Invariant F11.3 — the metric must actually be a metric.** Levenshtein is.
Damerau-Levenshtein with unrestricted transpositions is **not** (it violates
the triangle inequality), and using it silently makes the pruning wrong —
results just quietly go missing. If you want transpositions, use the
*optimal string alignment* variant and know that it is a metric only for
adjacent transpositions.

**Invariant F11.4 — results are sorted by distance, then lexicographically.**
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

**Invariant F11.5 — matching is case-insensitive and the STORED key is
normalised, but the DISPLAYED title is the original.** Lowercasing on the way
in and rendering the lowercase form is the bug you ship on day one.

**Invariant F11.6 — completion is bounded.** A prefix of `""` must not walk
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
