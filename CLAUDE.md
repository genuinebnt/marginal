# Marginal

A **self-hosted, real-time collaborative markdown notebook.** Block-based WYSIWYG editing, live multiplayer with no merge-conflict UI, inline diagnostics on prose, per-actor undo, and scrubable version history.

Eleven microservices, event-sourced on a CRDT operation log.

**Currently building the Track 1 MVP in Go + TypeScript** (`ADR-011`) — Claude
writes the implementation directly. The eventual Rust port is a separate,
later repo; see `docs/rust/README.md` for what's archived there and why.

## Rules

**Read `.agents/agents.md` before every response.** Testing philosophy,
documentation conventions, the Go/TS idiom table, and the feature-depth
complexity budget all live there. This file is fast context only.

**Read `docs/porting/PROGRESS.md` at the start of any new or compacted
session.** It is the record of what's actually done — don't re-derive or
assume prior decisions from a stale summary.

**Deadline: none imposed by this track** — the goal is a complete,
demo-quality MVP, not a fixed date. Scope discipline comes from
`.agents/agents.md` §3 (feature depth, not surface area), not a calendar.

---

## Objective & Order

**Primary objective: a complete, demo-quality Track 1 MVP in Go + TypeScript**
(`ADR-011`). Rust depth (`ADR-002`) is suspended for this track, not
abandoned — it resumes in the future Rust-port repo.

**Phase numbers are identifiers, not a sequence.** Work `ROADMAP.md` § Execution Order — only Track 1 is in scope for this repo right now:

```
Track 1 — MVP             1 Documents → 2 Auth → 3 Collaboration
                          🏁 log in, write a page, edit live with someone
```

All three Track 1 services are scaffolded as standalone code areas now
(`.agents/agents.md` §3) — they share no code, so there's no benefit to
phasing them in one at a time. Real logic still lands one at a time,
starting with `document-service`.

**Tracks 2–6 are out of scope for this repo.** They were previously gated on
the 🏁 and built here afterward (`ADR-009`); now the plan is to build them,
if at all, after the Rust port, in that future repo, per `ADR-011`.

**Cloud is deferred, not designed away.** `CLOUD_ROADMAP.md`/`ADR-008`/`ADR-010`'s
GCP+Terraform plan still applies conceptually (Cloud Run, one Postgres per
service, the two-tier cost posture) — it's pulled in once a service is ready
to deploy, same "Phase 0 is a backlog" principle as before, just not yet
exercised for the Go build.

**Current state:** see `docs/porting/PROGRESS.md` — the single source of
truth for what's implemented and what's next.

---

## Stack

| Layer | Technology |
|---|---|
| HTTP | Go stdlib `net/http` (health probes) + `chi` (REST, gateway only) |
| Database | PostgreSQL 18 + `pgx/v5` + `sqlc` (JSONB, LTREE, `uuidv7()`) |
| Cache / presence | Redis |
| Event bus | NATS JetStream (local / self-host) · **Pub/Sub** (cloud, deferred) — one `EventBus` interface, two adapters |
| Object storage | MinIO (local) / Cloud Storage (cloud, deferred) |
| Search | deferred — out of Track 1 scope |
| gRPC | `google.golang.org/grpc` + `buf` — **the east-west default**, REST only at the gateway |
| Frontend | React 19 + TypeScript SPA (Vite), Tailwind v4, Radix UI |
| **Editor core** | **Native TypeScript** — document model, marks, selection, ops (no `wasm32` for this track — `ADR-011` overrides `ADR-004`) |
| API contract | Hand-maintained OpenAPI in `docs/api/` → `openapi-typescript` |
| IaC / hosting | Terraform (HCL) → Google Cloud — deferred until a service is ready to deploy |
| Observability | OpenTelemetry → Jaeger/Cloud Trace + Prometheus + Grafana — deferred |

---

## Services (Track 1 only, this repo)

| Service | Port | Boundary justification |
|---|---|---|
| `document-service` | 8001 | Stateless; owns pages, blocks, **its own** outbox. gRPC `PageService`; HTTP probes only |
| `auth-service` | 8006 | Distinct security surface; also users, roles, preferences |
| `collaboration-service` | 8002 | **Stateful** — rope per doc, scales on connection count. **Owns `collab`** — the op log and its outbox |

The other 8 services from the full design (`api-gateway`, `diagnostics-service`,
`history-service`, `search-service`, `notification-service`,
`publishing-service`, `plugin-service`, `assistant-service`) are out of scope
for this repo — see `ADR-011`. A service exists only if it differs in
**scaling profile, state, failure mode, or deploy cadence**; owning a
different noun is not sufficient (`ADR-001`, unchanged).

---

## Layout

```
go.work                         at repo root — one Go module per service, no wrapper directory

services/                       backend, kept separate from web/
├── document-service/           go.mod, cmd/main.go, internal/documentcore/, internal/...
├── auth-service/
└── collaboration-service/

web/                            frontend — React 19 + TS SPA + native TS document-core (Vite)
├── src/document-core/          page.ts, block.ts, operation.ts, history.ts, inline.ts
└── ...

testdata/<module>/*.json        golden test vectors — shared behavior spec across languages

docs/rust/                      docs only, no code — the archived Rust-mentor track (see docs/rust/README.md)

docs/porting/                   PROGRESS.md, PORTING_GUIDE.md, OPEN_QUESTIONS.md, BENCHMARKS.md
```

