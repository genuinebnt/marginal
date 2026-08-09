# Marginal — Project Structure & Code Architecture

**Governing document.** Applies identically to all seven services.
**Principle:** keep the dependency rules, drop the ceremony.

---

## 1. Why This Document Exists

An earlier attempt used **layer-first** directories — the textbook Clean Architecture shape:

```
src/
├── domain/entities/block.rs
├── domain/repository.rs
├── application/usecases/block/{command,port,service,mod}.rs   ← port.rs abstracted
├── infrastructure/database/postgres.rs                          another trait
└── presentation/handlers/page.rs
```

Adding **one field to a block** meant editing six files across four directories: the entity, the repository trait, the row mapping, the command struct, the service, and the handler.

Nothing there is wrong. It is priced for twenty maintainers on a decade-old codebase, and it was being paid by one person learning Rust. Progress stalled on it.

The fix is not to abandon Clean Architecture. It is to **organise directories by feature instead of by layer**, keeping every dependency rule that earns its cost.

---

## 2. The Rule in One Picture

Dependencies point inward — that never changes. What changes is that a **slice** is the unit of organisation.

```
                    ╔══════════════════════════════════════════════╗
                    ║      libs/domain  (shared, zero deps)         ║
                    ║  ids · BlockKind · Op · events · errors       ║
                    ║        wasm32-compatible — required           ║
                    ╚═══════════════════════▲══════════════════════╝
                                            │  depends on
        ┌───────────────────────────────────┼───────────────────────────────┐
        │                                   │                               │
┌───────┴────────┐                ┌─────────┴──────┐              ┌────────┴───────┐
│  pages/        │                │  blocks/       │              │  tree/         │
│  mod.rs        │                │  mod.rs        │              │  mod.rs        │
│  model.rs      │                │  model.rs      │              │  service.rs    │
│  repo.rs       │                │  repo.rs       │              │  (LTREE walk,  │
│  handlers.rs   │                │  handlers.rs   │              │   cascade)     │
│  service.rs?   │                │                │              │                │
└───────┬────────┘                └────────┬───────┘              └────────┬───────┘
        └───────────────────────────────────┼───────────────────────────────┘
                                            │  wired in
                                  ┌─────────▼──────────┐
                                  │  routes.rs         │
                                  │  state.rs          │
                                  │  error.rs          │
                                  │  main.rs           │
                                  └────────────────────┘

   A slice owns its whole vertical: HTTP in → logic → SQL out.
   Slices never import each other's internals. Shared behaviour
   lives in a slice of its own (tree/) or in libs/.
```

---

## 3. The Service Template

**Every service has this shape.** Uniformity is the point — you should be able to open `search-service` having only read `document-service` and know where everything is.

```
services/<name>/
├── Cargo.toml
├── config.yaml                  safe local defaults; secrets via env only
├── migrations/                  this service's schema only
│   └── 0001_init.sql
├── tests/                       real Postgres via #[sqlx::test]
│   ├── pages.rs
│   └── blocks.rs
└── src/
    ├── main.rs                  config → telemetry → state → router → serve
    ├── config.rs                Settings struct — the definitive schema
    ├── state.rs                 AppState { pg, nats, redis, repos }
    ├── error.rs                 AppError (internal) → ApiError (HTTP boundary)
    ├── routes.rs                every route in one place, readable at a glance
    │
    ├── pages/                   ← a vertical slice
    │   ├── mod.rs               pub use — the slice's public surface
    │   ├── model.rs             Page + request/response types
    │   ├── repo.rs              trait PageRepo + PostgresPageRepo (SAME file)
    │   ├── handlers.rs          axum handlers — thin, no business logic
    │   └── service.rs           ONLY when real logic exists (§5.3)
    │
    ├── blocks/                  ← identical shape
    │   ├── mod.rs  model.rs  repo.rs  handlers.rs
    │
    └── tree/                    ← cross-aggregate slice
        ├── mod.rs
        └── service.rs           LTREE traversal, cascade delete
```

