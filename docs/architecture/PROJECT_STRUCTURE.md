# Marginal — Project Structure & Code Architecture

**Governing document.** Applies identically to every service.
**Principle:** keep the dependency rules, drop the ceremony.

---

## 1. Why This Document Exists

The default reach is **layer-first** directories — the textbook Clean Architecture shape:

```
src/
├── domain/entities/block.rs
├── domain/repository.rs
├── application/usecases/block/{command,port,service,mod}.rs   ← port.rs abstracted
├── infrastructure/database/postgres.rs                          another trait
└── presentation/handlers/page.rs
```

Adding **one field to a block** means editing six files across four directories: the entity, the repository trait, the row mapping, the command struct, the service, and the handler.

Nothing there is wrong. It is priced for twenty maintainers on a decade-old codebase, and here it would be paid by one person learning Rust.

The fix is not to abandon Clean Architecture. It is to **organise directories by feature instead of by layer**, keeping every dependency rule that earns its cost.

---

## 2. The Whole Repository

### One directory, flat, named by role

**All Rust lives in `crates/`.** There is no `libs/`, no `services/`, no `wasm/` — a crate's
job is carried by its **name**, not by which bucket it sits in. `document-core` is a library
because it has no `main`; `document-service` is a binary because it does; `editor-wasm` is a
cdylib because its `Cargo.toml` says so. A directory cannot enforce any of that, so it was
only ever a comment with a slash in it.

**A crate name never contains the project name.** `document-core`, not `marginal-doc` — the
repository already says which project it is, and the project may be renamed.

```
marginal/
├── Cargo.toml                 workspace · members = ["crates/*"]
├── crates/
│   └── document-core/         the only crate that exists today
├── web/                       TS SPA — not in the workspace
├── deploy/                    docker-compose, later Terraform
└── docs/
```

**Start with as few crates as the work needs.** Creating one before a second consumer exists is
the speculative abstraction §5 forbids. The `.proto` file and its `build.rs` live inside the
service that owns them until something else needs the generated types.

### What it grows into

Each crate appears when a **trigger** fires — §7 has the full table.

```
marginal/
├── crates/
│   ├── domain/             ← a SECOND crate needs the ids and kinds
│   ├── document-core/      the model: block tree, rope, anchors, marks, ops
│   ├── document-parser/    ← the editor needs to turn typing into a block tree
│   │                         lex · parse · lower · normalise · sanitise
│   ├── proto/              ← a second service needs the generated types
│   ├── editor-wasm/        ← the browser needs to call Rust
│   │                         cdylib+rlib · bindgen boundary · syntect rendering
│   ├── api-gateway/        ← the browser needs REST. THE ONLY REST + WS SURFACE
│   └── document-service/   gRPC + HTTP probes
│
├── web/                    React + TypeScript SPA — NOT in the Cargo workspace
├── deploy/                 Terraform, docker-compose, prometheus.yml
└── docs/
```

**The name carries the role, and the roles are three.** A `*-service` is a binary that binds two
listeners. `editor-wasm` is the one cdylib. Everything else is a plain library. Nothing about
that is enforced by a directory, and pretending otherwise cost a rename.

`document-core` and `document-parser` are split by **who links them**, not by size: the parser
pulls in an HTML parser for `sanitise` and `syntect` for highlighting, and no backend service
should link either. See `lld/document-core.md` §2.

### The transport rule, in three lines

```
   browser  ──REST + WSS──▶  api-gateway  ──gRPC──▶  every other service
                                  │
                                  └── the ONLY translator. Nothing else speaks REST.
```

- **Every service speaks gRPC.** That is the east-west default (ADR-007).
- **Only the gateway speaks REST and WebSocket**, because browsers cannot speak gRPC.
- **Every service also serves HTTP on a second port** — but only `/health` and `/health/ready`,
  because Kubernetes needs them. That is the entire HTTP surface outside the gateway.

So a service binds **two listeners**: gRPC for real traffic, HTTP for probes. Both are three lines
in `lib.rs` and then never change.

---

## 3. Inside a Service

**Every service has this shape.** Uniformity is the point: you should be able to open
`search-service` having only read `document-service` and know where everything is.

```
crates/<noun>-service/
├── Cargo.toml
├── config.yaml           safe local defaults · NO secrets, ever
├── migrations/           this service's schema only
└── src/
    ├── main.rs           thin: parse config, init telemetry, call run()
    ├── lib.rs            run(): state → both listeners → serve → drain on SIGTERM
    ├── config.rs         Settings — the definitive schema; missing var = refuse to start
    ├── telemetry.rs      tracing subscriber
    ├── state.rs          AppState { pool, repos, clients }  — Clone, cheap
    ├── error.rs          AppError → axum Response AND → tonic::Status
    │
    └── pages/            ← a FEATURE SLICE. Owns its whole vertical.
        ├── mod.rs        pub use — the slice's public surface
        ├── model.rs      one struct with stacked derives (§5.1)
        ├── repo.rs       trait PageRepo + PostgresPageRepo — SAME FILE
        └── api.rs        the transport impl: extract → delegate → map
```

