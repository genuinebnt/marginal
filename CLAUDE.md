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

**Cloud is built ahead of live use, not deferred anymore.** `CLOUD_ROADMAP.md`/
`ADR-008`/`ADR-010`'s GCP+Terraform plan (Cloud Run, one Postgres per
service, the two-tier cost posture) is implemented at `deploy/terraform/`
— written and self-reviewed without live GCP credentials (2026-08-26, at
explicit request), not yet `apply`'d against a real project. See that
directory's own `README.md` for exactly what's provisioned, the cost
posture, and known limitations (a few real gaps between what the Go code
does today and what a full cloud deployment would need — documented there,
not silently papered over).

**Current state:** see `docs/porting/PROGRESS.md` — the single source of
truth for what's implemented and what's next.

---

## Stack

| Layer | Technology |
|---|---|
| HTTP | Go stdlib `net/http` (health probes) + `chi` (REST, gateway only) |
| Database | PostgreSQL 18 + `pgx/v5` + `sqlc` (JSONB, LTREE, `uuidv7()`) |
| Cache / presence | Redis |
| Event bus | NATS (local / self-host) · **Pub/Sub** (cloud, deferred) — one topic in use today, `auth.user_registered` (`auth-service` → `notification-service`, `DATA_MODEL.md` §10). Plain core NATS, not JetStream — no durability/redelivery of its own; a deliberate gap at this repo's scope, see `internal/notify`'s doc comment |
| Object storage | MinIO (local) / Cloud Storage (cloud, deferred) |
| Search | deferred — out of Track 1 scope |
| gRPC | `google.golang.org/grpc` + `buf` — **the east-west default**, REST only at the gateway |
| Frontend | React 19 + TypeScript SPA (Vite), Tailwind v4, Radix UI |
| **Editor core** | **Go, compiled to `GOOS=js GOARCH=wasm`** — `services/documentcore` (its own module); TS is views + a JSON bridge only, never a second implementation (`ADR-011` addendum; keeps ADR-004's wasm boundary, source language differs for now) |
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
| `notification-service` | 8007 | Pulled back into scope 2026-08-26 — real logic now: consumes `auth.user_registered` (`DATA_MODEL.md` §10, the one event topic Track 1 can actually produce) over NATS, persists a welcome notification, serves `GET /notifications` directly (not proxied — same convention as `collaboration-service`'s WebSocket). Every other notification-worthy topic still needs a feature that's out of scope (mentions/comments, sharing, RBAC) |
| `api-gateway` | 8000 | Pulled back into scope 2026-08-26, at the "minimum to reach portable code" — **not** the full 11-service design's `api-gateway` (RS256 verify, rate limit, circuit breaker, WS consistent-hash routing, all still absent). A thin REST↔gRPC shim only: translates `pages.md`/`auth.md` §2's REST contract onto `document-service`/`auth-service`'s gRPC, so a browser can call them at all. `collaboration-service`'s WebSocket is reached directly, not proxied — a persistent connection isn't a request/response resource |

The other 6 services from the full design (`diagnostics-service`,
`history-service`, `search-service`, `publishing-service`, `plugin-service`,
`assistant-service`) are out of scope for this repo — see `ADR-011`. A
service exists only if it differs in **scaling profile, state, failure
mode, or deploy cadence**; owning a different noun is not sufficient
(`ADR-001`, unchanged).

---

## Layout

```
go.work                         at repo root — one Go module per service, no wrapper directory
docker-compose.yml              local dev stack — all 5 services + one Postgres each + Redis + web/'s Vite dev server; `docker compose up --build`

services/                       backend, kept separate from web/; each has its own Dockerfile
├── documentcore/                its own module (marginal/documentcore), no cmd — page.go, block.go, operation.go, history.go, inline.go; imported by document-service (wasm bridge) AND collaboration-service (block-op session state) — moved out of document-service/internal in the block-collab reconciliation pass so neither has to reimplement the other's Page.Apply
├── document-service/
│   ├── go.mod, cmd/main.go
│   ├── cmd/wasm/                GOOS=js GOARCH=wasm entrypoint — the editor core's browser build; imports marginal/documentcore
│   ├── genproto/documentv1/     generated from proto/document.proto — NOT under internal/, so api-gateway (a separate module) can import the client stub across module boundaries
│   └── internal/blockproj/      materialises docs.blocks by consuming collab.ops_flushed (NATS) — document-service's read model, never a second writer
├── auth-service/                genproto/authv1/ at the same non-internal path, same reason; internal/outbox publishes auth.user_registered
├── collaboration-service/       session.Session now holds a documentcore.Page per page (block ops) alongside the flat doctext.Text (character ops within a block's own content) — see docs/architecture/DATA_MODEL.md § collab.ops → docs.blocks
├── notification-service/        internal/notify — NATS consumer + Postgres + GET /notifications; see the Services table above
└── api-gateway/                 thin REST↔gRPC shim — see the Services table above; internal/{pagesrest,authrest,apierror,actorctx}

web/                            frontend — React 19 + TS SPA (Vite); real screens, not a scaffold
├── public/documentcore.wasm     built by services/document-service/scripts/build-wasm.sh, gitignored — NOT yet wired to any screen (see note below)
├── src/document-core/           types.ts (wire types) + wasm.ts (the JSON bridge) + history.ts (thin undo/redo bookkeeping)
├── src/api/                     REST clients — auth.ts, pages.ts, notifications.ts, one shared http.ts (pages.md/auth.md §2's error shape)
├── src/auth/AuthContext.tsx     token storage (localStorage) + the current actor id every other client derives from the JWT `sub` claim
├── src/collab/                  useCollabPage.ts — the block-aware WebSocket client for docs/api/collaboration.md (internal/pageop's wire shape), plus the browser's query-param actor-auth workaround; blockKind.ts (BlockKind ⇄ <select> key mapping)
├── src/screens/                 AuthPage, DashboardScreen (page grid + create), EditorScreen (rail + RichEditorPane + InspectorRail)
└── src/design-system.css        copied from docs/ui-mockups/mockup.css — "if a mockup and a doc disagree, the doc wins," so this stays a copy, not a reinterpretation

testdata/document-core/*.json   golden test vectors for services/documentcore — Go today, Rust later

docs/rust/                      docs only, no code — the archived Rust-mentor track (see docs/rust/README.md)

docs/porting/                   PROGRESS.md, PORTING_GUIDE.md, OPEN_QUESTIONS.md, BENCHMARKS.md
```

`reference/` and `deploy/` don't exist yet — pulled in when a service
actually needs a Go answer-key reference or infra, same "Phase 0 is a
backlog" principle as before.

**The Track 1 editor is feature-complete, end to end.** `collaboration-service`'s
`Session` holds a `documentcore.Page` (block structure: insert/delete/
reorder/kind, via `internal/pageop`'s `Block` ops) **and** one
`doctext.Text` live rope per block (character-level edits within that
block, via `pageop`'s `Text` ops) — RFC-002 §2's two ISA tiers, one WAL,
one flush pipeline, one broadcast. `document-service`'s `internal/blockproj`
consumes `collab.ops_flushed` (via a real outbox poller — both sides now
built) and materialises `docs.blocks`/`docs.page_links`, resolving
`[[Page Title]]` backlinks.

`web/`'s `RichEditorPane` (paragraph/heading 1-3/quote/code_block/divider,
each its own block-tree node) speaks that same `pageop`-tagged protocol
via `useCollabPage`, plus: a floating "/" slash menu and a persistent "+"
insert-element bar (same popup, one converts the current block, the other
inserts a new one after it), drag-to-reorder blocks, real live presence
(join/leave, not an op-broadcast heuristic), and inline marks
(bold/italic/strike/code/link) via a selection bubble menu —
`SetBlockContent` is the only op that can carry `Content.Marks` at all,
so **a block with any mark trades real-time character-level merging for
whole-block last-write-wins on every future edit to it**, a deliberate,
stated tradeoff (`web/src/collab/marks.ts`'s own doc comment), not a bug.
`PageTreeRail` (the left rail) is a real lazily-loaded nested page tree
with its own drag-and-drop reparent/reorder — separate from the editor's
own block-level drag-and-drop. `InspectorRail`: Outline, People, and
Backlinks are real; Checks/Comments/History stay honest empty-state,
naming the out-of-scope service each would need.

Verified against the real running stack throughout — multiple standalone
Go WS smoke tests (structural ops, presence join/leave, marks surviving
into a fresh connection's snapshot) plus real HTTP calls through
`api-gateway`, not just unit tests. `docs/porting/PROGRESS.md` has the
detail, chronologically, including every real bug this stretch of work
surfaced and how each was fixed.

**Still open, stated plainly:** page-link marks as a real inline mark
kind (distinct from `blockproj`'s plain-text-regex backlink scan), and
mark offsets are JS string indices (UTF-16), not the byte offsets
`documentcore` persists — identical for ASCII text, an accepted
simplification for multi-byte text. `Manager` still keeps every session
open indefinitely (no idle-eviction), matching this repo's demo scale.

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
| `docs/api/pages.md` | Pages contract — gRPC `PageService` §1 + the gateway's REST mapping §2 |
| `docs/api/auth.md` | Auth contract — gRPC `AuthService` §1 + the gateway's REST mapping §2 |
| `docs/api/collaboration.md` | Collaboration contract — the WebSocket wire format (one contract, no REST projection) |
| `docs/architecture/ARCHITECTURE.md` | Service map, event bus, request flows |
| `docs/architecture/DATA_MODEL.md` | **Database per service** — schemas, ownership, and where a join happens |
| `docs/architecture/CLOUD_PORTABILITY.md` | Ports & adapters, local vs Google Cloud |
| `docs/architecture/GLOSSARY.md` | Ubiquitous language |
| `docs/planning/ROADMAP.md` | Full phase list — only Track 1 is in scope here |
| `docs/planning/USER_STORIES.md` | **What each phase means from the outside** — testable *Done when* |
| `docs/planning/CLOUD_ROADMAP.md` | Cloud track + cost discipline — `deploy/terraform/` now implements this, see its own README |
| `docs/planning/TIMELINE.md` | Original estimates — largely superseded by `ADR-011`'s scope change |
| `docs/porting/PROGRESS.md` | **Current state — read first every session** |
| `docs/porting/PORTING_GUIDE.md` | How the future Rust port should approach this codebase |
| `docs/porting/OPEN_QUESTIONS.md` | Still-open, language-independent product decisions |
| `docs/api/README.md` | API documentation conventions |
| `deploy/terraform/README.md` | GCP IaC — what's provisioned, cost posture, known limitations, setup steps |
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
RBAC/spaces, comments, reactions, notifications (the feature — digesting,
per-user preferences, delivery — not the `notification-service` skeleton,
which is scaffolded; see the Services table above), publishing, plugins,
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
