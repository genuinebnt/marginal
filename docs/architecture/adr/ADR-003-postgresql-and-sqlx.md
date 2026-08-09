# ADR-003 — PostgreSQL 18 + sqlx

**Date:** 2026-08-06
**Status:** Accepted
**Deciders:** @genuinebasilnt

---

## Context

The store must hold: a block tree per page, an append-only operation log that is the system's source of truth, semi-structured per-block-type content, and time-ordered ids that sort correctly without coordination.

A document-oriented store was considered first. It was rejected once the op log became the source of truth — an event-sourced system needs strong transactional guarantees on append, and the outbox pattern requires writing state and event **in one transaction**.

---

## Decision

**PostgreSQL 18** as the only persistent store, accessed via **sqlx**.

One **database per service** — see § Amendment. No cross-database joins (ADR-001).

### Why PostgreSQL 18 specifically

| Feature | Used for |
|---|---|
| **Native `uuidv7()`** | Time-ordered primary keys — sequential inserts avoid B-tree index fragmentation, and ids sort chronologically without a coordinator |
| **JSONB** | Per-block-type `content` — each `BlockType` variant has its own shape; a relational column-per-type would be sparse and unmaintainable |
| **LTREE** | Materialised block ancestry paths — subtree queries without recursive CTEs on the hot path |
| **Recursive CTEs** | Tree reconstruction where LTREE is insufficient |
| **`FOR UPDATE SKIP LOCKED`** | Outbox and snapshot-worker polling without lock contention between workers |
| **Transactional DDL** | Migrations that roll back cleanly |
| **`LISTEN`/`NOTIFY`** | Local change notification, complementing NATS |

### Why sqlx

- **Compile-time checked queries** (`query!`, `query_as!`) — SQL verified against the live schema at build time. A renamed column is a compile error, not a runtime 500
- No ORM. SQL stays visible, which matters when the point is learning query behaviour and reading `EXPLAIN ANALYZE`
- `#[sqlx::test]` provisions an isolated database per test, enabling parallel integration tests against real Postgres
- Native async, no blocking pool wrapper

### Trade-offs accepted

- **`DATABASE_URL` must be reachable at build time**, or `cargo sqlx prepare` must have been run and `.sqlx/` committed. CI needs this; it is a real friction cost
- **Macro-checked queries cannot be fully dynamic.** Dynamic filters use `QueryBuilder` with bound parameters — never string interpolation
- JSONB is validated by the application, not the database. `content` shape correctness is a Rust-side invariant, so it needs the version tag described in ADR-001

---

## Consequences

### Migrations

Each service owns `migrations/` and runs `sqlx migrate` against its own schema. Independent cadence; no shared migration ordering.

### The op log is the source of truth

Block rows are a **projection** of the op log, not the authority. That means:

- The op log table is append-only — no `UPDATE`, no `DELETE`
- Block rows can, in principle, be rebuilt by replay. A test should prove it
- Snapshots exist for performance, not correctness

### Local and cloud parity

`postgres:18-alpine` locally; Amazon RDS PostgreSQL in the cloud. Same major version, same extensions, so `uuidv7()` and LTREE behave identically. Integration tests run against the local container — **no mocking of the database, ever.**

---

## Amendment — database per service, not schema per service

**Added 2026-08-09.** Supersedes the original *"one schema per service, in one instance"*.

Each service gets its **own PostgreSQL instance**, not a schema in a shared one. The reason is
**failure isolation**: one service's database going down must not take the others with it, and a
shared instance couples them through the connection pool, the CPU, and the buffer cache regardless
of how disciplined the schemas are.

| | Shared instance | Instance per service |
|---|---|---|
| A service's DB dies | **All three schemas unavailable** — logins, pages, and history together | Only that service degrades. Existing JWTs still verify; editing continues from the rope |
| Noisy neighbour | Pool exhaustion or a lock storm in one schema starves the others | Contained |
| Isolation enforced by | Grants, if you remember to write them | **The network** |
| Backup / restore | One schedule, one blast radius | Per service, and a restore drill affects one thing |
| Cost | One instance | **N instances** — real money on Cloud SQL, and the honest reason the original decision said otherwise |

