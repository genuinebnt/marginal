# Marginal — Cloud Deployment Roadmap

**Google Cloud is the primary deployment target** (ADR-008). This document is
how the eleven services get there, provisioned entirely as code.

This is not an optional track. **A phase is not done until its increment in §2 is deployed**
— which is why those increments are attached per phase rather than collected into a big-bang
deployment at the end.

`docker compose up` remains supported for self-hosting and is how you develop locally, but
it is no longer the definition of "running".

---

## 1. Local → Google Cloud Mapping

> The **Google Cloud** column is Tier R — what stays running (ADR-010). Tier S is the same
> architecture rented by the hour; §2 is its syllabus.

| Component | Local | Google Cloud | Why |
|---|---|---|---|
| Services | `cargo run` / compose | **Cloud Run**, every service, `min = 0` | GKE is rented per session, not the resident runtime (ADR-010 §1). Cloud Run speaks gRPC natively, so ADR-007 is unaffected |
| Database | `postgres:18-alpine` | **serverless Postgres**, schema + role per service | Cloud SQL cannot idle at zero (ADR-010 §3); it stays a Tier S rental for replicas and `pgvector`. **Check the available major version** — see § The version-skew check |
| Cache / presence | `redis:7-alpine` | in-process behind the trait, or the free `e2-micro` | Memorystore is provisioned and always billing — Tier S |
| Event bus | `nats:2` JetStream (`NatsBus`) | **Pub/Sub** (`PubSubBus`) | JetStream is stateful and cannot idle at zero; Pub/Sub has no self-hostable equivalent. Two adapters, one trait — ADR-010 §2 |
| Object storage | MinIO | **Cloud Storage** | One `object_store` adapter covers both |
| Static client | nginx / vite | **Firebase Hosting** | No server; a direct benefit of ADR-004's no-SSR decision. Cloud CDN needs a load balancer, which is never free |
| Ingress | exposed ports | the **Cloud Run service URL** | Managed TLS either way. A forwarding rule bills continuously — Tier S |
| Secrets | `.env` | **Secret Manager** | Injected as env at runtime |
| Traces | Jaeger | **Cloud Trace** via OTel Collector | Same OTLP exporter |
| Metrics | Prometheus | **Managed Service for Prometheus** | Same `/metrics` |
| Logs | stdout | **Cloud Logging** | Collected from stdout automatically on Cloud Run and GKE alike — no DaemonSet needed |
| Images | — | **Artifact Registry** | Pushed by CI |
| Pod identity | — | **Workload Identity Federation** | No static keys in cluster |

### The version-skew check

Do this **before** writing any Terraform: confirm which PostgreSQL major version Cloud SQL
offers. `migrations/0001_init.sql` uses `DEFAULT uuidv7()`, which is a PostgreSQL 18
built-in, and managed Postgres lags upstream by a year or more on every cloud.

The design already survives it — ids are generated in Rust by the service and passed into the model, never by the database — but the
`DEFAULT` clause must come out of the migration if the managed version predates 18.
`CLOUD_PORTABILITY.md` § The `uuidv7()` trap has the rule.

---

## 2. Cloud Increments — one per phase, not a track at the end

> **The cloud work is pulled in by the service that needs it.** Same rule as Phase 0 in
> `ROADMAP.md`: nothing is built speculatively, and nothing is deferred to a big-bang
> deployment phase at the end.

Deploying one service teaches more than reading about ten. Each phase below ends with its
service actually running in Google Cloud, and each introduces the **smallest set of new
GCP services** that phase genuinely demands.