### Request flow

```
   HTTP POST /pages
        │
        ▼
  ┌──────────────┐   route table only — no logic
  │  routes.rs   │
  └──────┬───────┘
         ▼
  ┌──────────────┐   extract, validate, map errors.
  │pages/handlers│   NEVER business rules.
  └──────┬───────┘
         │
         ├─── plain CRUD? ────────────────────────┐
         │                                         │
         ▼ real logic exists                       ▼
  ┌──────────────┐                        ┌──────────────┐
  │pages/service │  transactions,         │  pages/repo  │
  │              │  events, can_apply ───▶│ Arc<dyn Repo>│
  └──────────────┘                        └──────┬───────┘
                                                 ▼
                                          ┌──────────────┐
                                          │  PostgreSQL  │
                                          └──────────────┘

  Handlers may call repo directly for plain CRUD.
  Skipping an empty service layer is not a violation — it is the point.
```

---

## 4. Non-Negotiable

| Rule | Why it earns its cost |
|---|---|
| **A trait for every external dependency** — DB, cache, broker, object store | `CLOUD_PORTABILITY.md`: Cloud SQL↔Postgres, GCS↔MinIO, Memorystore↔Redis swap behind one trait. Also what makes `#[sqlx::test]` viable |
| **Trait and impl in the same `repo.rs`** | A trait in a separate file from its only implementation is ceremony |
| **`libs/domain` has zero external deps** | `libs/diagnostics` and the editor core compile to `wasm32` |
| **No cross-service database access** | ADR-001. Data crosses as NATS events, never as a join |
| **`AppError` internal, `ApiError` at the boundary** | One-way `From`. Internal errors never leak to HTTP |
| **One `config.yaml` per service, secrets via env only** | Missing variable ⇒ fail to start, loudly |
| **Handlers contain no business logic** | The single structural rule that reliably prevents rot |
| **Every mutation passes `can_apply(op, actor)`** | ADR-001 seam. One auditable authorization chokepoint |

---

## 5. Deliberately Relaxed

### 5.1 Two representations, not four

```
   PURIST                              PRAGMATIC
   ┌──────────┐                        ┌──────────┐
   │ BlockRow │  DB                    │          │  FromRow
   └────┬─────┘                        │  Block   │  + Serialize
        │ From                         │          │  + ToSchema
   ┌────▼─────┐                        └──────────┘
   │  Block   │  domain                      │
   └────┬─────┘                              │ split ONE type
        │ From                               ▼ only when the shapes
   ┌────▼─────┐                     ┌────────────────┐  truly diverge
   │ BlockDto │  API                │ BlockResponse  │
   └──────────┘                     └────────────────┘
   3 mapping hops per field          0 hops
```

```rust
#[derive(sqlx::FromRow, Serialize, Deserialize, utoipa::ToSchema)]
pub struct Block { /* ... */ }
```

This couples the API shape to the DB shape. **Accept that until it hurts**, then split only the type that hurts. The safety net is not the type system here: `openapi.json` is a committed artifact, so a schema change leaking into a response appears as a reviewable diff.

This single concession saves more time than everything else in this document.

### 5.2 No trait abstracting another trait

The old `usecases/block/port.rs` existed to abstract `domain/repository.rs`, which abstracted Postgres. **One trait per dependency.**

### 5.3 No service layer for CRUD

```
  ┌──────────────────────────────────────────────┬─────────────┐
  │ Transaction spans two or more aggregates     │  service.rs │
  │ An event must publish after the write        │  service.rs │
  │ An op must pass can_apply                    │  service.rs │
  │ Business rules beyond field validation       │  service.rs │
  ├──────────────────────────────────────────────┼─────────────┤
  │ Insert one row and return it                 │  handler →  │
  │ Fetch by id                                  │  repo       │
  │ List with pagination                         │  directly   │
  └──────────────────────────────────────────────┴─────────────┘
```

