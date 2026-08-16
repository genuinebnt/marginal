# Marginal

A **self-hosted, real-time collaborative markdown notebook.** Block-based WYSIWYG editing, live multiplayer with no merge-conflict UI, inline diagnostics on prose, per-actor undo, and scrubable version history.

Eleven Rust microservices, event-sourced on a CRDT operation log.

## Rules

**Read `.agents/agents.md` before every response.** All mentor behaviour, strict review rules, TDD guidance, and documentation requirements live there. This file is fast context only.

### The scaffold checklist — every module, no exceptions

`agents.md` §2 in full. Reproduced here because this file is loaded every session and that one is not.

```
1  Types          structs, enum variants, error types — no bodies
2  Signatures     including the Result and its error type
3  Invariants     numbered; what the tests check
4  Algorithm      pseudocode, numbered steps, for anything non-obvious
5  Test list      names + which is hardest
6  Before         1–3 prerequisites — cite a CHAPTER, never a book
7  DSA            the named algorithm + 2–4 LeetCode problems, closest marked
8  After          how real projects solved it; what the spec chose for me
```

**They write all the Rust.** Tests become real Rust once it compiles.

**Draw 6 and 8 from:** *Rust for Rustaceans* · Gjengset's *Crust of Rust* (name the episode) ·
Skiena · **DDIA** · **Database Internals** · *Crafting Interpreters* · *Rust Atomics and Locks* ·
*Zero To Production* · system-design writing. **Applicability, not completeness** — a step that
touches no storage gets no DDIA row.

**The tedium rule:** Cargo, build errors, Docker, Terraform, CI, sqlx setup, mechanical refactors
→ complete copy-ready commands, never an exercise. Algorithms, invariants, schemas → theirs.

**Deadline: end of January 2027.** Dense scaffolds, no seminar.

---

## Objective & Order

**Primary objective: really good Rust learning** (ADR-002). Microservices, distributed systems, cloud, security, and DSA all remain goals — Rust depth wins ties.

**Phase numbers are identifiers, not a sequence.** Work `ROADMAP.md` § Execution Order:

```
Track 1 — MVP             1 Documents → 2 Auth → 3 Collaboration
                          🏁 log in, write a page, edit live with someone
Track 2 — Differentiators 4 Diagnostics → 5 Undo/Redo → 6 History
Track 3 — Distributed     7 Search → 8 Saga → 9 Gateway → 10 Session routing
Track 4 — Platform        13 Identity/RBAC → 14 Comments → 15 Notifications
                          → 16 Full editor → 20 Settings/admin      (ADR-009)
Track 5 — Reach           17 Publishing → 18 Plugins → 19 Assistant
                          → 21 Related content                       (ADR-009)
Track 6 — Cloud           11 Containers/CI + self-host ops → 12 Observability + hardening
```

**Tracks 4–5 are gated on the 🏁.** ADR-009 § Guard Rails is binding: nothing starts
before the MVP ships, every phase names new Rust or is cut, and the document core is
closed to incidental changes from these features.

**Cloud is interleaved, not deferred.** Each phase deploys its own service to Google Cloud
as part of that phase — see `CLOUD_ROADMAP.md` §2. Phase 1 includes Terraform, serverless
Postgres, Cloud Storage, Secret Manager, and a Cloud Run deploy.

**Two tiers, and the budget decides which (ADR-010).** *Tier R* stays running and must idle at
zero — Cloud Run `min = 0`, Pub/Sub, GCS, one Postgres with a schema and role per service.
*Tier S* — GKE, load balancers, Cloud SQL, Memorystore — is rented by the hour and
`terraform destroy`ed. **≤ $10/month learning, < $2 idle.**

**Phase 0 is a backlog, not a step.** Foundation work is *pulled* in by the first service
that needs it — never built up front. `crates/` extraction follows PROJECT_STRUCTURE §5:
inline, duplicate on the second use, extract on the third. See ROADMAP § Phase 0 for the
floor (workspace, migration, Postgres, `Settings`) and each item's trigger.

**Current: Phase 1 — Documents.** `crates/document-core` exists — `Page`, `Block`, `Op` with
`invert`, and `History` with undo/redo, 18 tests green. **Next: replace `Vec<Span>` with flat text
+ marks over byte ranges** (RFC-001 §2, TASKS.md D-02). No service, no startup path, no database
yet — every line of those is unwritten.

`docs/architecture/lld/` specifies *what* to build and `docs/learning/` what to read first. Treat
the LLD module maps as a proposal rather than a contract: the layout is derived, not inherited.

`crates/document-core` and `crates/diagnostics` need no infrastructure — pure `cargo test`, interleavable any time.