**Six root files and N slices.** That is the whole template.

**A slice can be one file.** `api-gateway` owns no data, so its slices have no `model.rs` and no
`repo.rs` — just translation, plus two files the other services do not need:

```
crates/api-gateway/src/
├── main.rs  lib.rs  config.rs  telemetry.rs  state.rs  error.rs
├── auth.rs               JWT verify against cached JWKS — NO call to auth-service
├── clients.rs            one generated gRPC client per upstream
└── pages/api.rs          REST in → gRPC out → REST back
```

### One name for transport: `api.rs`

Not `handlers.rs`, not `grpc.rs`, not `controller.rs`. **A slice has one transport file** and the
transport it speaks is whatever the service speaks.

Split it only when a slice genuinely serves two — which today is only the gateway:

```
    api/
    ├── rest.rs      axum handler
    └── grpc.rs      tonic client call
```

That is §5.5 — *split on pain, not on principle*.

### No `service.rs` unless logic exists

```
   request ──▶ api.rs ──┬── plain CRUD ────────────▶ repo.rs ──▶ Postgres
                        │
                        └── real logic ──▶ service.rs ──▶ repo.rs
                                           (transactions spanning aggregates,
                                            cycle checks, can_apply, sagas)
```

**`api.rs` calling `repo.rs` directly is the normal case, not a shortcut.** An empty service layer
that forwards one call is the thing §5.3 forbids.

### Not everything is a request: `consumer.rs` and `poller.rs`

Three things in this project run without anyone calling them, and they belong **in the slice that
owns them** — not in a `workers/` bucket, which is layer-first thinking wearing a different hat.

```
    blocks/
    ├── mod.rs  model.rs  repo.rs
    └── consumer.rs      ← subscribes to op events, materialises blocks. THE CQRS READ SIDE

    outbox/
    ├── mod.rs  repo.rs
    └── poller.rs        ← FOR UPDATE SKIP LOCKED, publishes to NATS
```

| Kind | What it is | Example |
|---|---|---|
| **`consumer.rs`** | A NATS subscriber. **Idempotent, dedupes on `OpId`** — delivery is at-least-once | `blocks/consumer.rs` replaying ops into the projection |
| **`poller.rs`** | A loop over a table | `outbox/poller.rs` |
| **`worker.rs`** | A periodic task | snapshot writer, tombstone GC |

**`lib.rs` spawns them beside the listeners, and the drain must stop them too.** A `SIGTERM` that
closes the sockets but leaves a poller mid-transaction is a half-shutdown:

```rust
let grpc     = tokio::spawn(serve_grpc(..));
let probes   = tokio::spawn(serve_http(..));
let consumer = tokio::spawn(blocks::consumer::run(state.clone(), shutdown.clone()));
let poller   = tokio::spawn(outbox::poller::run(state.clone(), shutdown.clone()));
// SIGTERM → signal `shutdown` → join all four with a timeout → close the pool
```

### CQRS is between services, not inside one

Worth stating because the instinct is to build it inside a service — separate read and write models
behind one API — and here that would be pure ceremony.

```
   collaboration-service          document-service          search-service
   ────────────────────          ────────────────          ──────────────
   WRITE: ops (the truth)  ──NATS──▶  READ: blocks     ──NATS──▶  READ: index
                                      (consumer.rs)              (consumer.rs)
```

The write side and the read sides are **different services with different databases**. So a slice
keeps one `repo.rs` doing both reads and writes of *its own* data — splitting that would be
inventing a boundary the architecture already draws somewhere else.

**The saga (Phase 8) uses exactly this mechanism**: choreographed, no coordinator, each service a
`consumer.rs` that subscribes and publishes. No new structure.

### Where a transaction begins

**In `api.rs`, not in `repo.rs`.** A repo method takes `&mut Transaction`, never `&PgPool`,
because two writes have to commit together — the row and its outbox event. The handler owns the
boundary because only the handler knows how many writes the request implies.

```rust
let mut tx = state.pool.begin().await?;
let page = state.pages.insert(&mut tx, new).await?;
state.outbox.enqueue(&mut tx, Event::PageCreated { .. }).await?;
tx.commit().await?;
```

---

## 4. Non-Negotiable

