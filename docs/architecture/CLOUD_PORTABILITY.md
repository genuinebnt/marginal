# Marginal — Cloud Portability (Ports & Adapters)

**Google Cloud is the primary deployment target** (ADR-008). Marginal must also run under **`docker compose up`** — for local development, for the integration tests, and because self-hosting is a product capability (ADR-001).

Both run identical application code, which is only true if every external dependency sits behind a trait and the concrete implementation is chosen by configuration at startup.

> The traits are **not** scaffolding for self-hosting that a primary cloud target makes redundant. They are what lets integration tests hit real local Postgres, Redis, NATS, and MinIO instead of mocks — and what stops one cloud becoming an untestable dependency. They would be worth keeping even if self-hosting were dropped entirely.

---

## 1. Local vs Cloud

> **Two cloud columns, not one (ADR-010).** *Tier S* is the architecture at full size, rented by
> the hour — `CLOUD_ROADMAP.md` §2 is its syllabus. *Tier R* is what stays running under the cost
> ceiling. Where they differ it is the same trait with a different implementation, which is what
> this document exists to make possible.

| Concern | Local (compose) | Google Cloud — Tier S (session) | Google Cloud — Tier R (resident) | Notes |
|---|---|---|---|---|
| Database | `pgvector/pgvector:pg18`, **one container per service** | Cloud SQL for PostgreSQL, **one instance per service** (ADR-003) | **one serverless Postgres, schema + role per service** | **Version skew is real** — managed Postgres lags upstream. LTREE and `pgvector` are available; `uuidv7()` may not be. See § The `uuidv7()` trap. Neither managed option idles at zero except Neon |
| Cache / presence | `redis:7-alpine` | Memorystore for Redis | in-process behind the same trait, or the free `e2-micro` | Memorystore is provisioned and always billing |
| Event bus | `nats:2` JetStream (`NatsBus`) | **Pub/Sub** (`PubSubBus`) | **Pub/Sub** (`PubSubBus`) | Pub/Sub in the cloud, NATS locally and self-hosted. Why both, and what it costs: § Pub/Sub and NATS — why both |
| Object storage | `minio/minio` | Cloud Storage | Cloud Storage | One `object_store` adapter covers both; the difference is a URL and a credential source |
| Search index | Tantivy on a local volume | Tantivy on a Persistent Disk | Tantivy on the Cloud Run instance, rebuilt from events on cold start | In-process; no managed equivalent, and none needed at this scope |
| Static client | `vite dev` / nginx | Cloud Storage + Cloud CDN | Firebase Hosting | Static bundle, no server (ADR-004). Cloud CDN needs a load balancer, which is never free |
| Ingress | exposed ports | Cloud Load Balancing via the GKE Gateway API | the Cloud Run service URL | Managed TLS either way; a forwarding rule bills continuously |
| Secrets | git-ignored `.env` | Secret Manager, injected as env | Secret Manager, injected as env | Never in `config.yaml`, ever |
| Traces | Jaeger container | Cloud Trace via OTel Collector | Cloud Trace via OTel Collector | Same OTLP exporter |
| Metrics | Prometheus container | Managed Service for Prometheus | Cloud Monitoring | Same `/metrics` endpoint |
| Container images | local build | Artifact Registry | Artifact Registry | 500 MB free — prune tags |
| Pod identity | none needed | Workload Identity Federation | the Cloud Run service account | No static keys either way |

---

## 2. Trait → Implementation Mapping

Every trait is declared in the **same file** as its primary implementation (`PROJECT_STRUCTURE.md` §4).

| Trait | Local impl | Cloud impl | Test impl |
|---|---|---|---|
| `PageRepo`, `BlockRepo` (`document-service`) | `PostgresPageRepo` | same — Cloud SQL is Postgres | real Postgres via `#[sqlx::test]` |
| `OpLog` (**`collaboration-service`**, its own instance) | `PostgresOpLog` | same, separate instance | real Postgres via `#[sqlx::test]` |
| `ObjectStore` | `BlobStore` → MinIO endpoint | `BlobStore` → GCS + Workload Identity | MinIO in Testcontainers |
| `EventBus` | `NatsBus` | **`PubSubBus`** | real NATS in Testcontainers **and** the Pub/Sub emulator — both, per ADR-010 §2 |
| `CacheStore` | `RedisCache` | `RedisCache` → Memorystore (Tier S) or in-process (Tier R) | real Redis in Testcontainers |
| `SearchIndex` | `TantivyIndex` | `TantivyIndex` | temp-dir index |
| `AnalyticsSink` | `ParquetSink` → local files | `BigQuerySink` | in-memory sink |
| `Clock` | `SystemClock` | `SystemClock` | `FixedClock` — the one legitimate fake |