| Phase | Ships | New GCP surface |
|---|---|---|
| **1 — Documents** | `document-service` deployed | Terraform + GCS state · project & IAM basics · **serverless Postgres** (Cloud SQL is Tier S — ADR-010 §3) · **Cloud Storage** (the presign handler is Phase 1) · **Secret Manager** · **Artifact Registry** · **Cloud Run** · budget alert · **Eventarc + Cloud Run job** — object-finalize triggers image variant generation |
| **2 — Auth** | `auth-service` deployed | **Memorystore** (token blocklist) · service account per service, least privilege · **Cloud KMS** for the RS256 signing key |
| **3 — Collaboration** | Sticky routing built and tested on a rented cluster | **GKE (Tier S)** · **Workload Identity** · Persistent Disk for the WAL · `PodDisruptionBudget` · custom-metric HPA. **Cloud Run stays the resident runtime** — ADR-010 §1; GKE is applied, learned from, destroyed |
| **4 — Diagnostics** | Degradation made visible | Resource requests/limits · CPU HPA · pod eviction behaviour |
| **6 — History** | Snapshots to object storage | GCS **storage classes & lifecycle rules** (Nearline for cold snapshots) · V4 signed URLs · **Cloud SQL read replica** (Tier S) — history is the cold path, so point replay at a replica · **Cloud Scheduler** for the snapshot job |
| **7 — Search** | Tantivy with real disk | `StatefulSet` · Persistent Disk classes · volume expansion |
| **8 — Saga** | The choreographed delete saga running on the managed bus | **Pub/Sub** — topics, push vs pull subscriptions, ordering keys, dead-letter topics, `seek()` replay · **Cloud Monitoring** alerts on outbox depth and op-log lag |
| **9 — Gateway** | Public ingress | **Cloud Load Balancing** via Gateway API · Google-managed certificates · **Cloud Armor** rate limiting and adaptive protection — the managed half of the load-shedding lesson · **Cloud CDN** + GCS for the SPA · **Cloud DNS** |
| **10 — Session routing** | Real instance discovery | Headless services · `StatefulSet` DNS · custom metrics adapter |
| **13 — Identity & RBAC** | Real multi-user auth in cloud | Secret Manager rotation · IAM conditions |
| **14 — Comments** | — | (no new GCP surface) |
| **15 — Notifications** | Email actually sending | **Cloud Tasks** for retries · an SMTP relay or SendGrid via Secret Manager |
| **16 — Full editor** | — | (no new GCP surface) |
| **17 — Publishing** | Public pages live | **Cloud CDN** + GCS static hosting, with **cache invalidation on publish** · **BigQuery** as the cloud `AnalyticsSink` — Parquet locally, same trait |
| **18 — Plugins** | Untrusted code isolated | **gVisor (GKE Sandbox)** node pool · tighter `NetworkPolicy` |
| **19 — Assistant** | AI in production | **Vertex AI** or an external provider behind Secret Manager · **`pgvector` enabled on Cloud SQL** via Terraform — the embedding index lives with the data so retrieval can be permission-filtered in one query |
| **20 — Settings & admin** | — | Runtime config in **Firestore** or Cloud SQL — decide, do not default |
| **11 — Containers, CI & self-host ops** | The pipeline, plus backups that are restore-tested | Workload Identity Federation for GitHub Actions — **no service account keys in CI** · GCS backup bucket with object versioning **and a retention policy** (write-once, so ransomware cannot rewrite history) · **Artifact Analysis** container scanning · **Binary Authorization** — only signed images run · **Cloud Scheduler** for the nightly backup |
| **12 — Observability & hardening** | The finish, not the start | **Cloud Trace** · **Managed Prometheus** · Grafana · **Cloud Profiler** — continuous profiling, the real version of `ui-mockups/perf.html` · **log-based metrics + alerting policies** on outbox depth · **IAP** in front of Grafana and the admin console · SLOs · DR drill · cost review |

### Evaluated and not used

Breadth is not the goal — a service with no job teaches configuration, not architecture.
These were considered and declined, and the reasons are worth as much as the adoptions.

| Service | Why not |
|---|---|
| **NATS on GKE** | The alternative to a second `EventBus` adapter — one bus everywhere, at the price of a permanently running broker, which is the one thing the budget cannot absorb (ADR-010). Hosting it on Kubernetes would have taught `StatefulSet` and Persistent Disk, both of which Phase 7 already teaches with Tantivy, and the broker mechanics are still learned on local NATS |
| **Workflows** | Would orchestrate the delete saga — and ADR-001 chose **choreography, no central coordinator**, deliberately. Adopting it would reverse an architectural decision to gain a service |
| **Spanner, Bigtable, Firestore** | No job. Postgres is chosen (ADR-003) and the data is relational and small |
| **Dataflow, Dataproc** | Wildly oversized for a workload measured in megabytes |
| **Cloud Build** | GitHub Actions is more transferable and already carries the pipeline |
| **Identity Platform** | Auth is Phase 2 and building it is the point (Argon2id, RS256, rotation) |