**Locally, run one Postgres container per service too.** It costs RAM and nothing else, and a local
topology that does not match the deployed one hides exactly the failure this amendment exists to
expose (`CLOUD_PORTABILITY.md` §1).

### What this does not fix, stated so it is not assumed

**Postgres was never the largest single point of failure.** `ARCHITECTURE.md` §1 ranks them:

1. **Redis** — the page-ownership lease. If it dies, **live editing stops globally**, and splitting Postgres does not touch this
2. Postgres for `docs` — page reads and writes stop
3. The gateway — single entry point, mitigated by being stateless and replicated
4. NATS — degraded only; the outbox holds events durably

**Doing this and stopping here leaves the worst SPOF in place.** Redis HA, or a lease store that
is not Redis, is the follow-on decision.

### The consequence that changes an existing design

In the pre-amendment design `collaboration-service` wrote `docs.blocks`, `docs.ops`, and
`docs.outbox` — tables `document-service` owned. With one instance that was a convention violation.
**With separate instances it would mean handing one service credentials to another service's
database server**, which is no longer a shortcut but an architecture.

**Decided: the op log moves to `collaboration-service`.**

**`document-service` owns the op log in Phase 1 and hands it over at Phase 3.** Phase 1 ships a
working single-user editor, which means block edits must persist — so `document-service` writes
block-granular ops (RFC-002 §2.1) and applies them to `blocks` itself. When
`collaboration-service` arrives it takes ownership of the log, and `document-service` switches to
materialising `blocks` from op events.

**The handover is a Phase 3 task with a real cost — one data migration** — and it is worth paying:
the alternative is three phases before anything you type is saved.

| Owner | Tables | Role |
|---|---|---|
| **`document-service`** | `pages`, `blocks`, `page_links`, its own `outbox` | Page metadata is CRUD over gRPC. **`blocks` is a projection**, materialised by replaying op events |
| **`collaboration-service`** | **`ops`** (append-only, the source of truth), its own `outbox` | Owns the live rope, so it owns what the rope produces |

**Why this is the right split rather than merely a tidier one:**

- **It buys the isolation this amendment is for.** With ops in `document-service`, a `docs` database
  outage means `collaboration-service` cannot flush, the WAL grows without bound, and sessions
  eventually have to stop. With ops local, **editing continues fully durable** and only the
  projection goes stale
- **The replay invariant becomes production code, not just a test.** `DATA_MODEL.md` §1 requires that
  replaying ops reproduces blocks; under this split `document-service` *is* that replay
- **`blocks` being eventually consistent was already the design.** RFC-001 §2: while a session is
  live the rope is authoritative and the JSONB is a checkpoint. And read-your-writes was already
  specified as *read from the doc-actor, not the projection* (`ROADMAP.md` § Distributed systems).
  This makes an existing property explicit instead of introducing a new one
- **Two outboxes, one per publishing service** — which is simply what database-per-service requires,
  and it removes the coupling `ARCHITECTURE.md` §1 flags, where `document-service` being down
  silenced the event bus for everyone

**Fallback if replay-in-`document-service` proves heavy:** `collaboration-service` publishes a
flushed block snapshot per page instead of raw ops, and `document-service` applies it. Same
ownership, less machinery, and it forfeits the production-exercised replay invariant. Prefer the
first; take this only with a profile in hand.

### Cost discipline

`CLOUD_ROADMAP.md`'s budget assumed one instance. Three of the smallest Cloud SQL tiers is real
recurring spend for a project that is not running continuously. **Provision per phase and destroy
between sessions** — the Terraform already makes that a one-command operation, and it is the same
discipline the cloud track asks for everywhere else.

---

## Alternatives Considered

| Alternative | Why not |
|---|---|
| SurrealDB / document store | Weaker transactional guarantees than an event-sourced outbox needs; smaller Rust ecosystem; less transferable knowledge |
| SQLite | Excellent for the client-side local queue, insufficient for the server (no concurrent writers at scale, no LISTEN/NOTIFY) |
| Diesel | Synchronous core, and its DSL hides the SQL this project exists to learn |
| SeaORM | An ORM — same objection, plus runtime query construction loses compile-time checking |
| An event store (EventStoreDB) | A purpose-built event store would teach event sourcing with less work, which is the reason not to use one here |
