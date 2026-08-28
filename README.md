# Marginal

A **self-hosted, real-time collaborative markdown notebook.**

Block-based WYSIWYG editing. Multiple people on one page, live, with no merge-conflict dialog ever. Inline diagnostics on prose — dangling `[[page links]]`, heading-level skips — with one-click fixes. Per-actor undo that survives interleaved collaborative edits. Version history you can scrub.

Built in Rust as a learning project. Seven microservices, event-sourced on a CRDT operation log.

> **Status: documentation only.** No code exists yet — Phase 0 has not started.
> This repository currently contains the architecture, data model, and roadmap.

---

## Why this exists

A learning project with a real finish line. The primary objective is **Rust depth** — the CRDT, the rope, lock-free op batching, a write-ahead log, an incremental analysis engine — with microservice architecture, distributed systems patterns, and cloud/IaC as genuine secondary goals rather than decoration.

Scope is deliberately narrow (ADR-001). No databases, no formulas, no workspaces, no permissions matrix. A notebook that does three things unusually well:

1. **Real-time collaboration** with no conflict UI, on a CRDT written from scratch
2. **Diagnostics on prose** — IDE-grade incremental static analysis applied to notes
3. **Event sourcing that is real** — the op log *is* the source of truth; block rows are a projection

---

## Architecture at a glance

```
   browser ── React SPA + Rust WASM editor core
      │  HTTPS / WSS
      ▼
   api-gateway :8000 ── RS256 verify · rate limit · circuit breaker · WS hash routing
      │
      ├── document-service      :8001   pages, blocks, tree, outbox
      ├── collaboration-service :8002   WebSocket, CRDT, rope, WAL      [stateful]
      ├── diagnostics-service   :8003   analyzers, incremental engine   [degradable]
      ├── history-service       :8004   op replay, snapshots            [cold path]
      ├── search-service        :8005   Tantivy, backlinks
      └── auth-service          :8006   Argon2id, RS256, rotation
             │
      PostgreSQL 18 · Redis · NATS JetStream · MinIO/S3
```

A service exists only if it differs in **scaling profile, state, failure mode, or deploy cadence**. Owning a different noun is not sufficient — see ADR-001.

**Stack:** Axum · Tokio · sqlx · tonic · Tantivy · NATS JetStream · React 19 + TypeScript · Pulumi · OpenTelemetry

---

## Running it

Not yet runnable. When Phase 0 lands:

```bash
docker compose up -d          # Postgres, Redis, NATS, MinIO, Jaeger, Prometheus, Grafana
cargo run -p api-gateway      # and each service
cd web && npm run dev         # the SPA
```

Self-hosting is the same compose file plus the built services. Deploying to AWS is [`AWS_ROADMAP.md`](docs/planning/AWS_ROADMAP.md).

---

## Documentation

| Doc | What it covers |
|---|---|
| [`ROADMAP.md`](docs/planning/ROADMAP.md) | 13 phases in four tracks, DSA map, correctness tooling |
| [`PROJECT_STRUCTURE.md`](docs/architecture/PROJECT_STRUCTURE.md) | Feature-first slices, and the pragmatic limits on abstraction |
| [`ARCHITECTURE.md`](docs/architecture/ARCHITECTURE.md) | Service map, event bus, request flows, the delete saga |
| [`DATA_MODEL.md`](docs/architecture/DATA_MODEL.md) | Postgres schema; why the op log is truth and rows are a projection |
| [`RFC-001`](docs/architecture/rfc/RFC-001-document-model.md) | Block tree as AST, the spans↔rope boundary, input rules, paste |
| [`RFC-002`](docs/architecture/rfc/RFC-002-operation-model.md) | The op instruction set, invertibility, log versioning, the WAL |
| [`RFC-003`](docs/architecture/rfc/RFC-003-diagnostics-engine.md) | Analyzers, symbol table, incremental invalidation |
| [`CLOUD_PORTABILITY.md`](docs/architecture/CLOUD_PORTABILITY.md) | Ports and adapters; local Docker vs AWS |
| [`AWS_ROADMAP.md`](docs/planning/AWS_ROADMAP.md) | EKS, Pulumi, and honest cost discipline |
| [`GLOSSARY.md`](docs/architecture/GLOSSARY.md) | Ubiquitous language — and the terms deliberately absent |
| [`ui-mockups/`](docs/ui-mockups/) | Static visual spec — open [`v2/index.html`](docs/ui-mockups/v2/index.html) (40 screens) in a browser; [`DESIGN_GUIDELINES.md`](docs/ui-mockups/v2/DESIGN_GUIDELINES.md) is how to implement them 1:1 |

**Decisions:** [ADR-001 scope](docs/architecture/adr/ADR-001-scope-and-service-boundaries.md) · [002 Rust depth](docs/architecture/adr/ADR-002-rust-depth-as-primary-objective.md) · [003 Postgres](docs/architecture/adr/ADR-003-postgresql-and-sqlx.md) · [004 SPA + Rust editor core](docs/architecture/adr/ADR-004-react-spa-with-rust-editor-core.md) · [005 Go reference as answer key](docs/architecture/adr/ADR-005-go-reference-as-answer-key.md) · [006 gRPC](docs/architecture/adr/ADR-006-grpc-selective-transport.md)

---

## A note on how this is built

`.agents/agents.md` sets the working rules: **the Rust is written by the author, not generated.** An AI assistant supplies failing test suites, architecture review, and — only *after* an attempt exists — Go reference implementations as an answer key for algorithm-heavy work (ADR-005).

The frontend is the exception: UI is not a learning goal, so `web/` is AI-authored. The **editor core is not** — the document model, rope, marks, selection mapping, and operation handling are Rust compiled to `wasm32`, and they are the author's (ADR-004).
