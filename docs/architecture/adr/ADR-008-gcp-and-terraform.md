# ADR-008 — Google Cloud and Terraform

**Date:** 2026-08-07
**Status:** Accepted
**Related:** ADR-001 (scope) · ADR-002 (Rust depth) · ADR-010 (cost-bounded cloud posture)
**Deciders:** @genuinebasilnt

---

## Context

The cloud track was written for AWS with Pulumi's Rust SDK, on the reasoning that AWS
teaches the most transferable skills and that Rust IaC serves ADR-002. Two pressures
changed the calculus.

**Cost during learning.** The cluster exists only while learning and is destroyed after
each session. Under that usage pattern GKE waives the control-plane management fee for
one zonal cluster, while EKS bills $0.10/hour whether or not a node is attached. New
account credits are $300/90 days on GCP against roughly $200 on AWS. An accidentally
abandoned stack costs ~$120/month on AWS and near-nothing on GCP.

**Pulumi's Rust SDK is not officially supported.** Pulumi ships TypeScript, Python, Go,
C#, Java, and YAML. Rust support is community work. Building the infrastructure track on
an unofficial SDK risks the whole of Phase 12 on a dependency that may stall — and the
alternative within Pulumi is writing IaC in Go, which serves ADR-002 no better than HCL.

---

## Decision

**Deploy to Google Cloud. Write infrastructure in Terraform.**

The application code does not change. Every cloud dependency already sits behind a trait
chosen by configuration at startup (`CLOUD_PORTABILITY.md` §2), which is the property that
makes this a documentation change rather than a rewrite.

### Service mapping

| Concern | GCP |
|---|---|
| Kubernetes | GKE — one zonal cluster, Standard mode. **Tier S only** (ADR-010): rented per session, never resident |
| Database | **Serverless Postgres** resident; Cloud SQL is a **Tier S** rental for replicas and `pgvector` (ADR-010 §3) |
| Cache / presence | In-process behind the trait, or the free `e2-micro`. Memorystore is **Tier S** — provisioned and always billing |
| Event bus | **Pub/Sub**. JetStream is stateful and cannot idle at zero, so `NatsBus` is the local and self-host adapter and `PubSubBus` is the cloud one (ADR-010 §2) |
| Object storage | Cloud Storage |
| Secrets | Secret Manager |
| Container registry | Artifact Registry |
| Ingress | The **Cloud Run service URL**. Cloud Load Balancing is **Tier S** — a forwarding rule bills continuously and is never free |
| Static client | **Firebase Hosting**. Cloud CDN needs a load balancer, so it is **Tier S** |
| Traces | Cloud Trace via the OTel Collector |
| Metrics | Google Cloud Managed Service for Prometheus |
| Logs | Cloud Logging |
| Pod identity | Workload Identity Federation |

---

## Consequences

### `uuidv7()` cannot be relied on in the cloud

`migrations/0001_init.sql` uses `DEFAULT uuidv7()`, a **PostgreSQL 18** built-in. Managed
Postgres lags upstream by a year or more — this applies to Cloud SQL and to RDS equally,
so it is not a GCP-specific problem, but the move surfaces it now rather than in Phase 12.

**The fix is already the design.** `PageId::new()` generates UUIDv7 in Rust
(`uuid` crate, `now_v7`). Ids are assigned by the application, and the column default is
belt-and-braces for hand-written SQL only. Keep it that way: **never write an `INSERT`
that omits the id and relies on the database to generate it**, or the code stops working
the moment it meets a managed Postgres.

If a managed instance offers only Postgres ≤ 17, drop the `DEFAULT` clause in a migration
rather than changing how ids are made.

### Object storage stops being S3-shaped

The old `S3Store` adapter assumed `aws-sdk-s3` against both MinIO and S3. Cloud Storage
has an S3-compatible XML API, but reaching it needs static HMAC keys, which defeats
Workload Identity — the exact anti-pattern the old ADR warned about with IRSA.

