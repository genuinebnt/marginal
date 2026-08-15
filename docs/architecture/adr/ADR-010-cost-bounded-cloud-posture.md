# ADR-010 — Cost-Bounded Cloud Posture

**Date:** 2026-08-15
**Status:** Accepted
**Related:** ADR-001 (service boundaries) · ADR-002 (Rust depth) · ADR-003 (Postgres) · ADR-007 (gRPC east-west) · ADR-008 (GCP + Terraform)
**Deciders:** @genuinebasilnt

---

## Context

Two requirements that look opposed:

1. **The architecture is microservices-first.** Eleven services, each justified by ADR-001's rule —
   a service exists only if it differs in scaling profile, state, failure mode, or deploy cadence.
2. **A hard budget.** **≤ $10/month while learning, < $2/month with everything destroyed,
   $0 preferred.** A portfolio link must answer at any time, which means something has to stay
   resident.

The tension is not between the services and the budget. Cloud Run scales to zero, so eleven
services at no traffic cost the same as one — the count was never the expense. The expense is
**anything that cannot idle at zero**: GKE, load balancers, Cloud SQL, Memorystore, and any
broker that needs a disk.

---

## Decision

**The estate splits in two tiers, and every service belongs to exactly one.**

| | Tier R — Resident | Tier S — Session |
|---|---|---|
| Lifetime | survives between sessions | `terraform apply` → learn → `terraform destroy` |
| Cost model | free tier or scale-to-zero | billed hourly |
| Contains | Cloud Run, Pub/Sub, Cloud Storage, Secret Manager, Artifact Registry, Cloud Build, Cloud Scheduler, Cloud Logging/Monitoring, Firebase Hosting, Firestore, BigQuery, one free `e2-micro` | GKE, Cloud Load Balancing, Cloud SQL, Memorystore, Cloud Armor, Cloud CDN, Cloud NAT, gVisor pools |

The full table with limits is `CLOUD_ROADMAP.md` §2 § *The budget, and the two tiers it forces*.
**The budget does not narrow the syllabus** — it decides how long each service is allowed to
exist. GKE, load balancers and Cloud SQL stay on the curriculum; they are rented for a session
and destroyed. A four-hour GKE session with a load balancer and Cloud SQL lands near $1.

Three consequences are architectural rather than operational, and belong here.

### 1. Cloud Run is the resident runtime for every service

Including `collaboration-service`, which ADR-001 justifies as **stateful**. Cloud Run supports
gRPC and HTTP/2 natively, so ADR-007 is unaffected — the transport does not change, only what
schedules the container.

The stateful justification is not withdrawn; it is **unloaded**. Session affinity, the rope's
residency, and connection-count scaling are real, and they are exactly what GKE is rented for in
Tier S. Until a second concurrent editor exists, sticky routing has nothing to be sticky about,
and a single Cloud Run instance with `min-instances = 0` is not wrong — it is idle.

### 2. The `EventBus` trait carries two adapters

**`NatsBus`** for local development and self-hosting. **`PubSubBus`** for the resident GCP
deployment.

Self-hosting is an ADR-001 requirement and Pub/Sub has no self-hostable equivalent — the emulator
is for tests. JetStream, meanwhile, is stateful: it needs a disk and therefore a machine, which
under a Cloud Run posture would make it the only always-on component in an otherwise near-zero
estate. Neither adapter can serve both jobs, so there are two.

`NatsBus` is primary in the sense that matters: it is the self-host path, and it defines the
delivery semantics `PubSubBus` must satisfy.

**The cost is real and is not being minimised.** Every delivery guarantee, ordering model and
redelivery behaviour now exists twice, and every consumer must be correct under both — JetStream
gives stream sequence numbers, Pub/Sub gives ordering keys that interact differently with retries
and dead-lettering. The integration suite runs each consumer against both adapters.
`DATA_MODEL.md` §10 is the contract they share.

### 3. Database per service is realised as schema + role per service

ADR-003 specifies one database **instance** per service. Eleven instances do not fit
this budget at any provider.

**Resident:** one serverless Postgres (Cloud SQL cannot idle at zero), with a schema and a login
role per service.

**The rule does not weaken, only its enforcer changes.** With no network boundary between schemas,
the grant *is* the boundary — which is what `DATA_MODEL.md` §1 already requires:

> A cross-schema join fails at the database instead of passing review.

No cross-schema joins, no cross-schema foreign keys, independent migration cadence — all
unchanged. A service that respects its grant is extractable to its own instance by changing a
connection string.

**Session:** Cloud SQL appears in Tier S, because its Terraform surface, read replicas and
`pgvector` are on the syllabus. It is rented for those, not relied on.

---

## Consequences

**Good**

- The budget holds without narrowing the syllabus. Expensive services are rented, not owned.
- `terraform destroy` becomes load-bearing rather than hygienic — the thing that keeps Tier S
  affordable is the same thing that proves the IaC works.
- Two `EventBus` adapters make the port real. A trait with one implementation is a guess.

**Bad, and accepted**

- **Two delivery semantics**, tested twice, forever.
- **A resident deployment that is not the deployment.** Cloud Run on a shared Postgres instance is
  not the topology ADR-001 describes. The gap is deliberate and belongs on the portfolio page
  rather than hidden — the architecture is the design; Tier R is what is currently paid for.
- **An external Postgres.** It sits outside the Terraform-managed estate and outside GCP IAM.

---

## Alternatives considered

**A modular monolith first, extracting services later.** The usual and often correct order:
boundaries drawn after the domain is understood, one deploy instead of eleven.

Declined because the objective is learning a **genuinely scalable microservice architecture**
(ADR-001, ADR-002), and the extraction would be indefinitely deferrable once a working monolith
existed. Cloud Run also removes the argument's strongest leg — scale-to-zero means the service
count is not what costs money.

The cost of the choice is real and is not being pretended away: eleven deploys, eleven configs, no
joins anywhere, and distributed debugging from the first week, all before the boundaries have met
a real workload. ADR-001's rule is therefore doing its work **on the design**, not on observed
load, and every boundary it justifies is a prediction that may turn out wrong.

**GKE as the resident runtime.** ~$0.15/hour with one small node is roughly $110/month resident.
Tier S for that reason alone.

**Cloud SQL as the resident database.** No scale-to-zero at any tier; a shared-core instance
carrying no traffic still bills. Kept in Tier S for its Terraform surface.

**NATS on GKE as the cloud bus.** Would remove the second adapter and its doubled semantics, at
the price of a permanently running broker — the one thing the budget cannot absorb. Hosting a
broker on Kubernetes would have taught `StatefulSet` and Persistent Disk, both of which Phase 7
already teaches with Tantivy, and the broker mechanics are still learned on local NATS.

---

## Revisit when

- A second concurrent editor exists — `collaboration-service` then has the load its stateful
  justification predicts, and Tier R has to answer for it.
- Artifact Registry exceeds 500 MB, or Pub/Sub exceeds 10 GiB/month. Both are the first free
  tiers this design will cross.
- A managed Postgres appears that idles at zero inside GCP.