**Notice how few adapters differ between local and cloud.** That is the point: the Postgres wire protocol and the [`object_store`](https://docs.rs/object_store) abstraction are the portability layer. `ObjectStore` is the only trait where the cloud path meaningfully differs, and only in authentication.

### The one real code difference: Workload Identity

Locally, MinIO takes a static access key. On GKE, `BlobStore` authenticates via **Workload Identity Federation** — a Kubernetes service account bound to a Google service account, credentials sourced from a projected token, no static keys anywhere.

The client library's default credential chain handles this if you let it. The failure mode is hardcoding a credential provider that works locally and silently ignores the projected token in cluster.

Do **not** reach GCS through its S3-compatible XML API. It works, but it requires static HMAC keys, which defeats Workload Identity entirely — that is the anti-pattern, not the shortcut.

### Pub/Sub and NATS — why both

`EventBus` is the one trait with two genuinely different implementations, and it is worth being
precise about why, because "we support both" is usually a smell.

**On capability, Pub/Sub is adequate.** `seek()` to a timestamp or snapshot, retention
configurable to 31 days, ordering keys, dead-letter topics, exactly-once delivery per
subscription. Nothing in this project's use of the bus exceeds it.

Unbounded replay is not the differentiator either: **the op log is the source of truth and it
lives in Postgres** (`DATA_MODEL.md` §1). The bus carries derived events for indexing, cache
invalidation and saga steps. A full rebuild replays `docs.ops`, not the bus.

**Neither can do the other's job.**

1. **Pub/Sub cannot self-host.** There is no local production equivalent — the emulator is for
   tests — and self-hosting is an ADR-001 product capability. `NatsBus` is the self-host and
   local-development path, and it is what the integration tests run against.
2. **NATS cannot idle at zero.** JetStream is stateful: it needs a disk, therefore a machine.
   Under the Cloud Run posture (ADR-010) it would be the only always-on component in an estate
   that otherwise costs nothing at rest — the single largest line on the bill.

So there are two adapters, and the cost is paid rather than argued away: **every delivery
guarantee, ordering model and redelivery behaviour exists twice, and every consumer must be
correct under both.** Ordering is where it bites — JetStream gives stream sequence numbers,
Pub/Sub gives ordering keys that interact differently with retries and dead-lettering.
`DATA_MODEL.md` §10 tabulates exactly where they diverge, and the integration suite runs each
consumer against both.

**`NatsBus` is primary in the sense that matters:** it is the self-host path, and it defines the
delivery semantics `PubSubBus` must satisfy.

> **Running a broker is also the better teacher.** Streams, consumers, ack policies and
> replay-from-sequence are mechanics you learn by operating them; configuring a managed queue
> teaches configuration. Local NATS keeps that, which is part of why it stays.

### Extensions are dependencies too

`ltree` and `pgvector` must exist on both sides. Locally that means the
`pgvector/pgvector` image rather than stock `postgres`; on Cloud SQL both are supported
extensions that have to be **enabled per instance**, which is a Terraform line, not a
runtime `CREATE EXTENSION` the application can perform.

Check availability before the phase that needs it. Discovering in Phase 19 that the
managed instance cannot host the vector index is discovering it a year late.

### The `uuidv7()` trap

`migrations/0001_init.sql` declares `id UUID PRIMARY KEY DEFAULT uuidv7()`. That function is a **PostgreSQL 18** built-in, and managed Postgres lags upstream — on Cloud SQL, and on RDS equally.

The design already survives this: ids are generated as UUIDv7 **by the service** and passed into `PageId::new(uuid)`, so they come from the application and the column default is a convenience for hand-written SQL. Keep it that way. `document-core` itself never generates one — it cannot, because `wasm32-unknown-unknown` has no source of randomness.

> **Never write an `INSERT` that omits the id and lets the database generate it.** It works locally and fails on the first managed instance that ships Postgres 17.

---

## 3. Configuration

### One `config.yaml` per service

No `local.yaml`, no `production.yaml`. One file with **safe local defaults**, overridden by environment variables.

```yaml
# crates/document-service/config.yaml
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

Ports are assigned dynamically so tests run in parallel. `crates/test-utils` provides the `TestContext` that wires a full stack per test module.

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