---

## Stack

| Layer | Technology |
|---|---|
| HTTP | Axum + Tower + Tokio |
| Database | PostgreSQL 18 + sqlx (JSONB, LTREE, `uuidv7()`) |
| Cache / presence | Redis |
| Event bus | NATS JetStream (local / self-host) · **Pub/Sub** (cloud) — one `EventBus` trait, two adapters (ADR-010) |
| Object storage | MinIO (local) / Cloud Storage (cloud) |
| Search | Tantivy (in-process) |
| gRPC | tonic + prost — **the east-west default**, all 4 RPC modes (ADR-007) |
| Frontend | React 19 + TypeScript SPA (Vite), Tailwind v4, Radix UI (ADR-004) |
| **Editor core** | **Rust → `wasm32`** — document model, rope, marks, selection, ops |
| API contract | `utoipa` → OpenAPI → `openapi-typescript` |
| IaC / hosting | Terraform (HCL) → **Google Cloud, the primary target** (ADR-008) |
| Observability | OpenTelemetry → Jaeger/Cloud Trace + Prometheus + Grafana |

---

## Services

| Service | Port | Boundary justification |
|---|---|---|
| `api-gateway` | 8000 | The edge — only public component; REST/WSS in, gRPC out (ADR-007) |
| `document-service` | 8001 | Stateless; owns pages, blocks, **its own** outbox. gRPC `PageService`; HTTP probes only |
| `collaboration-service` | 8002 | **Stateful** — rope per doc, scales on connection count. **Owns `collab`** — the op log and its outbox (ADR-003) |
| `diagnostics-service` | 8003 | CPU-bound, bursty, **degradable** |
| `history-service` | 8004 | Cold path — replay, snapshots to object storage |
| `search-service` | 8005 | Own Tantivy index, own rebuild cadence |
| `auth-service` | 8006 | Distinct security surface; also users, roles, preferences |
| `notification-service` | 8007 | Bursty fan-out, **degradable** — a lost notification costs nothing (ADR-009) |
| `publishing-service` | 8008 | **Unauthenticated** public read path, CDN-fronted (ADR-009) |
| `plugin-service` | 8009 | **Untrusted code** — isolation is the whole point (ADR-009) |
| `assistant-service` | 8010 | External API latency, **degradable**, never on the editing path (ADR-009) |

A service exists only if it differs in **scaling profile, state, failure mode, or deploy cadence**. Owning a different noun is not sufficient.

---

## Crate Layout

```
crates/                  ALL Rust lives here, flat, named by role
├── domain/              vocabulary: ids, BlockKind, Op, errors — zero deps, wasm32-clean
├── document-core/       the model: block tree, ops, anchors, rope, CRDT — wasm32-clean
├── document-parser/     what the user types: lex → parse → lower → normalise → sanitise
├── rbac-core/           roles · permissions · policy · can() — PURE, no I/O
├── event-core/          one EventEnvelope, typed payload per publisher
├── infra/               config · error · tracing · metrics — at the THIRD service
├── proto/               .proto + generated tonic/prost — NOT wasm32
├── editor-wasm/         cdylib+rlib — the bindgen boundary + syntect rendering
├── api-gateway/         THE ONLY REST/WS surface
└── <noun>-service/      one binary each — gRPC + HTTP probes only

web/                     React + TypeScript SPA — NOT in the workspace (ADR-004)
deploy/                  Terraform, docker-compose, prometheus.yml
reference/               Go reference implementations — NOT in the workspace (ADR-005)

Only `document-core` exists. Every other crate appears when its trigger fires —
PROJECT_STRUCTURE §7. A crate name never carries the project name; the directory
says which repo it is in.
```

`crates/document-core` and `crates/diagnostics` must stay `wasm32`-clean and infrastructure-free — they run in the browser, which is also what keeps them Miri-reachable and fuzzable.

---

## Key Docs