| Rule | Why it earns its cost |
|---|---|
| **A trait for every external dependency** — DB, cache, broker, object store | `CLOUD_PORTABILITY.md`: Cloud SQL↔Postgres, GCS↔MinIO, Memorystore↔Redis swap behind one trait. Also what makes `#[sqlx::test]` viable |
| **Trait and impl in the same `repo.rs`** | A trait in a separate file from its only implementation is ceremony |
| **`crates/domain` has near-zero deps** | It and `crates/document-core` compile to `wasm32` — CI-enforced |
| **No cross-service database access** | ADR-001. Data crosses as NATS events, never as a join |
| **`AppError` internal; mapped at the boundary** | One-way `From` to `tonic::Status` and to an axum response. **A database message never reaches a client** |
| **One `config.yaml` per service, secrets via env only** | Missing variable ⇒ fail to start, loudly |
| **`api.rs` contains no business logic** — extract, delegate, map | The single structural rule that reliably prevents rot |
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
  │      └─ yes ──▶ crates/domain        zero deps, wasm32-safe         │
  │                                                                   │
  │  Cross-cutting plumbing (telemetry, config, AppError)?            │
  │      └─ yes ──▶ crates/infra                                        │
  │                                                                   │
  │  Wire contract between two services?                              │
  │      └─ yes ──▶ crates/proto         tonic + prost                  │
  │                                                                   │
  │  Document model — rope, marks, ops, input rules?                  │
  │      └─ yes ──▶ crates/document-core           wasm32; the editor core        │
  │                                                                   │
  │  A diagnostic analyzer?                                           │
  │      └─ yes ──▶ crates/diagnostics   wasm32; pure, no infra         │
  │                                                                   │
  │  Test scaffolding?                                                │
  │      └─ yes ──▶ crates/test-utils    Testcontainers, TestContext    │
  │                                                                   │
  │  Anything else?                                                   │
  │      └─ KEEP IT IN THE SERVICE. Duplicate if two services need    │
  │         it. Extract on the third, never on the second.            │
  └──────────────────────────────────────────────────────────────────┘
```

### Vocabulary, not entities

The decision tree above is easy to over-apply. The sharper test, because it settles almost every
case in one line:

> **`crates/domain` holds the *vocabulary*, never the *entities*.**

| Vocabulary — shared | Entity — stays in the slice that owns it |
|---|---|
| `PageId` · `BlockId` · `OpId` · `ActorId` | `Page` — only `document-service` constructs one |
| **`BlockKind`** — the editor produces it, the service stores it | **`Block`** — the row, with `page_id`, `path`, `content` |
| `SortKey` — `lower.rs` assigns them, the service orders by them | `Title` · `MaterialisedPath` (an LTREE detail) · `LifecycleState` |
| `Op` · domain events · `DomainError` | `User` · `Email` · `Password` — `auth-service`'s alone |

`crates/domain/src/block.rs` is the pattern in miniature: it holds **`BlockKind`**, not `Block`.

**Two tests, and a type must pass one:**

1. **Count the consumers.** Two or more crates → `crates/domain`. Exactly one → that crate.
2. **Does it cross a serialization boundary?** If `crates/editor-wasm` produces it and a service stores
   it, they must agree *exactly* — so it is shared even at two consumers. This is the one case
   where *extract on the third use* does not apply: **a duplicated type that crosses a wire is a
   bug, not a duplication.**

`Page` fails both — one constructing crate, and it reaches the browser as a proto message rather
than as a Rust type. So there is no `page.rs` in `crates/domain`, and there should not be.

---

## 7. The Expansion Path

The structure in §2 is deliberately small. Everything below is what you add **when something needs
it**, never before — `ROADMAP.md` § Phase 0: *foundation work is pulled in by the first service
that needs it.*

| Trigger | Add |
|---|---|
| **A second crate needs the ids, `BlockKind`, `Op`** | `crates/domain`. **The one case where two consumers is enough** rather than three — a type that crosses a serialization boundary must be identical, not merely compatible (§6) |
| **The editor core exists** | `crates/document-core` — the model: block tree, ops, anchors, rope, CRDT. wasm32-clean |
| **Typing has to become a block tree** | `crates/document-parser` — lex · parse · lower · normalise · sanitise. Split from `document-core` by **who links it**, not by size: `sanitise` needs an HTML parser and no backend service should link one |
| **A second consumer of the permission check** | `crates/rbac-core` — roles, permissions, policy, `can()`. **Pure, no I/O** — it is the shape `can_apply(op, actor)` needs |
| **A second service publishes events** | `crates/event-core` — one `EventEnvelope`, typed payload per publisher |
| **A second service needs the generated types** | `crates/proto`. Until then the `.proto` and `build.rs` live in the one service that uses them |
| **The browser needs to call Rust** | `crates/editor-wasm` |
| **Second service** | Nothing shared. **Copy** the six root files. Copying twice is cheaper than a wrong abstraction |
| **Third service** | *Now* extract `crates/infra` — telemetry, config, `AppError`. Three consumers is the rule (§6) |
| A service needs another's data | An **event** and local materialisation. Never a call on a hot path, never a cross-database join (`DATA_MODEL.md` §1) |
| A slice's `api.rs` serves two transports | Split to `api/rest.rs` + `api/grpc.rs`. Only the gateway has needed this |
| Logic spans two aggregates | `service.rs` in the slice that owns the operation — §5.3 |
| A proc macro would remove real duplication | `crates/macros`. Not before: `macro_rules!` covers most of it |
| Integration tests repeat container setup three times | `crates/test-utils` |
| A diagnostic analyzer exists | `crates/diagnostics` — wasm32-clean, pure, no infra |

**What never gets added**: a `usecases/` directory (§5.4), a trait that abstracts another trait
(§5.2), or a `crates/` crate with one consumer (§6).

### `crates/editor-wasm` is the `wasm-bindgen` boundary, made into a file

ADR-004 requires the boundary be **designed, not discovered**. Without a crate to hold it, it gets
discovered — one `#[wasm_bindgen]` at a time, scattered wherever one was needed.

