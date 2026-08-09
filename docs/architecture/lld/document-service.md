# LLD — `document-service`

**Owns:** its own database — `pages`, `blocks`, `page_links`, its outbox, and **`ops` for Phase 1 only**. Phase 3 hands the op log to `collaboration-service`, after which `blocks` becomes a projection materialised from op events (ADR-003 § Amendment)
**Transport:** gRPC `PageService` (ADR-007). HTTP exists only for Kubernetes probes.
**Depends on:** PostgreSQL 18 only (Phase 1). NATS arrives with the outbox poller.
**Related:** `DATA_MODEL.md` §4 (schema) · `PROJECT_STRUCTURE.md` §3 (service template) · `RFC-001` (document model) · `RFC-002` (op model) · `docs/api/pages.md` (the contract) · `ADR-007` (gRPC east-west)

This document is the specification the implementation is written against. **No code exists yet** —
the executable half of the same spec is a test suite you write before each slice
(`agents.md` § stage 1).

---

## 1. Scope — what is hand-written here

> **This document is one half of Phase 1.** The other half is the **editor core** — parser, input
> rules, paste, lowering — which lives in `libs/doc` and is specified in
> [`libs-doc.md`](libs-doc.md). Nothing below contains a parser, deliberately: no server-side code
> path parses markdown. Build from both documents or you will ship a service with no editor.

The startup path — boot, `/health`, `SIGTERM` drain, config, telemetry — is **scaffolding, not
design**. It has to be written, but it is out of scope *here*: write it once from *Zero To
Production* Ch. 3–5 and never think about it again.

| Scaffolding — write once, then ignore | Designed below |
|---|---|
| `main` / `lib` — serve, pool, drain, listeners | The protobuf contract — `docs/api/pages.md` §1 |
| config loading + `config.yaml` | Domain types — ids, `SortKey`, validation |
| telemetry subscriber | `pages` — repo and transport |
| probe router + edge middleware | `blocks` — the projection |
| shared state | `tree` — LTREE, cycle check, cascade |
| `AppError` → transport status | `ops` — the write path (§5.1) |
| liveness / readiness | the outbox |
| | migrations |

**Every row on the right teaches something on `ROADMAP.md` § Rust, DSA & Concepts Map.** A row
that teaches nothing should be a dependency, not hand-written code.

> **The module map below is a proposal, not a contract.** It was written before the layout was
> yours to choose. The *contracts* — invariants, error mapping, §9's algorithms, §12's traps — hold
> regardless of how you arrange the files. If your arrangement is better, this document is what
> needs changing.

---

## 2. Module map

Feature-first slices, per `PROJECT_STRUCTURE.md` §2. A slice owns its whole vertical:
HTTP in → logic → SQL out. Slices never import each other's internals.

```
src/
│  ── scaffolding · §1 · not designed here, not to be rewritten ──
├── main.rs           config → telemetry → state → serve
├── lib.rs            pool, both listeners, bounded drain
├── config.rs         Settings — the definitive schema
├── telemetry.rs      subscriber
├── routes.rs         probe router + middleware — HTTP, nothing else ever
├── state.rs          AppState — one field per slice
├── error.rs          AppError → ApiError (probes) and → tonic::Status (gRPC)
├── health.rs         liveness / readiness
│
│  ── designed below ──
│                     (ids, SortKey, BlockKind, Op come from `libs/domain`)     §3
│
├── pages/            mod.rs · model.rs · repo.rs · grpc.rs                      §4
├── blocks/           mod.rs · model.rs · repo.rs                                §5
├── ops/              mod.rs · repo.rs · apply.rs · grpc.rs        §5.1 · Phase 1 only
├── tree/             mod.rs · service.rs                                        §6
└── outbox/           mod.rs · repo.rs · poller.rs                               §7
```

`grpc.rs` replaces what would have been `handlers.rs`: it holds the
`impl PageService for PageApi` block. Same rule applies — it extracts, delegates, and maps
errors, and contains no business logic.

**Rules that bind these files** (`PROJECT_STRUCTURE.md` §5):

- No `service.rs` for plain CRUD. `pages/` does **not** get one; `tree/` is *only* a
  service because it spans aggregates.