### 5.4 No `usecases/` directory

A use case that is one function **is a function**: `pub async fn create_block(...)` in `service.rs`. No struct, no `Command` type, no `mod.rs` per operation.

### 5.5 Split on pain, not on principle

```
  STAGE 1 — new slice, day one
  links/
  ├── mod.rs
  └── model.rs          handlers + repo live here too. ~120 lines. Fine.

  STAGE 2 — queries multiply
  links/
  ├── mod.rs  model.rs
  ├── repo.rs           ← extracted when SQL crowded model.rs
  └── handlers.rs

  STAGE 3 — real logic appears
  links/
  └── + service.rs      ← extracted when resolution needed a transaction
                          plus a NATS event

  A 250-line file is fine. Six 40-line files are not.
  Splitting is a response to friction, never a starting position.
```

### 5.6 Tests next to the code

`#[cfg(test)] mod tests` at the bottom of the file for unit tests. Integration tests in `tests/` with `#[sqlx::test]` against real Postgres. **No mocking of infrastructure.**

---

## 6. Where Shared Code Goes

The default is **not** to share. Duplication across services is cheaper than a shared crate that couples deployments.

```
  ┌──────────────────────────────────────────────────────────────────┐
  │  Domain primitive (id newtype, BlockKind, Op, event, error)?      │
  │      └─ yes ──▶ libs/domain        zero deps, wasm32-safe         │
  │                                                                   │
  │  Cross-cutting plumbing (telemetry, config, AppError)?            │
  │      └─ yes ──▶ libs/infra                                        │
  │                                                                   │
  │  Wire contract between two services?                              │
  │      └─ yes ──▶ libs/proto         tonic + prost                  │
  │                                                                   │
  │  Document model — rope, marks, ops, input rules?                  │
  │      └─ yes ──▶ libs/doc           wasm32; the editor core        │
  │                                                                   │
  │  A diagnostic analyzer?                                           │
  │      └─ yes ──▶ libs/diagnostics   wasm32; pure, no infra         │
  │                                                                   │
  │  Test scaffolding?                                                │
  │      └─ yes ──▶ libs/test-utils    Testcontainers, TestContext    │
  │                                                                   │
  │  Anything else?                                                   │
  │      └─ KEEP IT IN THE SERVICE. Duplicate if two services need    │
  │         it. Extract on the third, never on the second.            │
  └──────────────────────────────────────────────────────────────────┘
```

### Vocabulary, not entities

The decision tree above is easy to over-apply. The sharper test, because it settles almost every
case in one line:

> **`libs/domain` holds the *vocabulary*, never the *entities*.**

| Vocabulary — shared | Entity — stays in the slice that owns it |
|---|---|
| `PageId` · `BlockId` · `OpId` · `ActorId` | `Page` — only `document-service` constructs one |
| **`BlockKind`** — the editor produces it, the service stores it | **`Block`** — the row, with `page_id`, `path`, `content` |
| `SortKey` — `lower.rs` assigns them, the service orders by them | `Title` · `MaterialisedPath` (an LTREE detail) · `LifecycleState` |
| `Op` · domain events · `DomainError` | `User` · `Email` · `Password` — `auth-service`'s alone |

`libs/domain/src/block.rs` is the pattern in miniature: it holds **`BlockKind`**, not `Block`.

**Two tests, and a type must pass one:**

1. **Count the consumers.** Two or more crates → `libs/domain`. Exactly one → that crate.
2. **Does it cross a serialization boundary?** If `wasm/editor` produces it and a service stores
   it, they must agree *exactly* — so it is shared even at two consumers. This is the one case
   where *extract on the third use* does not apply: **a duplicated type that crosses a wire is a
   bug, not a duplication.**

`Page` fails both — one constructing crate, and it reaches the browser as a proto message rather
than as a Rust type. So there is no `page.rs` in `libs/domain`, and there should not be.

---