> Being able to say *"we evaluated Workflows and chose choreography for this reason"* demonstrates
> more GCP judgement than having used it. The declines are part of the coverage.

### Why Cloud Run first and GKE at Phase 3

`document-service` and `auth-service` are stateless request/response services. Cloud Run
deploys them in one Terraform resource, scales to zero, and costs nothing between
sessions — so Phases 1 and 2 get real cloud experience without a billed control plane.

`collaboration-service` is where that stops working: it is a doc-actor with sticky
consistent-hash routing and in-memory state (ADR-001), which Cloud Run cannot express.
**Hitting that wall is the lesson.** Migrating a working system from Cloud Run to GKE
because the architecture demanded it teaches more than starting on Kubernetes because a
roadmap said so.

### The rule that keeps this affordable

Every increment follows the same loop:

```
   build locally  →  k3d/compose  →  terraform apply  →  verify  →  terraform destroy
```

Nothing stays running between sessions. `terraform destroy` is not cleanup, it is the
proof that the IaC works — a stack you are afraid to destroy is one you have not
automated.

### The budget, and the two tiers it forces

**≤ $10/month while learning · < $2/month with everything destroyed · $0 preferred.**

The goal is still to touch as much of GCP as possible — the budget does not narrow the
syllabus, it decides *how long each service is allowed to exist*. That splits the estate
in two, and every service belongs to exactly one tier.

**Tier R — Resident.** Survives between sessions because the portfolio link must answer.
Must be free or near-free at idle, which in practice means scale-to-zero or a free tier.

| Service | Always-free limit | Its job here |
|---|---|---|
| **Cloud Run** | 2M requests · 180K vCPU-s · 360K GiB-s /mo | the monolith, and later every extracted service. Speaks gRPC natively |
| **Pub/Sub** | 10 GiB messages/mo | the cloud `EventBus` adapter — see § Why the bus is Pub/Sub here |
| **Cloud Storage** | 5 GB-months · 5K Class A · 50K Class B | images, snapshots, Terraform state |
| **Secret Manager** | 6 active versions · 10K access ops/mo | config; `Settings` reads it at boot |
| **Artifact Registry** | 500 MB | images. Prune tags or this is the first thing to exceed |
| **Cloud Build** | 2,500 build-min/day · 120 builds/day | CI |
| **Cloud Scheduler** | 3 jobs | outbox poller, snapshot job, nightly backup — exactly three |
| **Cloud Logging** | 50 GiB ingest/mo · 30-day retention | logs |
| **Cloud Monitoring** | all GCP metrics · 500 custom · alerting | outbox depth, op-log lag |
| **Firebase Hosting** | 10 GiB · 360 MB/day · custom domain + TLS | the SPA. Replaces Cloud CDN + Load Balancer, which are not free |
| **Firestore** | 1 GiB · 50K reads/day | runtime config (Phase 20 already names it) |
| **BigQuery** | 1 TiB queries/mo · 10 GiB storage | the cloud `AnalyticsSink` (Phase 17 already names it) |
| **Compute Engine `e2-micro`** | 1 VM/mo in `us-west1`/`us-central1`/`us-east1` + 30 GB disk | the escape hatch for anything stateful that must persist — NATS JetStream or Redis |

**Tier S — Session.** Rented by the hour, `terraform destroy` at the end. This is where
the expensive-but-educational services live, and renting them is what keeps the syllabus
complete without keeping the bill.

| Service | Why it cannot be resident | Rough hourly |
|---|---|---|
| **GKE** | cluster + node fees accrue whether or not anything runs | ~$0.15/hr with one small node |
| **Cloud Load Balancing** | a forwarding rule bills continuously; **never free**, ~$18/mo resident | ~$0.03/hr |
| **Cloud SQL** | no scale-to-zero at any tier | ~$0.02/hr at `db-f1-micro` |
| **Memorystore** | provisioned, always on | ~$0.05/hr at 1 GB basic |
| **Cloud Armor · Cloud CDN · Cloud NAT · gVisor pools** | all attach to the above | — |

A four-hour GKE session with a load balancer and Cloud SQL lands near **$1**. Ten sessions
a month fits the ceiling with room to spare. Treat these numbers as order-of-magnitude —
the **budget alert is the real control**, not this table.

