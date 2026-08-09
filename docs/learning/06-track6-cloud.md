# Track 6 — Cloud hardening · Phases 11, 12

`11 Containers/CI + self-host ops → 12 Observability + hardening`

> **Cloud is interleaved, not deferred.** Each phase deploys its own service as part of that phase
> (`CLOUD_ROADMAP.md` §2). Phase 1 already includes Terraform, Cloud SQL, Cloud Storage, Secret
> Manager and a Cloud Run deploy. **These two phases are the *hardening* pass, not first contact.**

So this file has an extra section at the top: **§0 What you need at Phase 1**, because you will
touch GCP long before you reach Phase 11.

---

# §0 What you need at Phase 1 — the cloud minimum

Read only this much now. The rest of the file waits.

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**Terraform — Language docs**](https://developer.hashicorp.com/terraform/language) + the [GCP provider getting-started](https://developer.hashicorp.com/terraform/tutorials/gcp-get-started) | docs/tutorial | Resources, variables, outputs, and **state**. State is the concept people skip and then corrupt |
| [**Terraform state and remote backends**](https://developer.hashicorp.com/terraform/language/state) + [GCS backend](https://developer.hashicorp.com/terraform/language/settings/backends/gcs) | docs | **Remote state in a GCS bucket, with locking, from the first commit.** Local state on a laptop is how an environment becomes unreproducible |
| [Google Cloud — **Cloud SQL for PostgreSQL** overview](https://cloud.google.com/sql/docs/postgres) + [private IP](https://cloud.google.com/sql/docs/postgres/configure-private-ip) | docs | Private IP, not a public one with an allowlist. Also: **which Postgres version Cloud SQL actually offers** — `CLOUD_PORTABILITY.md` § The `uuidv7()` trap depends on this |
| [**Secret Manager**](https://cloud.google.com/secret-manager/docs/overview) + [accessing secrets from Cloud Run](https://cloud.google.com/run/docs/configuring/services/secrets) | docs | Secrets injected as env at deploy, never baked into an image, never in a ConfigMap |
| [**Workload Identity Federation** — best practices](https://docs.cloud.google.com/iam/docs/best-practices-for-using-workload-identity-federation) | docs | **No static service-account keys, ever.** Google is already restricting key creation by default org policy. Read before writing your first IAM binding |
| [Cloud Run — container contract](https://cloud.google.com/run/docs/container-contract) | docs | Port from `$PORT`, statelessness, and the shutdown signal. Your graceful drain depends on this being right |

### Optional at Phase 1

| Resource | Type | Why |
|---|---|---|
| [Terraform — modules](https://developer.hashicorp.com/terraform/language/modules) | docs | Wait until you have duplication. Premature modules are worse than none |
| [`gcloud` CLI cheat sheet](https://cloud.google.com/sdk/docs/cheatsheet) | docs | Useful reference, not reading |
| [Google Cloud — Architecture Framework](https://cloud.google.com/architecture/framework) | docs | Skim the *Reliability* pillar. The rest is enterprise scaffolding you do not need |
| `CLOUD_ROADMAP.md` § cost discipline | project doc | **Read this one.** Learning-project cost control is a real skill and a real bill |

---

# Phase 11 — Containers, CI & Self-host Operations

**Where the project becomes something someone else can run.** Multi-stage builds, image size,
supply chain, and a restore drill that actually restores.

**What you must be able to decide alone at the end:** how to build a small Rust image with a warm
cache, what "consumers deploy before producers" means for your op log, and whether your backup
works — which is a different question from whether it runs.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**Docker multi-stage builds**](https://docs.docker.com/build/building/multi-stage/) + [`cargo-chef`](https://github.com/LukeMathWalker/cargo-chef) | docs/repo | **`cargo-chef` is the answer to Rust's Docker layer-caching problem.** Without it every source change rebuilds every dependency. Read its README fully |
| [**Distroless** or `scratch` for Rust](https://github.com/GoogleContainerTools/distroless) | repo | The final stage should not contain a shell. Smaller attack surface and smaller image |
| [Google Cloud — **Artifact Registry**](https://cloud.google.com/artifact-registry/docs) + [Docker auth](https://cloud.google.com/artifact-registry/docs/docker/authentication) | docs | Where images live, and how CI pushes without a key file |
| [**GitHub Actions** — Rust caching](https://github.com/Swatinem/rust-cache) + [OIDC to GCP](https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/configuring-openid-connect-in-google-cloud-platform) | repo/docs | Keyless CI→GCP auth via Workload Identity Federation. **The right way, and not much harder than the wrong way** |
| [`cargo-deny` book](https://embarkstudios.github.io/cargo-deny/) + [`cargo-audit`](https://github.com/rustsec/rustsec/tree/main/cargo-audit) | book/repo | Licences, advisories, duplicate dependencies. `ROADMAP.md` calls this "the cheapest real security work available" and means it |
| [**SLSA** framework](https://slsa.dev/spec/v1.0/levels) — levels 1–2 only | spec | Provenance for your build. Level 1–2 is achievable in a weekend and is the vocabulary for supply-chain claims |
| **DDIA** Ch. *Encoding and Evolution* — the **rolling upgrade** section | owned | **Consumers deploy before producers, always.** Old and new code read the same op log during a rollout, and this is the rule that makes that safe |
| [Postgres backup — `pg_dump` vs PITR](https://www.postgresql.org/docs/current/backup.html) + [Cloud SQL PITR](https://cloud.google.com/sql/docs/postgres/backup-recovery/pitr) | docs | The difference between a backup and a *recoverable* backup, and what PITR needs configured in advance |

### Optional

| Resource | Type | Why |
|---|---|---|
| [`cargo-semver-checks`](https://github.com/obi1kenobi/cargo-semver-checks) | repo | `libs/proto` is a contract several services depend on. A silent break there is a runtime failure elsewhere |
| [Buf — breaking-change detection](https://buf.build/docs/breaking/overview) | docs | The protobuf-specific version of the same idea |
| [`sccache`](https://github.com/mozilla/sccache) | repo | If CI build times become the bottleneck. Measure first |
| [Binary Authorization](https://cloud.google.com/binary-authorization/docs) | docs | Only signed images run. Real security value, and a good Terraform exercise |
| [Trivy](https://github.com/aquasecurity/trivy) / [Grype](https://github.com/anchore/grype) | repo | Image vulnerability scanning in CI |
| [The Twelve-Factor App](https://12factor.net/) | article | Read once, in full. Half of it is now obvious and the other half is still ignored by most projects |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| **Do a restore drill.** Delete a database and restore it from backup, timed | Not a resource. `admin.html` claims "backups and verified restore". An untested backup is a hope, and this is the phase where hope becomes evidence |
| [Google — SRE Book](https://sre.google/sre-book/table-of-contents/) Ch. *Release Engineering* | Free. Reads as bureaucracy before you have a deploy pipeline and as good sense after |

---

# Phase 12 — Kubernetes, IaC, Observability

**The final hardening pass.** GKE, OpenTelemetry, Prometheus, and the operational discipline
`admin.html` and `perf.html` already draw.

**What you must be able to decide alone at the end:** what an SLO is and how it differs from a
metric, which four signals to alert on, why traces and logs must share a correlation id, and what
a stateful workload needs from Kubernetes that a stateless one does not.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**Kubernetes concepts**](https://kubernetes.io/docs/concepts/) — Workloads, Services, ConfigMap/Secret, Probes | docs | The minimum. Read the **probes** page twice: `liveness` restarting a healthy-but-slow pod is a classic self-inflicted outage |
| [**StatefulSet**](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) + [pod termination lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-termination) | docs | `collaboration-service` is stateful and holds long-lived connections. **`terminationGracePeriodSeconds` must exceed your drain timeout** or Kubernetes kills you mid-flush |
| [**GKE Gateway API**](https://cloud.google.com/kubernetes-engine/docs/concepts/gateway-api) | docs | Ingress is deprecated in favour of Gateway. Managed TLS and the routing model |
| [**GKE Workload Identity Federation**](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) | docs | The in-cluster half of the Phase 1 decision. No static keys in the cluster |
| [**Google SRE Book** — *Service Level Objectives*](https://sre.google/sre-book/service-level-objectives/) + [*Monitoring Distributed Systems*](https://sre.google/sre-book/monitoring-distributed-systems/) | free book | **The two chapters that matter.** SLI/SLO/SLA distinguished properly, and the four golden signals: latency, traffic, errors, saturation |
| [**SRE Workbook** — *Alerting on SLOs*](https://sre.google/workbook/alerting-on-slos/) | free book | Burn-rate alerting. The difference between an alert that helps and a pager that gets ignored |
| [**OpenTelemetry** concepts](https://opentelemetry.io/docs/concepts/) — traces, spans, context propagation | docs | Read *context propagation* carefully. A trace that breaks at a service boundary is worse than no trace, and gRPC metadata is where yours is carried |
| [`tracing` + `tracing-opentelemetry`](https://docs.rs/tracing-opentelemetry/) + [SigNoz's Rust OTel guide](https://signoz.io/blog/opentelemetry-rust/) | docs/blog | The Rust side. `tracing` spans → OTLP. The guide is current and practical |
| [**Prometheus** — metric types](https://prometheus.io/docs/concepts/metric_types/) + [histograms and summaries](https://prometheus.io/docs/practices/histograms/) | docs | **Histograms vs summaries and why you almost always want histograms.** Also why averaging a percentile is meaningless — which `perf.html` already asserts |
| [Gil Tene — **How NOT to Measure Latency**](https://www.youtube.com/watch?v=lJ8ydIuPFeU) | talk (~45 min) | **Coordinated omission.** The single most important talk on latency measurement. Your load generator is probably lying to you and this explains exactly how |

### Optional

| Resource | Type | Why |
|---|---|---|
| [Managed Service for Prometheus](https://cloud.google.com/stackdriver/docs/managed-prometheus) | docs | The GCP-native path. Same `/metrics` endpoint |
| [Cloud Trace](https://cloud.google.com/trace/docs) + [Cloud Profiler](https://cloud.google.com/profiler/docs) | docs | Profiler is genuinely good and there is no local equivalent. Worth a phase row of its own |
| [Grafana dashboard design](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/) | docs | `admin.html` and `perf.html` are dashboard designs. This is how to not ruin them in Grafana |
| [`pprof-rs`](https://github.com/tikv/pprof-rs) / [`cargo-flamegraph`](https://github.com/flamegraph-rs/flamegraph) / [`dhat`](https://docs.rs/dhat/) | repo/docs | Where the time and allocations actually go. `perf.html` draws flame graphs; these produce them |
| [Brendan Gregg — flame graphs](https://www.brendangregg.com/flamegraphs.html) | site | The inventor's own page. Read *how to read one* before generating a hundred |
| [Kubernetes the Hard Way](https://github.com/kelseyhightower/kubernetes-the-hard-way) | tutorial | Only if you want to understand the control plane. GKE hides all of it, deliberately |
| [Terraform — testing](https://developer.hashicorp.com/terraform/language/tests) + [`terraform plan` in CI](https://developer.hashicorp.com/terraform/tutorials/automation/automate-terraform) | docs | Plan on PR, apply on merge. The discipline that stops IaC drift |
| [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/) | spec | Attribute naming. Boring and it is what makes traces queryable later |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| Define **one** SLO for the editing path and alert on its burn rate | Not a resource. `p99 ack latency` is the editor's felt performance. One real SLO beats twenty dashboards |
| Kill a `collaboration-service` pod under load and watch the drain | The graceful-shutdown code from Phase 3 either works under a real `SIGTERM` with a real grace period or it does not |
| [SRE Book](https://sre.google/sre-book/table-of-contents/) Ch. *Postmortem Culture* | Short. Then write one for the outage you just caused on purpose |

### Optional

| Resource | Why |
|---|---|
| [Marc Brooker — blog](https://brooker.co.za/blog/) | Read broadly now. Everything he writes about operations will land differently once you have operated something |
| [Dan Luu — blog](https://danluu.com/) | Especially the posts on measurement and on why systems fail in ways nobody predicted |
| [Jepsen analyses](https://jepsen.io/analyses) | Full circle from Phase 3 |