## 7. Repository Root

**Three Rust buckets, split by deployment target — not by layer.**

```
marginal/
├── libs/               shared Rust · imported by BOTH services and wasm
│   ├── domain/         ids, BlockKind, Op, events, errors    zero deps · wasm32-clean
│   ├── doc/            block tree, rope, anchors, marks, ops           · wasm32-clean
│   ├── diagnostics/    analyzers, symbol table, incremental            · wasm32-clean
│   ├── infra/          telemetry, config, AppError/ApiError, define_id!  native only
│   ├── proto/          protobuf definitions (tonic + prost)             native only
│   ├── macros/         #[derive(...)] proc macros (syn + quote)         native only
│   └── test-utils/     Testcontainers wrappers, TestContext             native, dev-dep
│
├── services/           native binaries · one crate per service (11, ADR-009)
│
├── wasm/               browser binaries
│   └── editor/         cdylib+rlib · #[wasm_bindgen] exports · serde_wasm_bindgen
│                       AND the editor front end: lex · parse · lower ·
│                       normalise · sanitise — one consumer, so it lives here
│                       AND rendering: syntect + two-face
│
├── web/                React + TypeScript SPA — NOT in the Cargo workspace (ADR-004)
├── reference/          Go reference implementations — NOT in the workspace (ADR-005)
├── deploy/             Terraform IaC (ADR-008), prometheus.yml
├── docker-compose.yml
└── docs/
```

### `wasm/editor` is the `wasm-bindgen` boundary, made into a file

ADR-004 requires that the boundary be **designed, not discovered**. Without a crate to hold it, it
gets discovered — one `#[wasm_bindgen]` at a time, scattered across whichever module needed it.

| It owns | Why here and not in `libs/doc` |
|---|---|
| Every `#[wasm_bindgen]` export | The TypeScript-facing API is one reviewable surface |
| `serde_wasm_bindgen` marshalling | The crossing cost lives at the crossing |
| **The editor front end** — `lex`, `parse`, `ast`, `lower`, `normalise`, `sanitise` | **Exactly one consumer.** A module in a shared crate that only one crate uses is mislabelled — and it would make two backend services link an XSS-boundary HTML parser they never invoke |
| **`syntect` + `two-face`** | Highlighting is *rendering*, not modelling, and the project's largest dependency (`lld/libs-doc.md` §2) |

`crate-type = ["cdylib", "rlib"]`, so **all of it tests natively** — `cargo test` needs no wasm
toolchain. If a server-side consumer for the parser ever appears, extract `libs/editor` **then**;
that is the third-use rule working rather than being ignored.

**One crate under `wasm/`, not several.** The browser loads one module; a second crate means a
second module and a second boundary to keep consistent.

### Workspace mechanics

```toml
members         = ["libs/*", "services/*", "wasm/editor"]
default-members = ["libs/*", "services/*"]     # root `cargo test` skips the cdylib
```

Without `default-members`, a root build compiles a `cdylib` for the host target — useless at best.
With it, `cargo check -p editor-wasm --target wasm32-unknown-unknown` still works, which is what the
CI gate needs.

### What exists, and what is deliberately absent

`ROADMAP.md` § Phase 0 is binding: **foundation work is *pulled in* by the first service that needs
it, never built up front.** So the tree above is the target, not the current state.

| Crate | Created | Why |
|---|---|---|
| `libs/domain` | **yes** | Phase 1 step 1, and `libs/doc` + `document-service` must agree on `BlockKind` and the ids **exactly** — a duplicated type that crosses a wire is a bug, not a duplication, so the *extract on the third use* rule does not apply to shared vocabulary |
| `libs/doc` | **yes** | Phase 1 — `tree.rs` first. **Model only**; the front end is in `wasm/editor` |
| `libs/proto` | **yes** | Phase 1 step 3 |
| `wasm/editor` | **yes** | The boundary needs a home before the first `#[wasm_bindgen]`, or it gets discovered instead of designed |
| `libs/diagnostics` | **no** | Phase 4. Creating it now is the speculative abstraction §5 forbids |
| `libs/infra` | **no** | Extract on the **third** consumer. `auth-service` is the second and copies (`lld/auth-service.md` §1) |
| `libs/macros`, `libs/test-utils` | **no** | Pulled when something needs them |