### The two gaps

Neither has a scale-to-zero answer inside GCP, and both are Tier R needs:

1. **Postgres.** Cloud SQL cannot idle at zero and AlloyDB is worse. Use an external
   serverless Postgres (Neon, Supabase) for the resident deployment and keep Cloud SQL for
   Tier S sessions, where the point is learning its Terraform surface, replicas, and
   `pgvector`. The wire protocol is the portability layer, so this costs nothing in code.
2. **Redis.** Memorystore is always on. In the monolith, prefer an in-process cache behind
   the same trait; if presence genuinely needs Redis before extraction, run it on the free
   `e2-micro`.

### Why the bus is Pub/Sub here

NATS JetStream is stateful — it needs a disk, therefore a machine. In a Tier R estate where
everything else idles at zero, it would be the only always-on component and the single largest
line on the bill.

Pub/Sub cannot replace it outright either: there is no self-hostable equivalent, and self-hosting
is an ADR-001 requirement. So `EventBus` carries two adapters, which is the entire point of having
the trait — `NatsBus` for local development and self-hosting, `PubSubBus` for the resident GCP
deployment. `NatsBus` defines the delivery semantics both must satisfy.

The price is doubled delivery semantics, tested twice: ADR-010 §2 and `DATA_MODEL.md` §10.

> The table in § *Cloud Increments* above is the **learning** order and assumes Tier S. This
> section governs what is allowed to *stay*.

---

## 3. Foundations

### Step 1 — CI/CD before any cloud

