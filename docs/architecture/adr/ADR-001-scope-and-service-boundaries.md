# ADR-001 — Scope: Collaborative Markdown Notebook, and Its Service Boundaries

**Date:** 2026-08-06
**Status:** Accepted
**Deciders:** @genuinebasilnt

---

## Context

Marginal began as "build Notion" — 13 services, 24 phases, databases with typed properties, formulas, workspaces, RBAC, templates, publishing, a spatial canvas, and a WASM sandbox. Phase 1 stalled, and the scope was a large part of why: the finish line was invisible, and roughly half the phases repeated Rust already learned.

The product is now scoped deliberately tight:

> **A self-hosted, real-time collaborative markdown notebook.**
> Block-based WYSIWYG editing, live multiplayer with no merge-conflict UI, inline diagnostics on prose, per-user undo across collaborative edits, and scrubable version history.

The learning objectives are unchanged and remain the point of the project: **Rust depth first**, then microservice architecture, distributed systems, cloud/IaC, security, DSA, and data modelling.

The tension this ADR resolves: a tight product does not obviously need microservices. Scoping down without thought would collapse to a single binary and delete the architecture goals along with the feature bloat.

---

## Decision

### What the product does

- Pages with a block-based WYSIWYG editor — paragraphs, headings, lists, code blocks, toggles, quotes, images
- Markdown input rules (`# `, `- `, ` ``` `, `**bold**`) converting live as you type
- Multiple people on one page URL: live cursors, character-level convergence, **no merge-conflict UI ever**
- **Inline diagnostics on prose** — dangling `[[page links]]`, heading-level skips, orphan pages, empty code blocks — with click-to-fix
- Per-user undo/redo that survives interleaved collaborative edits
- Version history with a scrubber and restore
- **Deployed to Google Cloud via Terraform** as the primary target (ADR-008); also self-hostable via `docker compose up`

### What it deliberately does not do

| Cut | Why |
|---|---|
| Databases, tables, views, relations, rollups | The largest source of scope; a separate project |
| Formula language / expression VM | Separate project — it needs databases to be worth anything |
| Workspaces, RBAC, multi-tenancy | Single-tenant instance; auth stays minimal but real |
| Comments, notifications, templates, publishing | Feature breadth without new learning |
| Spatial canvas, WASM code execution sandbox | Interesting, unrelated, unbounded |
| Semantic search / vector index | Full-text is enough for `[[link]]` resolution |
| Mobile apps | Web only |
| Offline-first beyond what the CRDT gives free | Local queue + replay, no bespoke conflict resolution |

### Eleven services, each with a defensible boundary

A service exists only if it differs in **scaling profile, state, failure mode, or deploy cadence**. Owning a different noun is not sufficient — that is how the previous scope reached thirteen.

Seven are listed below; **ADR-009 adds four** — `notification-service` (8007), `publishing-service` (8008), `plugin-service` (8009), `assistant-service` (8010) — each justified against the same rule. `CLAUDE.md` § Services carries the full table.

| Service | Port | Boundary justification |
|---|---|---|
| `api-gateway` | 8000 | The edge. Only component exposed publicly; JWT verification, rate limiting, WebSocket routing |
| `document-service` | 8001 | Stateless request/response, scales with read traffic. **Owns the outbox** — first publisher |
| `collaboration-service` | 8002 | **A doc-actor: one document, one owner, at any time.** In-memory rope per open document, sticky sessions, scales on *connection count* rather than RPS. The ownership invariant is what makes sharding and lease-based handoff necessary rather than optional |
| `diagnostics-service` | 8003 | **CPU-bound, bursty, memo-cache-heavy**, and **degradable** — if it dies the editor still works and only loses squiggles. Real failure isolation, not a nominal one |
| `history-service` | 8004 | **Cold path.** Replay-heavy, write-often/read-rarely, snapshots to object storage. Different storage medium entirely |
| `search-service` | 8005 | Own Tantivy index with its own rebuild cadence, independent of the write path |
| `auth-service` | 8006 | Distinct security surface and deploy cadence; the only service holding password hashes |

### The ownership invariant

`collaboration-service` is best understood as an **actor per document**. Exactly one instance owns a given page's live state at a time, which is why:

- The gateway must know *which* instance owns a page — consistent hashing, not round-robin
- Ownership must be held under a **lease**, not merely recorded. A recorded owner that pauses (GC, network partition, `SIGSTOP`) and resumes can accept ops for a page another node now owns. That is split-brain on the write path
- Handoff on node loss or scale-down is a real protocol, not a config setting

Presence is deliberately **not** a separate service. It is high-churn — cursor movement vastly outnumbers keystrokes — but that argues for a different *transport* (Redis pub/sub), not a different deployment unit: it shares the doc-actor's scaling profile, and if the owner dies, presence for that page is meaningless anyway.

Persistence is deliberately **not** a separate service either. The doc-actor must fsync its WAL *before* acknowledging an op (RFC-002 §6); routing that through an RPC adds network latency to every keystroke. `history-service` owns only replay and snapshots — genuinely cold path, genuinely different storage medium.

`storage-service` is **not** a separate service at this scope. Image upload is a presigned-URL handler inside `document-service` — the boundary test fails, since it shares the document lifecycle and has no independent scaling profile.

### Why microservices survive the scope cut

Each of the seven maps to a pattern the project exists to teach, and — more importantly — the **op log makes event sourcing structural rather than decorative**:

| Pattern | Where it lives |
|---|---|
| **Event sourcing** | The CRDT op log *is* an event store. Truth is the op sequence, not the block rows |
| **CQRS** | `history-service` is a read model projected from the op log |
| **Outbox** | `document-service`: Postgres write + NATS publish in one transaction |
| **Saga (choreographed)** | Page deletion: purge blocks → drop search segments → invalidate diagnostics → seal history, with compensations |
| **Idempotent consumers** | At-least-once NATS delivery, deduped on op id |
| **Consistent hashing + session affinity** | WebSocket routed by `page_id` to the owning collaboration instance |
| **Instance registry + failure detector** | Redis heartbeat with TTL; φ-accrual detection; rehash on failure |
| **Circuit breaker + graceful degradation** | Diagnostics unavailable → editor unaffected. Demonstrable, not hypothetical |
| **Distributed lock + fencing token** | Snapshot worker mutual exclusion in `history-service` |
| **Anti-entropy / read repair** | Search index reconciled against Postgres periodically |
| **API gateway** | Tower middleware stack at the edge |

The previous scope had CRUD with events bolted on. This one is genuinely event-sourced, which is a stronger story with fewer services.

---

## Consequences

### Self-hosting is `docker compose up`, not a single binary

The original product sketch called for "a single binary + Postgres." That is a monolith and incompatible with this decision. Self-hosting means seven containers plus Postgres, Redis, NATS, and MinIO.

Heavier to self-host; the architecture goals are the reason. Accepted.

### Real network boundaries, with real costs

- Local development requires `docker compose up` before any code runs
- Debugging a single feature may span three service logs — OpenTelemetry distributed tracing is required from Phase 0, not optional
- Each service runs its own migrations against its own Postgres schema (`docs`, `auth`, `history`, `search`)
- No cross-schema joins. Data needed elsewhere travels as NATS events and is materialised locally

### Extensibility seams, committed now

Four one-line commitments that keep expansion cheap without building anything speculative:

1. **`can_apply(op, actor) -> bool`** on the op path, returning `true` today. Flipping the stub is how workspaces and RBAC arrive later — and it is a single auditable authorization chokepoint rather than checks threaded through every mutation path.
2. **A version tag on every persisted op**, with explicit enum discriminants. A persisted op log is a permanent wire format: history replay must decode ops written by every prior version, forever.
3. **A `ReferenceResolver` trait** with one implementation (page names → page ids). Formula property lookups would be a second implementation if that project happens.
4. **Block data and view configuration never merge into one object** — a written rule, with no view mechanism built.

Explicitly **rejected**: a generic `properties: Map<String, Value>` on every block. It duplicates the per-variant `content` JSONB, and it punches a hole in exactly the tagged-enum type safety that makes the block model work. New block types arrive as new variants.

### If databases ever arrive, their CRDT is new work

Concurrent edits to a 2D grid — column insert racing row insert, cell edit racing a column type change — is a materially different convergence problem from sequence CRDT. The block tree and op layer would be reused; the merge logic would not. Not something to design away in advance, and not cheap.

---

## Alternatives Considered

| Alternative | Why not |
|---|---|
| Keep the full Notion scope | No visible finish line; half the phases repeated Rust already learned |
| Single binary (as originally sketched) | Deletes microservices, gRPC, saga, gateway, session routing, and most of the cloud rationale |
| Modular monolith with "extract later" | Module boundaries in one process have no network failure, no serialisation boundary, no independent deployment. Extraction later means rewriting transport, auth propagation, and consistency handling simultaneously |
| Thirteen services at the reduced scope | Service-per-noun. Most would fail the boundary test above |
| Text-file source of truth instead of blocks | Would make parsing load-bearing, but block manipulation and structural diagnostics get much harder, and the CRDT story becomes ordinary |

---

## Resources

| Resource | For |
|---|---|
| [Microservices Patterns — Richardson](https://microservices.io/patterns/) | Saga, CQRS, event sourcing, outbox, API gateway |
| [DDIA Ch. 8, 11 — Kleppmann](https://dataintensive.net/) | Distributed systems trouble; logs as message storage |
| [Event Sourcing — Fowler](https://martinfowler.com/eaaDev/EventSourcing.html) | Why the op log is the source of truth |
| [Local-First Software — Ink & Switch](https://www.inkandswitch.com/local-first/) | The sync model this product implements |
