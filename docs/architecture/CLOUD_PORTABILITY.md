# Marginal — Cloud Portability (Ports & Adapters)

**Google Cloud is the primary deployment target** (ADR-008 § Amendment). Marginal must also run under **`docker compose up`** — for local development, for the integration tests, and because self-hosting is a product capability (ADR-001).

Both run identical application code, which is only true if every external dependency sits behind a trait and the concrete implementation is chosen by configuration at startup.

> The traits are **not** scaffolding for self-hosting that a primary cloud target makes redundant. They are what lets integration tests hit real local Postgres, Redis, NATS, and MinIO instead of mocks — and what stops one cloud becoming an untestable dependency. They would be worth keeping even if self-hosting were dropped entirely.

---

## 1. Local vs Cloud

| Concern | Local (compose) | Google Cloud | Notes |
|---|---|---|---|
| Database | `pgvector/pgvector:pg18`, **one container per service** | Cloud SQL for PostgreSQL, **one instance per service** (ADR-003 § Amendment) | **Version skew is real** — managed Postgres lags upstream. LTREE and `pgvector` are available; `uuidv7()` may not be. See § The `uuidv7()` trap |
| Cache / presence | `redis:7-alpine` | Memorystore for Redis | |
| Event bus | `nats:2` JetStream | **NATS on GKE**, not Pub/Sub | Not a capability gap — see § Pub/Sub, evaluated |
| Object storage | `minio/minio` | Cloud Storage | One `object_store` adapter covers both; the difference is a URL and a credential source |
| Search index | Tantivy on a local volume | Tantivy on a Persistent Disk | In-process; no managed equivalent, and none needed at this scope |
| Static client | `vite dev` / nginx | Cloud Storage + Cloud CDN | Static bundle, no server (ADR-004) |
| Ingress | exposed ports | Cloud Load Balancing via the GKE Gateway API | Managed TLS termination |
| Secrets | git-ignored `.env` | Secret Manager, injected as env | Never in `config.yaml` |
| Traces | Jaeger container | Cloud Trace via OTel Collector | Same OTLP exporter |
| Metrics | Prometheus container | Managed Service for Prometheus | Same `/metrics` endpoint |
| Container images | local build | Artifact Registry | |
| Pod identity | none needed | Workload Identity Federation | No static keys in cluster |

---

## 2. Trait → Implementation Mapping

Every trait is declared in the **same file** as its primary implementation (`PROJECT_STRUCTURE.md` §4).

| Trait | Local impl | Cloud impl | Test impl |
|---|---|---|---|
| `PageRepo`, `BlockRepo` (`document-service`) | `PostgresPageRepo` | same — Cloud SQL is Postgres | real Postgres via `#[sqlx::test]` |
| `OpLog` (**`collaboration-service`**, its own instance) | `PostgresOpLog` | same, separate instance | real Postgres via `#[sqlx::test]` |
| `ObjectStore` | `BlobStore` → MinIO endpoint | `BlobStore` → GCS + Workload Identity | MinIO in Testcontainers |
| `EventBus` | `NatsBus` | `NatsBus` → in-cluster NATS | real NATS in Testcontainers |
| `CacheStore` | `RedisCache` | `RedisCache` → Memorystore | real Redis in Testcontainers |
| `SearchIndex` | `TantivyIndex` | `TantivyIndex` | temp-dir index |
| `AnalyticsSink` | `ParquetSink` → local files | `BigQuerySink` | in-memory sink |
| `Clock` | `SystemClock` | `SystemClock` | `FixedClock` — the one legitimate fake |