| Doc | Purpose |
|---|---|
| `.agents/agents.md` | Mentor rules — governs every response |
| `docs/architecture/PROJECT_STRUCTURE.md` | **Layout + principles — governs every file placement** |
| `docs/architecture/rfc/RFC-001-document-model.md` | Block tree, spans↔rope, input rules, paste |
| `docs/architecture/rfc/RFC-002-operation-model.md` | **Op ISA, invertibility, log versioning, WAL** |
| `docs/architecture/rfc/RFC-003-diagnostics-engine.md` | Analyzers, symbol table, incrementality |
| `docs/architecture/lld/document-service.md` | **LLD — module map, type contracts, invariants, build order** |
| `docs/api/pages.md` | Pages contract — gRPC `PageService` + the gateway's REST mapping |
| `docs/architecture/ARCHITECTURE.md` | Service map, event bus, request flows, saga |
| `docs/architecture/DATA_MODEL.md` | **Database per service** (ADR-003) — schemas, ownership, and where a join happens |
| `docs/architecture/CLOUD_PORTABILITY.md` | Ports & adapters, local vs Google Cloud |
| `docs/architecture/GLOSSARY.md` | Ubiquitous language |
| `docs/planning/ROADMAP.md` | Phases, **Rust/DSA concepts map**, tooling |
| `docs/planning/USER_STORIES.md` | **What each phase means from the outside** — stories with a testable *Done when*, in execution order |
| `docs/planning/TASKS.md` | **Track 1 subtasks with an owner on every line** — the ADR-005 split, applied |
| `docs/planning/CLOUD_ROADMAP.md` | Cloud track + cost discipline |
| `docs/planning/TIMELINE.md` | Estimates, the two handoffs, division of labour |
| **`docs/learning/`** | **Per-phase reading lists — prerequisites and post-build, mandatory vs optional.** Start at `learning/README.md`; `00-foundations.md` §1 is a ten-day start-here order |
| `docs/api/README.md` | Why `utoipa` annotations are mandatory |
| `docs/ui-mockups/` | Static visual specs + **editor and reader chrome spec** — `editor.html` is the block editor |
| `docs/architecture/adr/` | 001 scope · 002 Rust depth · 003 Postgres · **004 SPA** (a Rust frontend is revisitable after the 🏁) · 005 Go reference · **007 gRPC east-west** · **008 GCP + Terraform** · **009 scope expansion** · **010 cost-bounded cloud posture** |

---

## Skill Workflow

| When | Run |
|---|---|
| After implementing a feature | `/project:simplify` |
| Before merging any PR | `/project:review` |
| Any auth / paste / op-authorization boundary touched | `/project:security-review` |

---

## Architecture Rules (summary)

- **Feature-first slices, not layer-first directories** — `pages/`, `blocks/`, `tree/`, each owning `model.rs` + `repo.rs` + `api.rs` (+ `service.rs` **only** when logic exists). Never `application/usecases/<entity>/`
- **gRPC internally, REST only at the gateway.** Every service binds two listeners: gRPC for traffic, HTTP for `/health` probes. The gateway is the sole translator
- **The UI never mutates the tree — every change is an `Op`** (RFC-002 §1)
- **Every op is invertible**, designed in from the start, not discovered in the undo phase
- **The op log is the source of truth**; block rows are a projection that replay must reproduce
- **Every op passes `can_apply(op, actor)`** — one auditable authorization chokepoint
- Every external dependency sits behind a **trait declared in the same file** as its impl
- `crates/domain` has **zero** external dependencies — required for `wasm32`
- One `config.yaml` per service; safe local defaults; secrets via env only
- Integration tests hit real services via Testcontainers / `#[sqlx::test]` — **never mock infrastructure**

**Speed rules (over-abstraction is a review failure too):** one struct with stacked derives instead of Row→Domain→Dto chains · no trait abstracting another trait · no `service.rs` for CRUD · start a slice at two files and split on friction · duplicate on the second use, extract on the third.

**`unsafe` and concurrent code need `loom` + Miri + `cargo-fuzz`, not just tests.**

---

## Out of Scope (ADR-001, narrowed by ADR-009)

**Still out — these need structural change, not just an ADR:**

Databases/tables/views/relations/rollups · formula language · spatial canvas · mobile apps.

The first is the hard one: `collab.ops.page_id` is `NOT NULL` and `collaboration-service`
owns exactly one page per instance, so cross-page aggregation has **no owner**. That is a
second ownership tier, not a feature.

**Now in scope (ADR-009):** RBAC/spaces · comments · reactions · notifications ·
publishing/feeds/newsletter · analytics · WASM plugins · semantic search and the assistant
· the full editor, fonts, reader modes, ⌘K.

If a still-out item appears in a request, it needs an ADR first.

---

## Documentation Rules (summary)

Before writing any code, check whether these need updating:

1. `docs/architecture/DATA_MODEL.md` — schema changed?
2. `docs/api/` — endpoint added or modified?
3. `docs/architecture/rfc/` — document model, op set, or analyzer set changed?
4. `docs/architecture/adr/` — major architectural decision?

5. `docs/learning/` — **phase added, split, renumbered, or cut?** A phase with no reading list is
   a phase whose decisions get made by whoever is nearest.

Full rules: `.agents/agents.md` § Continuous Documentation.