- The repository trait and its Postgres implementation live in the **same file**.
- Handlers contain no business rules — extract, delegate, map errors.
- One struct with stacked derives, never `PageRow` → `Page` → `PageDto`.
- `blocks/` gets no handlers. **Blocks change only through ops** (RFC-002 §1) — the write path is
  `ops/`, never block CRUD. `blocks/` stays a read slice at every phase; what changes at Phase 3 is
  *who produces the ops*, not how blocks are written.

---

## 3. Domain types — in `libs/domain`, not in this service

Newtypes over primitives so the compiler rejects a `BlockId` where a `PageId` belongs
(`DATA_MODEL.md` §7).

**They live in `libs/domain`, not in `src/domain.rs`.** The *extract on the third use* rule
(`PROJECT_STRUCTURE.md` §5) does not apply here: `wasm/editor` produces a block tree that this
service stores, so both must agree on `BlockKind`, `SortKey`, and the ids **exactly**. A duplicated
type that crosses a serialization boundary is a bug, not a duplication.

What stays in this service is anything only this service has: `MaterialisedPath` (an LTREE detail),
`Title`, `LifecycleState` — put them in the slice that owns them, not in a shared crate.

| Type | Wraps | Invariant to enforce | Enforced where |
|---|---|---|---|
| `PageId`, `BlockId`, `UserId` | `Uuid` (v7) | Time-ordered; distinct types | Constructor |
| `Title` | `String` | Trimmed, non-empty, no control characters, ≤ `MAX_TITLE_LEN` | `TryFrom<String>` |
| `SortKey` | `String` | Non-empty, restricted alphabet, never ends on the lowest digit | `TryFrom<String>` |
| `MaterialisedPath` | `String` | Valid LTREE labels, no empty label, depth-limited | `TryFrom<String>` |
| `LifecycleState` | enum | `active` \| `deleting` \| `deleted` | DB `CHECK` + type |

**The decision to make first:** validation belongs in `TryFrom`, and `TryFrom` has to be
*unskippable*. A newtype whose field can be built by any code in the crate has no
invariant. Look at `#[serde(try_from = "String")]` — it makes deserialization itself the
enforcement point, so a handler cannot forget to validate. That interacts with what
`#[derive(sqlx::Type)]` and `#[sqlx(transparent)]` need on the same struct.

Required surface — pin it with tests before implementing:

```
PageId::new() -> Self                MaterialisedPath::root(PageId) -> Self
PageId::from_uuid(Uuid) -> Self      MaterialisedPath::child(&self, PageId) -> Self
PageId::as_uuid(self) -> Uuid        MaterialisedPath::parent(&self) -> Option<Self>
                                     MaterialisedPath::is_ancestor_of(&self, &Self) -> bool
SortKey::first() -> Self             MaterialisedPath::depth(&self) -> usize
SortKey::key_between(Option<&Self>, Option<&Self>) -> Result<Self, AppError>
SortKey::as_str(&self) -> &str
```

---

## 4. `pages/` slice

### `model.rs`

One `Page` struct carrying `FromRow + Serialize` — the row *is* the response
(`PROJECT_STRUCTURE.md` §5.1). Fields mirror `docs.pages` exactly (`DATA_MODEL.md` §4).

Request types: `CreatePage { title, parent_id?, after? }`, `RenamePage { title }`,
`ReparentPage { parent_id?, after? }`, `ListPages { parent_id?, after?, limit? }`, and a
`PageList { pages, next_cursor }` envelope.

Two shape decisions the tests will hold you to:

- `deleted_at` is absent from an active page's JSON, and the type still round-trips back
  through `Deserialize`. Skipping a field on the way out without teaching the way in is
  the classic asymmetry bug.
- `created_at` / `updated_at` cross the wire as RFC 3339 strings. The `time` crate does
  **not** do that by default — see `time::serde::rfc3339` and the `serde-well-known`
  feature.

### `repo.rs` — `trait PageRepo` + `struct PostgresPageRepo`

`#[async_trait]`, both in this file. Store it concretely in `AppState`; revisit
`Arc<dyn PageRepo>` when ROADMAP Phase 1's "measure `Arc<dyn>` against monomorphisation"
task comes up, not before.