`reference/` and `deploy/` don't exist yet — pulled in when a service
actually needs a Go answer-key reference or infra, same "Phase 0 is a
backlog" principle as before.

---

## Key Docs

| Doc | Purpose |
|---|---|
| `.agents/agents.md` | Build rules for this track — governs every response |
| `docs/architecture/PROJECT_STRUCTURE.md` | Layout + principles — governs every file placement |
| `docs/architecture/rfc/RFC-001-document-model.md` | Block tree, spans, input rules, paste — language-agnostic |
| `docs/architecture/rfc/RFC-002-operation-model.md` | **Op ISA, invertibility, log versioning, WAL** — language-agnostic |
| `docs/architecture/rfc/RFC-003-diagnostics-engine.md` | Analyzers, symbol table, incrementality (Track 2, not in scope yet) |
| `docs/architecture/lld/document-service.md` | Module map, type contracts, invariants, build order |
| `docs/api/pages.md` | Pages contract — gRPC `PageService` + the gateway's REST mapping |
| `docs/architecture/ARCHITECTURE.md` | Service map, event bus, request flows |
| `docs/architecture/DATA_MODEL.md` | **Database per service** — schemas, ownership, and where a join happens |
| `docs/architecture/CLOUD_PORTABILITY.md` | Ports & adapters, local vs Google Cloud |
| `docs/architecture/GLOSSARY.md` | Ubiquitous language |
| `docs/planning/ROADMAP.md` | Full phase list — only Track 1 is in scope here |
| `docs/planning/USER_STORIES.md` | **What each phase means from the outside** — testable *Done when* |
| `docs/planning/CLOUD_ROADMAP.md` | Cloud track + cost discipline (deferred) |
| `docs/planning/TIMELINE.md` | Original estimates — largely superseded by `ADR-011`'s scope change |
| `docs/porting/PROGRESS.md` | **Current state — read first every session** |
| `docs/porting/PORTING_GUIDE.md` | How the future Rust port should approach this codebase |
| `docs/porting/OPEN_QUESTIONS.md` | Still-open, language-independent product decisions |
| `docs/api/README.md` | API documentation conventions |
| `docs/ui-mockups/` | Static visual specs + editor/reader chrome spec |
| `docs/rust/README.md` | What's archived from the Rust attempt, and why |
| `docs/architecture/adr/` | 001 scope · ~~002 Rust depth~~ · 003 Postgres · ~~004 SPA~~ · ~~005 Go reference~~ · 007 gRPC east-west · 008 GCP + Terraform · 009 scope expansion · 010 cost-bounded cloud posture · **011 Go+TS MVP, Rust port later** |

---

## Skill Workflow

| When | Run |
|---|---|
| After implementing a feature | `/code-review` or `/project:simplify` (idiom + simplicity) |
| Before merging any PR | `/code-review` |
| Any auth / paste / op-authorization boundary touched | `/security-review` |

---

## Architecture Rules (summary)

- **Feature-first slices, not layer-first directories** — `pages/`, `blocks/`, `tree/`, each owning `model.go`/`.ts` + `repo.go` + `api.go` (+ `service.go` **only** when logic exists). Never `application/usecases/<entity>/`
- **gRPC internally, REST only at the gateway** (deferred until a gateway exists in this repo's scope — direct HTTP is fine for the standalone services meanwhile)
- **The UI never mutates the tree — every change is an `Op`** (RFC-002 §1)
- **Every op is invertible**, designed in from the start, not discovered in the undo phase
- **The op log is the source of truth**; block rows are a projection that replay must reproduce
- **Every op passes `can_apply(op, actor)`** — one auditable authorization chokepoint
- Every external dependency sits behind a **small interface declared at its point of use** (Go) / a typed port (TS) — see `CLOUD_PORTABILITY.md`
- One `config.yaml` per service; safe local defaults; secrets via env only
- Integration tests hit real services via `testcontainers-go` — **never mock infrastructure**

**Speed rules (over-abstraction is a review failure too):** one struct with stacked tags instead of Row→Domain→DTO chains · no interface abstracting another interface · no `service.go` for CRUD · start a slice at two files and split on friction · duplicate on the second use, extract on the third.

**Concurrent/untrusted-input code needs `-race`, `goleak`, and native fuzzing, not just tests** (`.agents/agents.md`).

---

## Out of Scope

**Still out — these need structural change, not just an ADR:**

Databases/tables/views/relations/rollups · formula language · spatial canvas · mobile apps.

The first is the hard one: `collab.ops.page_id` is `NOT NULL` and `collaboration-service`
owns exactly one page per instance, so cross-page aggregation has **no owner**. That is a
second ownership tier, not a feature.

**Out for this repo specifically (`ADR-011`):** everything beyond Track 1 —
RBAC/spaces, comments, reactions, notifications, publishing, plugins,
semantic search/assistant, the full editor. These were "now in scope" under
ADR-009 for the eventual full build; they come after the Rust port, in that
future repo, not here.

If a still-out item appears in a request, it needs an ADR first.

---

## Documentation Rules (summary)

Before writing any code, check whether these need updating:

1. `docs/architecture/DATA_MODEL.md` — schema changed?
2. `docs/api/` — endpoint added or modified?
3. `docs/architecture/rfc/` — document model, op set, or analyzer set changed?
4. `docs/architecture/adr/` — major architectural decision?
5. `docs/porting/PROGRESS.md` — anything land, or any decision made? Log it.