1. `ci.yml` — `cargo fmt --check`, `cargo clippy -- -D warnings`, `cargo test`, with [`Swatinem/rust-cache`](https://github.com/Swatinem/rust-cache)
2. The **OpenAPI drift gate** — regenerate the TypeScript client and fail on a dirty diff (ADR-004), plus a `protoc` codegen check for `crates/proto` (ADR-007)
3. `cd.yml` — multi-stage builds with [`cargo-chef`](https://github.com/LukeMathWalker/cargo-chef), distroless runtime, push to Artifact Registry
4. A `cargo sqlx prepare` check, so compile-time query verification works without a live database in CI

### Step 2 — IaC with Terraform

**The console is never used.** Everything is code. State lives in a Cloud Storage bucket with versioning enabled — configure the backend before creating anything else, or the first real resource is stranded in local state.

1. **Networking** — VPC, subnets with secondary ranges for GKE pods and services, firewall rules
2. **Data tier** — Cloud SQL and Memorystore on private IPs, reachable only from the cluster's ranges
3. **Storage** — one bucket for files and snapshots, one for the static client, one for Terraform state; least-privilege IAM bindings
4. **Secrets** — Secret Manager entries, with `roles/secretmanager.secretAccessor` granted to each service's Google service account and nothing broader

Terraform-specific discipline worth learning deliberately: `plan` is read before every `apply`, `terraform fmt` and `validate` run in CI, and nothing is ever changed by hand — a drifted resource is fixed by importing or destroying it, never by clicking.

### Step 3 — Kubernetes locally first (costs nothing), before Phase 3

> Do not learn Kubernetes on a billed control plane.

`k3d` or `kind` teaches Deployments, Services, ConfigMaps, Secrets, Gateway/Ingress, HPA, probes, and rolling updates **identically** to GKE. Write and debug every manifest here.

Reserve GKE for what genuinely cannot be simulated: Workload Identity, real Cloud SQL behaviour, load balancer provisioning by the Gateway controller, and actual Terraform provider semantics.

### Step 4 — GKE, arriving at Phase 3

1. Zonal cluster and node pool via Terraform, Workload Identity enabled on the cluster
2. Deployments, Services, ConfigMaps, Secrets for every service
3. GKE Gateway API — a `Gateway` provisions a Cloud Load Balancer with a managed certificate
4. **Two HPA strategies**, which is the interesting part:

| Service | Metric | Why |
|---|---|---|
| `collaboration-service` | **custom: WebSocket connections per pod** | CPU stays low while thousands of sockets are held. CPU-based scaling would never trigger |
| `diagnostics-service` | **CPU** | Genuinely CPU-bound and bursty |

5. `PodDisruptionBudget` on `collaboration-service` — it is stateful, so eviction must be paced
6. Readiness probes that actually check dependencies, not just process liveness

### Step 5 — Observability, at Phase 12

1. Structured JSON logs to stdout → Cloud Logging picks them up on GKE with no agent to deploy. `APP__TELEMETRY__FORMAT=json` is all that is needed
2. Managed Service for Prometheus scraping `/metrics`
3. OTel Collector → Cloud Trace
4. Grafana: RED dashboards, plus **op-log lag** and **outbox depth** as the two panels that actually indicate trouble
5. SLOs — availability and p99 op-acknowledgement latency

---

## 4. Required Code Properties

Ports and adapters mean the domain needs no changes (`CLOUD_PORTABILITY.md`). Three things in the Rust do:

**Workload Identity, not static keys.** `BlobStore` must use the default credential chain so the projected service-account token is picked up. Hardcoding a provider that works locally and silently ignores the token in cluster is the classic failure. Reaching Cloud Storage through its S3-compatible API would force static HMAC keys — do not.

**Health endpoints.** `/health/live` and `/health/ready` on every service. Readiness must check the Postgres pool, NATS connection, and Redis — a pod reporting ready before its pool is warm receives traffic it cannot serve.

**Graceful `SIGTERM` draining.** This is the real work, and `collaboration-service` is the hard case:

```
   SIGTERM → stop accepting new sessions
           → flush the ArrayQueue
           → sync the WAL
           → close WebSockets with a reconnect hint
           → close the Postgres pool
           → exit
```

A naive shutdown drops unflushed ops. Kubernetes rolling updates make this routine, not exceptional — so it must be correct.

---

## 5. Cost Discipline

Left running, this architecture costs roughly:

| Resource | ~USD/month |
|---|---|
| GKE control plane (one zonal cluster) | 0 — the free-tier credit covers it |
| Node pool, one `e2-medium` | 25 |
| Cloud NAT | 32+ |
| Cloud SQL `db-f1-micro` | 9 |
| Memorystore Basic 1 GB | 25 |
| Cloud Load Balancing | 18 |
| **Idle total** | **~110**, or **~50** without NAT and Memorystore |

The waived control plane is the single biggest structural saving against EKS, but it does
not make an abandoned stack free — **node pool, NAT, and Memorystore bill by the hour
regardless of traffic.** That is the realistic risk to this project, far more than scale.

**In order of value:**

1. **Learn K8s on `k3d`** (Step 2.5). Free, and identical for everything except Workload Identity and managed services.
2. **`terraform destroy` after every session.** This is the point of IaC — rebuilding takes ~20 minutes and proves the code works. A stack you are afraid to destroy is a stack you have not really automated.
3. **Skip Cloud NAT while learning.** Nodes with external IPs and tight firewall rules is poor production practice and saves $32/month; you will understand *why* it is poor having done both.
4. **A $10 budget alert with an email channel, in the first Terraform run.** Not later. GCP budgets are themselves a Terraform resource, so this costs one block.
5. **Redis on GKE instead of Memorystore while learning.** Memorystore has no free tier and no scale-to-zero; a `redis:7-alpine` Deployment teaches the same integration for nothing. Swap to Memorystore for one session to see the managed difference, then swap back.
6. **`fake-gcs-server` or MinIO** for storage iteration without touching real GCS. There is no LocalStack equivalent that covers GCP well — the `object_store` abstraction is what makes this painless.

---

## 6. Scale Notes — Reason, Don't Build

Self-hosted Marginal serves a team. If it ever needed more, the order things break:

1. **Postgres write throughput.** Op batching (~20:1) is what makes it survivable at all. Beyond that: partition `docs.ops` by `page_id`, then read replicas. Nothing needs building now.
2. **`collaboration-service` connection count.** tokio holds 50–100k sockets per instance with tuned fd limits. Consistent hashing (Phase 10) already distributes pages across instances.
3. **Tantivy is in-process with no distributed mode.** The migration path is sharding by page or moving to [Quickwit](https://quickwit.io/), which is built on Tantivy — index-format knowledge transfers. Worth recording so Phase 7 does not look like a dead end.
4. **Diagnostics fan-out on rename.** A page rename invalidates diagnostics across every referring page. Batch or rate-limit if it becomes a spike.

Being able to say "we would partition by `page_id` and move search to Quickwit" is worth more than having built either.