| Method | Contract |
|---|---|
| `insert(author, &Title, parent_id, &MaterialisedPath, &SortKey)` | One `INSERT … RETURNING`. Select `path::text` — sqlx has no LTREE type |
| `fetch(PageId)` | `Option<Page>`; a missing row is `None`, never an error |
| `list(&ListPages)` | Keyset pagination on `(parent_id, sort_key)`, excludes deleted |
| `siblings_around(parent_id, anchor)` | The two keys `key_between` must land between |
| `rename(PageId, &Title)` | Sets `updated_at`; leaves `created_at` alone |
| `reparent(PageId, parent_id, &MaterialisedPath, &SortKey)` | **This row only** — descendants are `tree/`'s job |
| `soft_delete(PageId)` | `lifecycle_state = 'deleting'`, `deleted_at = NOW()` |

### `grpc.rs`

`impl page_service_server::PageService for PageApi` — six RPCs, thin. The proto contract,
status codes, and semantics are in `docs/api/pages.md`.

Three things this layer owns, and only this layer:

- **`tonic::Request<T>` → domain types.** The proto uses `string` for ids and
  `google.protobuf.Timestamp` for times; `TryFrom` at this boundary is what stops an
  unvalidated string reaching a repo.
- **`AppError` → `tonic::Status`.** Add `impl From<AppError> for Status` next to the
  existing `IntoResponse` — same one-way discipline, and `Database` still collapses to a
  detail-free `INTERNAL` after logging the cause.
- **Actor identity from metadata.** `created_by` never comes from a request field.
  `ARCHITECTURE.md` §3 has the gateway verifying the JWT and injecting the actor before
  forwarding, so it arrives as the `actor-id` metadata key — a client that could name its
  own `created_by` could forge authorship. Until the gateway exists (Phase 9), read the
  key and fall back to a fixed development actor.

An interceptor is the right home for actor extraction and deadline checks once there is
more than one service — write it inline first, extract on the third use.

---

## 5. `blocks/` slice

A read projection over `docs.blocks`, which does not exist in `migrations/0001` yet —
adding it is part of this slice.

- `Block` mirrors the table, with `content: serde_json::Value` and `content_version: i16`.
- `BlockKind` is a `snake_case` enum stored as `TEXT`: `paragraph`, `heading_1..3`,
  `quote`, `toggle`, `bulleted_list`, `numbered_list`, `todo_list`, `code`, `image`,
  `divider`.
- `BlockKind::accepts_spans()` / `accepts_children()` encode the RFC-001 §1 grammar.
  `code`, `image`, and `divider` carry no spans — code is never formatted.
- `trait BlockRepo`: `list_for_page(PageId)`, `fetch(BlockId)`.