### The wasm line

`libs/domain`, `libs/doc`, and `libs/diagnostics` must stay `wasm32`-clean and
infrastructure-free — they are the editor core, they run in the browser, and that purity is what
keeps them Miri-reachable and fuzzable.

> **Adding a crate to `libs/` means stating which side of that line it is on.** If it is clean, add
> it to the gate in the same commit:
> `cargo check -p domain -p doc -p diagnostics --target wasm32-unknown-unknown`
>
> A rule enforced only in this document is a rule that gets broken by an innocent import.

**The gate earned its place on its first run.** `uuid` does not compile for `wasm32` without an
explicit randomness source — the browser has no `getrandom` syscall — so `libs/domain` carries a
target-scoped dependency:

```toml
[target.'cfg(target_arch = "wasm32")'.dependencies]
uuid = { workspace = true, features = ["js"] }
```

Target-scoped, so native builds never link `js-sys`/`wasm-bindgen`; Cargo does not unify features
across targets, which is what makes that safe. **This is the class of failure the gate exists to
catch, and it caught it on day one rather than in Phase 16.**

---

## 8. Checklist for a New Slice

```
  □ Migration written first (the schema is the contract)
  □ utoipa annotation on every handler — docs/api/ is generated, not written
  □ model.rs — one struct, stacked derives
  □ repo.rs  — trait + Postgres impl, same file
  □ handlers.rs — thin: extract, call, map error
  □ routes.rs — wired in
  □ #[sqlx::test] integration test against real Postgres
  □ Mutation paths pass can_apply
  □ NO service.rs unless §5.3 says so
  □ NO new trait unless a second impl or a test needs it
  □ NO libs/ extraction unless §6 says so
```

---

## 9. Anti-Patterns — call these out in review

| Anti-pattern | Fix |
|---|---|
| `usecases/<entity>/<operation>.rs` | One `service.rs` per slice, functions inside |
| `port.rs` abstracting `repository.rs` | Delete it; one trait per dependency |
| `FooRow` → `Foo` → `FooDto` for identical shapes | One struct, stacked derives |
| Trait with one impl and no test need | Concrete type |
| Handler with an `if` on business rules | Move to `service.rs` |
| Six 30-line files in a new slice | Two files; split later |
| `libs/shared` as a dumping ground | §6 decision tree |
| Layer-first directories | Feature-first slices |
| UI or handler mutating the tree directly | Compile it to an `Op` (RFC-002 §1) |
| Infrastructure imported into `libs/doc` or `libs/diagnostics` | Those stay `wasm32`-clean |
| Go-shaped Rust after a reference port | `interface{}`-ish trait objects, channels where ownership transfer is simpler, stringly-typed errors |

---

## 10. Related Documents

| Doc | Governs |
|---|---|
| `adr/ADR-001` | Scope and why each service boundary exists |
| `adr/ADR-002` | Rust depth as primary objective; ordering |
| `adr/ADR-003` | PostgreSQL + sqlx |
| `adr/ADR-004` | React SPA, Rust editor core |
| `adr/ADR-005` | Go reference as answer key |
| `adr/ADR-006` | gRPC pairs |
| `rfc/RFC-001` | Block tree, spans↔rope, input rules, paste |
| `rfc/RFC-002` | Op ISA, invertibility, log versioning, WAL |
| `rfc/RFC-003` | Diagnostics analyzers, symbol table, incrementality |
| `CLOUD_PORTABILITY.md` | Why external dependencies sit behind traits |
| `docs/api/README.md` | Why `utoipa` annotations are mandatory |