**Notice how few adapters differ between local and cloud.** That is the point: the Postgres wire protocol and the [`object_store`](https://docs.rs/object_store) abstraction are the portability layer. `ObjectStore` is the only trait where the cloud path meaningfully differs, and only in authentication.

### The one real code difference: Workload Identity

Locally, MinIO takes a static access key. On GKE, `BlobStore` authenticates via **Workload Identity Federation** — a Kubernetes service account bound to a Google service account, credentials sourced from a projected token, no static keys anywhere.

The client library's default credential chain handles this if you let it. The failure mode is hardcoding a credential provider that works locally and silently ignores the projected token in cluster.

Do **not** reach GCS through its S3-compatible XML API. It works, but it requires static HMAC keys, which defeats Workload Identity entirely — that is the anti-pattern, not the shortcut.

### Pub/Sub, evaluated

An earlier version of this document claimed JetStream's replay had "no managed equivalent."
**That was wrong**, and the correction matters because it changes which argument is doing
the work.

Pub/Sub has `seek()` to a timestamp or snapshot, retention configurable to 31 days, ordering
keys, dead-letter topics, and exactly-once delivery per subscription. On capability it is
adequate. It is also cheaper — this project's event volume sits inside the free tier
permanently, against a NATS pod that must be run.

The justification weakens further on inspection: **the op log is the source of truth and it
lives in Postgres** (`DATA_MODEL.md` §1). The bus carries derived events for indexing, cache
invalidation, and saga steps. A full rebuild replays `docs.ops`, not the bus, so unbounded
bus replay was never load-bearing.

**Two arguments survive, and neither is about features:**

1. **Self-hosting.** Pub/Sub has no local production equivalent — the emulator is for tests.
   Keeping it would mean an `EventBus` trait with two implementations, which is legal under
   §2 but far costlier here than for a write-only sink: the bus is the core messaging
   substrate, so every delivery guarantee, ordering model, and redelivery behaviour exists
   twice, and every consumer must be correct under both. Ordering is where that bites —
   JetStream gives stream sequence numbers, Pub/Sub gives ordering keys that interact
   differently with retries and dead-lettering.
2. **Learning.** Running the broker teaches streams, consumers, ack policies, and
   replay-from-sequence as mechanics. Configuring a managed one teaches configuration.

**Decision: NATS stays primary.** Recorded here rather than dropped, because "we evaluated
Pub/Sub and chose otherwise for these reasons" is a stronger position than never having
looked.

### Extensions are dependencies too

`ltree` and `pgvector` must exist on both sides. Locally that means the
`pgvector/pgvector` image rather than stock `postgres`; on Cloud SQL both are supported
extensions that have to be **enabled per instance**, which is a Terraform line, not a
runtime `CREATE EXTENSION` the application can perform.

Check availability before the phase that needs it. Discovering in Phase 19 that the
managed instance cannot host the vector index is discovering it a year late.

### The `uuidv7()` trap

`migrations/0001_init.sql` declares `id UUID PRIMARY KEY DEFAULT uuidv7()`. That function is a **PostgreSQL 18** built-in, and managed Postgres lags upstream — on Cloud SQL, and on RDS equally.

The design already survives this: `PageId::new()` generates UUIDv7 in Rust, so ids come from the application and the column default is a convenience for hand-written SQL. Keep it that way.

> **Never write an `INSERT` that omits the id and lets the database generate it.** It works locally and fails on the first managed instance that ships Postgres 17.

---

## 3. Configuration

### One `config.yaml` per service

No `local.yaml`, no `production.yaml`. One file with **safe local defaults**, overridden by environment variables.

```yaml
# services/document-service/config.yaml
application:
  port: 8001
  host: 0.0.0.0

database:
  host: localhost
  port: 5432
  name: marginal
  schema: docs
  max_connections: 10
  # NO password here — env only

nats:
  url: nats://localhost:4222
  stream: marginal

object_store:
  endpoint: http://localhost:9000    # MinIO; unset in cloud to use GCS
  bucket: marginal-files

telemetry:
  otlp_endpoint: http://localhost:4317
  log_level: info
```

### Override convention

```
APP__DATABASE__HOST=10.x.x.x          (Cloud SQL private IP)
APP__DATABASE__PASSWORD=…             (from Secret Manager)
APP__OBJECT_STORE__ENDPOINT=          (empty ⇒ real GCS)
APP__NATS__URL=nats://nats.default.svc.cluster.local:4222
```

Double underscore is the nesting separator. The Rust `Settings` struct is the **definitive schema** — a missing required variable means the service **fails to start immediately**, never starts with a silent default.

### Secrets

Never in `config.yaml`, never committed. Local: git-ignored `.env`. Cloud: Secret Manager, mounted or injected as environment variables by the platform — never baked into an image, never in a `ConfigMap`.

---

## 4. Integration Test Strategy

> **Tests hit real local services. Infrastructure is never mocked.**

```
   #[sqlx::test]            → an isolated Postgres database per test, dropped after
   Testcontainers           → real Redis, NATS, MinIO on ephemeral ports
   FixedClock               → the only legitimate fake, because time is not infrastructure
```

Mocking a database means testing your understanding of Postgres rather than Postgres. LTREE queries, `FOR UPDATE SKIP LOCKED` semantics, JSONB operators, and transaction isolation behaviour are exactly the things a mock gets wrong.

Ports are assigned dynamically so tests run in parallel. `libs/test-utils` provides the `TestContext` that wires a full stack per test module.

---

## 5. Required Code Properties for the Cloud

Deploying to GKE is not just a packaging exercise — three things must be true in the Rust:

**Health endpoints.** Every service exposes `/health/live` and `/health/ready`. Readiness must actually check dependencies: a service that reports ready before its Postgres pool is warm gets traffic it cannot serve.

**Graceful shutdown.** Kubernetes sends `SIGTERM` on rolling update and scale-down. Tokio must intercept it and drain in order:

```
   SIGTERM → stop accepting new work → for collaboration-service:
             flush the ArrayQueue, sync the WAL, close WebSockets with a
             reconnect hint → close the Postgres pool → exit
```

`collaboration-service` is the hard case — it holds long-lived stateful connections, and a naive shutdown drops unflushed ops. This is where a hand-written `Future` earns its place (ADR-002).

**Stateless except where declared.** Only `collaboration-service` holds in-memory state, and it is recoverable: the WAL plus the op log rebuild any rope. That is what makes it safe to reschedule the pod.

---

## 6. What Is Deliberately Not Portable

| Thing | Why |
|---|---|
| Tantivy index | On-disk, in-process. Moving to a distributed search engine would be a rewrite, not a config change — and is unnecessary at this scope |
| `collaboration-service` sessions | Sticky by design. Consistent-hash routing means a given page lives on one instance; portability here means *rehashing on failure*, not statelessness |
| Local WAL | A filesystem path. In cluster it is an ephemeral volume, and correctness relies on it being flushed before shutdown, not on it surviving the pod |

Naming these keeps the ports-and-adapters rule honest: it applies to *external services*, not to everything.