`content` stays `Value` at the storage boundary and becomes a tagged enum at the domain
boundary. `content_version` is per row and additive-only (ADR-001 seam #2): old rows keep
old shapes forever, so a migration never rewrites JSONB.

---

## 5.1 `ops/` slice — Phase 1 only, and it moves at Phase 3

The write path for block content. **Block-granular ops only** (RFC-002 §2.1) — `InsertText` and
friends need a rope, and the rope is Phase 3.

```rust
#[async_trait]
pub trait OpLog: Send + Sync {
    /// Append and apply in ONE transaction, with the outbox row. Either all
    /// three land or none do — the same boundary §7 describes.
    async fn apply_batch(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        page: PageId,
        actor: ActorId,
        kind: ActorKind,
        group: Option<UndoGroup>,
        ops: Vec<Op>,
    ) -> Result<Vec<OpId>, AppError>;
}
```

| Rule | Why |
|---|---|
| **Every op passes `can_apply(op, actor)`** | One auditable chokepoint (RFC-002 §5). Trivial in Phase 1 — it becomes the typestate in Phase 13, so shape the signature for it now |
| **Append, apply, and enqueue in one transaction** | Otherwise the log and the projection can disagree, which breaks `DATA_MODEL.md` §1's central rule |
| **`SetBlockContent` carries `prev_spans`** | Invertibility is designed in, not discovered in Phase 5 |
| **Dedup on client-generated `OpId`** | A retried request must not double-apply. UUIDv7 from the client (`docs/api/pages.md` §3) |

`grpc.rs` exposes `ApplyOps(page_id, ops) → Vec<OpId>`. **One RPC, batched** — not one call per
keystroke; the browser debounces and sends a group.

> **This slice is temporary and that is fine.** At Phase 3 the table and this slice move to
> `collaboration-service`; `apply.rs` survives the move because `document-service` still needs it to
> materialise `blocks` from events. Write it as a pure function over `(blocks, op) → blocks` and the
> handover costs almost nothing.

---

## 6. `tree/` slice

The only place a `service.rs` is justified, because every function here spans more than
one page row.

| Function | Contract |
|---|---|
| `subtree(&PgPool, PageId)` | `WHERE path <@ $1` — GiST index, no recursive CTE |
| `ancestors(&PgPool, PageId)` | `WHERE path @> $1`, ordered by depth — the breadcrumb |
| `move_subtree(&mut Transaction, PageId, Option<PageId>)` | Reject cycles **first**, then rewrite descendant paths in one `UPDATE` |
| `cascade_soft_delete(&mut Transaction, PageId)` | Marks the whole subtree; returns rows touched |
| `would_create_cycle(&MaterialisedPath, &MaterialisedPath)` | Pure, no I/O — testable without a database |

**The trap:** ancestry is a comparison over whole labels, not a string prefix. `abc` is
not an ancestor of `abcdef`. Write that case explicitly — it is the one a prefix comparison gets wrong.

Transaction boundary: a reparent that moves 400 descendants is one transaction. Take the
`&mut Transaction` rather than the pool so the caller owns the boundary — that is what
lets the handler write an outbox row in the same commit.

---

## 7. `outbox/`

Postgres write plus NATS publish is a dual write with no distributed transaction. If the
commit succeeds and the publish fails, the event is gone: search never indexes it and
history has a hole (`DATA_MODEL.md` §4 Outbox).

- The event row is written **in the same transaction** as the page change.
- A poller claims rows with `FOR UPDATE SKIP LOCKED` so replicas never collide.
- The partial index `WHERE published_at IS NULL` keeps the scan small forever.
- Delivery is at-least-once. Consumers must be idempotent; the op id is the dedup key.

### The two halves have different triggers

| Half | Build it | Because |
|---|---|---|
| Same-transaction **write** | **Phase 1**, with the first mutating RPC | Retrofitting means revisiting every write path, and you can never know which events were lost before it existed |
| The **poller** | **Phase 4**, with the first subscriber | Until `diagnostics-service` subscribes there is nothing to publish to — NATS is not even in `docker-compose.yml` yet |

Rows accumulating unpublished in Phase 1 is the correct state, not a bug. The table is a
durable buffer; the poller drains it whenever it arrives, including for rows written months
earlier. That property is the reason this pattern beats publishing inline.

The outbox tests split along the same line: the first five need only the
write half, the last three exercise `SKIP LOCKED` claim semantics in raw SQL and need no
NATS at all.

Read: [microservices.io — Transactional Outbox](https://microservices.io/patterns/data/transactional-outbox.html).

---

## 8. Error mapping

`AppError` is internal, `ApiError` is the wire shape, and the `From` chain runs one way.
The mapping in `error.rs` is already written; these are the rules for *choosing* a variant.

| Situation | Variant | gRPC status | Gateway HTTP |
|---|---|---|---|
| Page id not found, or soft-deleted | `NotFound("page")` | `NOT_FOUND` | 404 |
| Title empty, oversized, malformed sort key | `Validation { field, reason }` | `INVALID_ARGUMENT` | 422 |
| Reparent under own descendant | `Conflict` | `FAILED_PRECONDITION` | 409 |
| Any `sqlx::Error` | `Database` | `INTERNAL` | 500 |

`FAILED_PRECONDITION` rather than `ABORTED`: gRPC's convention is that `ABORTED` invites a
retry, and these are client bugs that fail identically on retry.

A database message must never reach a caller. The detail goes to the log inside the
request span; the caller gets a detail-free status and the trace id correlates the two.

`error.rs` therefore grows a second conversion — `IntoResponse` for the probe router,
`From<AppError> for tonic::Status` for the service — and both must stay one-way. An
inbound status must never become an `AppError`.

---

## 9. Algorithms — named, not written

These are ROADMAP § Rust, DSA & Concepts Map items. The invariants and failing tests are provided; the
implementations are the exercise.

| Algorithm | Invariant | Reference |
|---|---|---|
| Fractional indexing (`key_between`) | Insertion order equals sort order; the gap between two keys is always subdividable; append does not grow keys linearly | [Figma — Realtime Editing of Ordered Sequences](https://www.figma.com/blog/realtime-editing-of-ordered-sequences/) |
| LTREE path maintenance | `path` always matches the `parent_id` chain, for every row, after every move | [PostgreSQL LTREE](https://www.postgresql.org/docs/current/ltree.html) |
| Cycle detection on reparent | A page is never its own ancestor; label-wise comparison, not prefix | — |
| Keyset pagination | A page boundary never skips or repeats a row under concurrent insertion | [Use the Index, Luke — Paging](https://use-the-index-luke.com/no-offset) |
| Outbox claim | Two concurrent pollers never claim the same row | `FOR UPDATE SKIP LOCKED` |

---

## 10. Test map

**Write these before the code they cover** (`agents.md` § stage 1). The table is the coverage the
design needs, not a suite that exists — nothing is written yet.

One technique worth knowing: **Cargo does not compile subdirectories of `tests/`**, so a
`tests/pending/` directory holds tests that do not yet compile without breaking the build. Move a
file up to `tests/` when the code it needs exists.

| File | Covers | Needs |
|---|---|---|
| `domain.rs` | Title/path/sort-key validation, ancestry | `domain.rs` |
| `fractional_index.rs` | `key_between` laws, including two `proptest` properties | `SortKey` |
| `pages.rs` | Full `PageService` surface, 30 cases | `libs/proto` + `pages/` |
| `tree.rs` | Cycle rules, subtree, breadcrumb, cascade | `tree/` |
| `blocks.rs` | Grammar, projection reads, JSONB round-trip, cascade | `blocks/` + migration |
| `outbox.rs` | Same-transaction write, `SKIP LOCKED` claim, at-least-once | `outbox/` + migration |
| `concurrency.rs` | Racing creates, reparents, and deletes | `pages/` + `tree/` |

`tests/common/mod.rs` is the shared harness. It serves `PageService` over an in-memory
duplex stream and returns a generated `PageServiceClient` — no ports, no gateway, no
Docker for the transport itself. Database tests use `#[sqlx::test]`, which creates a
throwaway database and runs `migrations/` against it — real Postgres, never a mock
(`agents.md` § Configuration & portability).

```
   tokio::io::duplex()  →  tonic Server::builder().add_service(…).serve_with_incoming()
                        →  Channel via Endpoint::connect_with_connector
                        →  PageServiceClient
```

That shape is the gRPC equivalent of driving an Axum router with `oneshot`, and it is
worth writing once carefully — every later service tests the same way.

`#[ignore = "reason"]` on a written-but-not-yet-passing test is the other half of that technique —
the suite stays green and the reason documents what is missing.

---

## 11. Build order

Bottom-up, because each step makes the next one's tests compile.

1. `domain.rs` — types, validation, `key_between`. Activate `domain.rs` and
   `fractional_index.rs`. No database, no proto, no service needed.
2. `migrations/0002` — `blocks`, `ops`, `outbox` per `DATA_MODEL.md` §4.
3. `libs/proto` — `document.proto` per `docs/api/pages.md` §1, `tonic-build` in `build.rs`.
4. `pages/model.rs` + `repo.rs` — insert and fetch first.
5. `pages/grpc.rs` + registration in `serve`. Activate `pages.rs`.
6. `tree/service.rs` — cycle check before anything that writes. Activate `tree.rs`.
7. `blocks/` — projection reads. Activate `blocks.rs`.
7b. `ops/` — `apply.rs` as a **pure function** first (unit-testable, no database), then the repo and
    `ApplyOps`. This is the write path the editor needs.
8. `outbox/` — same-transaction write, then the poller. Activate `outbox.rs`.
9. Activate `concurrency.rs` — it needs everything above and will find what the
   single-threaded tests could not.
10. CI: fmt, clippy, test, `cargo sqlx prepare --check`, `protoc` codegen check.

Steps 1 and 2 need no proto and no running service; start there.

### 11.1 The cloud increment for this phase

Phase 1 is not finished when the tests pass — it is finished when `document-service` is
running in Google Cloud, provisioned by Terraform (`CLOUD_ROADMAP.md` §2).

| Terraform resource | Replaces |
|---|---|
| `google_storage_bucket` (state) | — configure the backend first |
| `google_sql_database_instance` + `google_sql_database` | the compose Postgres |
| `google_storage_bucket` (files) | MinIO |
| `google_secret_manager_secret` (DB password) | `.env` |
| `google_artifact_registry_repository` | local image builds |
| `google_cloud_run_v2_service` | `cargo run` |
| `google_billing_budget` | nothing — add it first, at $10 |

Two things to verify in cloud that local testing cannot prove: the service starts when
its config comes entirely from environment variables and Secret Manager rather than
`config.yaml`, and `SIGTERM` from the platform drains cleanly during a revision rollout.
Both are already implemented in the startup path — this is where you find out whether
they work.

Then `terraform destroy`.

---

## 12. Implementation notes — the things that will bite

None of these are visible from the design docs, and each costs an afternoon to discover.

### `sort_key` must be `COLLATE "C"`

A fractional index assumes **byte-wise** lexicographic ordering — the same order
`key_between` reasons about in Rust. Postgres orders `TEXT` using the database collation,
which on a `en_US.UTF-8` cluster ignores case and punctuation in ways that do not match
byte order.

```sql
sort_key TEXT COLLATE "C" NOT NULL
```

Without this, `ORDER BY sort_key` and your Rust comparison disagree on a subset of keys,
and pages silently reorder. The sibling index must use the same collation or it will not
be used for the sort.

**Decide the alphabet before writing `key_between`** — digits and lowercase letters
(`0-9a-z`, base 36) is the usual choice and keeps keys short. Whatever you pick, the
`TryFrom<String>` validator enforces it.

### LTREE labels reject hyphens

An ltree label is alphanumerics and underscores only. A UUID's canonical form has hyphens
and cannot be a label. Strip them, and prefix so a label never starts ambiguously:

```
  018f2b1c-0000-7000-8000-000000000000  →  p018f2b1c000070008000000000000000
```

`MaterialisedPath::root` and `::child` own that transformation; nothing else should know
the encoding.

### sqlx has no LTREE type

Cast in both directions. Read with `path::text`, write with a cast or `text2ltree`:

```sql
SELECT id, path::text AS path, ... FROM docs.pages WHERE id = $1
INSERT INTO docs.pages (id, path, ...) VALUES ($1, $2::ltree, ...)
```

`query_as!` will infer `String` for `path::text`, which is what `MaterialisedPath`'s
`#[sqlx(transparent)]` wants.

### Nullable `parent_id` needs `IS NOT DISTINCT FROM`

Listing root pages and listing children are the same query, but `parent_id = NULL` is
never true. Binding an `Option<PageId>` to a plain `=` silently returns zero rows for
roots:

```sql
WHERE parent_id IS NOT DISTINCT FROM $1
```

One query, both cases, and the partial index still applies.

### `cargo sqlx prepare` before CI exists

ADR-003 commits to compile-time checked queries, so `query!` needs a live `DATABASE_URL`
at **build** time. Run `cargo sqlx prepare` to write `.sqlx/`, commit it, and CI compiles
without a database. `cargo sqlx prepare --check` in CI is what catches a query edited
without regenerating (ROADMAP Phase 1 checklist).

Check that `.env`'s `DATABASE_URL` and `docker-compose.yml`'s Postgres credentials
actually agree — `#[sqlx::test]` reads the former and connects to the latter.

### The transaction boundary is the outbox's

`create`, `reparent`, and `delete` each write a page row **and** an outbox row that must
share one commit (LLD §7). Those repo methods therefore take `&mut Transaction<'_, Postgres>`,
not `&PgPool`. Read-only methods take the pool.

Getting this wrong is invisible until Phase 4, when the first subscriber starts missing
events that Postgres definitely committed.

### Phase 1 is single-user by assumption, not by enforcement

Nothing stops two browser tabs from both calling `ApplyOps` on the same page. Both succeed, and
`SetBlockContent` silently clobbers — **last write wins at block granularity.**

The op log keeps both writes, so nothing is *lost* and the history shows exactly what happened. Only
the projection takes the last one.

**This is an accepted gap, not an oversight.** Phase 3 replaces the whole mechanism — rope, CRDT,
one owner per page — so hardening it now builds something that gets deleted. Two things follow:

- **Do not add optimistic concurrency to work around it.** A `version` check on `blocks` would be
  throwaway code and would give the false impression the problem is solved
- **Do not confuse this with `content_version`.** That column versions the *content shape* for
  additive JSONB evolution (ADR-001 seam #2). It is not a concurrency token, and using it as one
  would break shape migration later

If a demo needs the rough edge covered before Phase 3, the cheap honest answer is a **UI-level
warning** when a second session opens the same page — a Redis presence key, not a database
constraint.

### Never omit the id on insert

`DEFAULT uuidv7()` is a PostgreSQL 18 built-in and managed Postgres lags (ADR-008).
`PageId::new()` generates the id in Rust; the column default is a convenience for
hand-written SQL only.