Use the [`object_store`](https://docs.rs/object_store) crate instead. It covers local
filesystem, MinIO/S3, and GCS behind one API, so `ObjectStore` keeps a single
implementation and the local/cloud difference stays configuration. Presigned PUT for
browser upload works on GCS via V4 signing.

### Workload Identity replaces IRSA

Same shape, different mechanism: a Kubernetes service account is bound to a Google
service account, and the client library sources credentials from a projected token. No
static keys in cluster. The failure mode is identical — hardcoding a credential provider
that works locally and silently ignores the projected token in cluster.

### IaC is no longer Rust

Terraform is HCL. This is a real, accepted loss against ADR-002: Phase 12 now teaches
infrastructure rather than more Rust. The trade is deliberate — an officially supported
tool with a large provider ecosystem beats an unofficial SDK, and the alternative that
kept Pulumi would have meant Go, which is not Rust either.

Terraform state lives in a Cloud Storage bucket with versioning enabled. Everything is
one root module with per-environment `tfvars`; no workspaces at this scope.

### Transferability is the accepted cost

AWS has roughly 30% market share to GCP's 13%, and its IAM and VPC models are where the
genuinely transferable difficulty lives. GCP is easier to learn, which means it teaches
less per hour. Accepted: cost and momentum matter more for a project that exists to be
built, and the concepts map one-to-one — Workload Identity *is* IRSA, Cloud SQL *is* RDS.

### Cost guardrails are part of the deliverable

Two things exist before the first `terraform apply`: a billing budget alert at $10, and
no Cloud NAT in the learning VPC unless something demonstrably needs egress. `terraform
destroy` at the end of every session is the discipline that makes the whole track cost
$20–40 rather than a monthly bill.

---

## Google Cloud is the primary deployment target

**Google Cloud is the primary hosting target and a project requirement**, not an optional
learning track alongside a self-hosted product. The reference
deployment is GKE and Cloud Run in a real project; `docker compose up` remains supported
for self-hosting and for local development, but it is no longer the definition of "running".

### What that means

- **A phase is not done until it is deployed.** `CLOUD_ROADMAP.md` §2 already attaches a
  cloud increment to every phase; that increment is now part of the phase's definition of
  done rather than an optional extra.
- **Cloud-only failure modes are in scope.** Cold starts, Workload Identity token refresh,
  Cloud SQL failover, and a rolling replacement of a `StatefulSet` are things this project
  must handle, not things it may.
- **Cost discipline becomes a real constraint**, not advice. `terraform destroy` between
  sessions and a $10 budget alert are the mechanism that makes a required cloud target
  affordable on a learning budget.

### What it does not change

**Ports and adapters stay** (`CLOUD_PORTABILITY.md` §2). The trait indirection is not
scaffolding for self-hosting that can now be deleted — it is what makes integration tests
run against real local infrastructure instead of mocks, and what keeps a single cloud from
becoming an untestable dependency. Self-hosting remains a supported path and a product
capability (ADR-001).

### The event bus takes two adapters

Pub/Sub is capable — `seek()` to a timestamp or snapshot, retention to 31 days, ordering keys,
dead-letter topics — and it idles at zero. JetStream is stateful, so in the cloud it would be the
only always-on component in the estate.

It cannot simply replace NATS, because Pub/Sub has no local production equivalent and self-hosting
is an ADR-001 requirement. So `EventBus` carries both: `NatsBus` locally and self-hosted,
`PubSubBus` in the cloud (ADR-010 §2).

The price is paid knowingly — every delivery guarantee, ordering model and redelivery behaviour
exists twice, with every consumer correct under both. `DATA_MODEL.md` §10 is the contract they
share, and the integration suite runs each consumer against both.

---

## Alternatives considered

**Stay on AWS with Terraform.** Keeps transferability, fixes the Pulumi problem, costs
more. Rejected on the EKS control-plane fee under a spin-up/tear-down usage pattern.

**GCP with Pulumi (TypeScript or Go).** Rejected: if IaC cannot be Rust, HCL's ecosystem
and job-market presence beat a second general-purpose language in the stack.

**GKE as the resident runtime.** Kubernetes is load-bearing for `collaboration-service`'s sticky
consistent-hash routing (ADR-001), and Cloud Run cannot express it. But a cluster bills whether or
not anything runs, so GKE is **rented per session** and Cloud Run is the resident runtime
(ADR-010 §1). The stateful justification is not withdrawn, it is unloaded: with no second
concurrent editor, sticky routing has nothing to be sticky about. GKE becomes resident when the
load exists.

---

## Resources

| Resource | For |
|---|---|
| [Terraform GCP provider](https://registry.terraform.io/providers/hashicorp/google/latest/docs) | Resource reference |
| [GKE Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) | Pod → Google service account binding |
| [`object_store` crate](https://docs.rs/object_store) | One trait over local, S3/MinIO, and GCS |
| [GCS V4 signed URLs](https://cloud.google.com/storage/docs/access-control/signed-urls) | Presigned browser upload |
| [Terraform GCS backend](https://developer.hashicorp.com/terraform/language/settings/backends/gcs) | Remote state |