| It owns | Why here and not in `crates/document-core` |
|---|---|
| Every `#[wasm_bindgen]` export | The TypeScript-facing API is one reviewable surface |
| `serde_wasm_bindgen` marshalling | The crossing cost lives at the crossing |
| **`syntect` + `two-face`** | Highlighting is *rendering*, not modelling, and the project's largest dependency |

**The parser is `crates/document-parser`, not a module in here.** Earlier revisions put lex, parse,
lower, normalise and sanitise inside this crate on the argument that the browser is their only
consumer. The dependency argument that motivated it — *no backend service should link an
XSS-boundary HTML parser or `syntect`* — is fully satisfied by a separate crate that only
`editor-wasm` depends on, and a plain library tests, fuzzes and reviews better than a module
inside a cdylib. `editor-wasm` is left as what it actually is: the bindgen boundary plus rendering.

`crate-type = ["cdylib", "rlib"]`, so **all of it tests natively** — `cargo test` needs no wasm
toolchain.

> **The boundary is a syscall, not a function call.** Cross it rarely with large payloads, never
> often with small ones. A per-keystroke round trip to fetch a token list is the shape that kills
> wasm performance.

### Workspace mechanics

```toml
members         = ["crates/*"]
default-members = ["crates/*"]                 # editor-wasm is rlib too, so this is fine
```

Without `default-members`, a root build compiles a `cdylib` for the host target — useless at best.
With it, `cargo check -p editor-wasm --target wasm32-unknown-unknown` still works, which is what
the CI gate needs.

### The wasm line

`crates/domain` and `crates/document-core` must stay `wasm32`-clean and infrastructure-free — they run in the
browser, and that purity is what keeps them Miri-reachable and fuzzable.

> **Adding a crate to `crates/` means stating which side of that line it is on.** If it is clean, add
> it to the gate in the same commit:
> `cargo check -p domain -p doc --target wasm32-unknown-unknown`
>
> A rule enforced only in this document is a rule that gets broken by an innocent import. Expect
> the gate to catch something on its first run — `uuid` alone does not compile for `wasm32` without
> an explicit randomness source, which needs a target-scoped dependency so native builds never
> link `js-sys`.

---

## 8. Checklist for a New Slice

```
  □ Failing tests first — agents.md § stage 1
  □ Migration written before the code (the schema is the contract)
  □ model.rs — one struct, stacked derives
  □ repo.rs  — trait + Postgres impl, SAME file, takes &mut Transaction
  □ api.rs   — thin: extract → delegate → map. No business rules
  □ Registered in lib.rs — including any consumer/poller, and in the drain
  □ Integration test against real Postgres (#[sqlx::test]) — never a mock
  □ Mutation paths pass can_apply
  □ NO service.rs unless §5.3 says so
  □ NO new trait unless a second impl or a test needs it
  □ NO new crate unless §6 says so
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
| `crates/shared` as a dumping ground | §6 decision tree |
| Layer-first directories | Feature-first slices |
| UI or handler mutating the tree directly | Compile it to an `Op` (RFC-002 §1) |
| Infrastructure imported into `crates/document-core` or `crates/diagnostics` | Those stay `wasm32`-clean |
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
| `adr/ADR-007` | gRPC as the east-west default |
| `adr/ADR-008` | Google Cloud + Terraform |
| `adr/ADR-010` | Cost-bounded cloud posture; Tier R vs Tier S |
| `rfc/RFC-001` | Block tree, spans↔rope, input rules, paste |
| `rfc/RFC-002` | Op ISA, invertibility, log versioning, WAL |
| `rfc/RFC-003` | Diagnostics analyzers, symbol table, incrementality |
| `CLOUD_PORTABILITY.md` | Why external dependencies sit behind traits |
| `docs/api/README.md` | Why `utoipa` annotations are mandatory |
