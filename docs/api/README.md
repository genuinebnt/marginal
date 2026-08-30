# Marginal API Contracts

The OpenAPI document is **generated from Rust**, never hand-written. `utoipa` derives it from the Axum handlers and their DTOs, so Rust is the single source of truth for the HTTP contract.

## Why this is mandatory, not a convenience

A full-Rust client would share types with the server and could not disagree with it. The TypeScript SPA (ADR-004) has **no such guarantee** — a renamed field becomes a runtime `undefined` rather than a build error.

The generated client is what replaces that lost guarantee. That makes generation part of the contract, and **CI fails on a dirty regeneration diff.** A stale client is the one failure mode this stack has that a full-Rust stack would not.

---

## Pipeline

```
  Axum handler + DTO            Rust — source of truth
        │  #[utoipa::path(...)] on the handler
        │  #[derive(ToSchema)]  on every request/response type
        ▼
  openapi.json                  committed artifact, reviewable diff
        │  openapi-typescript
        ▼
  web/src/api/schema.d.ts       generated, never edited by hand
        │
        ▼
  typed fetch client            a wrong field name is a tsc error
```

Committing `openapi.json` is deliberate: it makes a contract change show up as a **reviewable diff in the pull request**, which is the safety net that lets `PROJECT_STRUCTURE.md` §5.1 collapse the DB and API types into one struct.

---

## Rules

1. **Every handler carries `#[utoipa::path]`** with its response codes and error shapes. A handler without it is invisible to the client.
2. **Every request and response type derives `ToSchema`.**
3. **`openapi.json` is committed.** Regenerating it is part of the change, not a follow-up.
4. **CI regenerates and fails on a dirty diff.** Non-negotiable.
5. **`web/src/api/schema.d.ts` is generated.** Never hand-edit it; the next generation overwrites you.
6. **Errors use one shape** — `ApiError` — so the client has exactly one error branch to handle.

---

## What is not in OpenAPI

Two transports fall outside it, and both need hand-written contracts documented here:

| Transport | Contract | Where documented |
|---|---|---|
| **WebSocket** (`/collab/pages/:id`) | Op frames, snapshot, acks, broadcast (no presence yet) | `collaboration.md` — the full wire contract; `rfc/RFC-002` for the op ISA itself |
| **gRPC** (internal, east-west) | `PageService`, `AuthService` | `services/*/proto/*.proto` — protobuf is already a schema |

The op frame encoding is versioned (`RFC-002` §4) because it is persisted as well as transmitted. OpenAPI covers only the REST surface the browser calls.

---

## Endpoint Surface

Written here per feature as it is built — request/response shapes, status codes, error cases. Since the schema is generated, this file documents **intent and semantics**: idempotency, pagination behaviour, which errors are retryable.

| Area | Status |
|---|---|
| Pages | ✅ implemented — see `pages.md` |
| Blocks | ✅ implemented as part of `document-core` (page/block ops); no separate REST surface — see `pages.md` |
| Files (presigned upload) | ⬜ not in Track 1 scope (`ADR-011`) |
| Auth | ✅ implemented — see `auth.md` |
| Collaboration (WebSocket) | ✅ implemented — see `collaboration.md` |
| Diagnostics | ✅ implemented (`v2.3.0`) — see `diagnostics.md` |
| History, trace & diff | ✅ implemented (`v2.4.0`) — `collaboration.md` §§5–7, served by `collaboration-service` directly |
| Search & backlinks | ✅ implemented (`v2.5.0`) — see `search.md`; backlinks in `pages.md` |
| Graph | ✅ implemented (`v2.2.0`) — see `graph.md` |
| Series & trash | ✅ implemented — see `pages.md` |
| Admin | ✅ implemented — `GET /admin/health` (the gateway's own probe of every service) and `GET /admin/people` (`auth.md`); the queue numbers beside them are `collaboration.md` §8, reached directly |
