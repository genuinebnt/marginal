# Track 1 — Terraform (Google Cloud)

Deploys the three Track 1 MVP services — `document-service` (:8001),
`auth-service` (:8006), `collaboration-service` (:8002), per `CLAUDE.md`'s
service table — to Google Cloud, plus three learning-exposure pieces added
later: static frontend hosting for `web/`, Cloud Build CI for the three
services, and Pub/Sub topics for the outbox events that already exist in
the Go code (see their own sections below). Written against `ADR-008` (GCP +
Terraform), `ADR-010` (cost-bounded cloud posture), `CLOUD_ROADMAP.md`, and
`CLOUD_PORTABILITY.md`. No `terraform init/plan/apply` has been run against
this — no GCP credentials exist for this repo yet. Self-review only; see
"Before you apply" below for what to check once credentials exist.

## What this provisions

| Resource | Count | Why |
|---|---|---|
| Cloud Run v2 service | 3 (one per service) | `ADR-010` §1: Cloud Run is the resident runtime for every service, `min_instance_count = 0`, including `collaboration-service` — its stateful justification is "unloaded," not withdrawn, until a second concurrent editor exists |
| Cloud SQL for PostgreSQL | 3 (one per service) | See "Postgres: which tier this actually is" below — this is the Tier S shape, not the Tier R one |
| Compute Engine `e2-micro` running Redis | 1, shared | `CLOUD_ROADMAP.md`'s Tier R table names this VM explicitly as the resident answer for Redis; Memorystore is Tier S (always billing, no scale-to-zero) |
| VPC + subnet + Serverless VPC Access connector + Private Services Access peering | 1 each | Lets Cloud Run reach Cloud SQL/Redis on private IPs — `CLOUD_ROADMAP.md` § Step 2 |
| Secret Manager secret per service | 3 | Each service's `DATABASE_URL`, least-privilege accessor granted only to that service's own Cloud Run SA |
| Artifact Registry repository | 1 | Image storage; nothing pushes to it automatically — see "Setup" |
| Billing budget + email notification channel | 1 | `ADR-008`: "a billing budget alert at $10 ... exists before the first `terraform apply`" |
| GCS bucket (static website hosting) | 1 | Serves `web/dist/` (the React 19 + TS + Vite SPA) — see "Frontend hosting" below for why a bucket, not Cloud CDN + Load Balancer |
| Pub/Sub topic + pull subscription | 1 per real outbox event type (currently 1: `collab.ops_flushed`) | See "Pub/Sub" below — provisioned ahead of the Go-side consumer, not after it |
| Cloud Build trigger | 3 (one per service) | Replaces the manual `docker build && docker push` steps in "Setup" — see "Cloud Build" below |
| Cloud Build repo connection (`google_cloudbuildv2_connection` + `_repository`) | 1 | Wires the triggers above to this repo's GitHub source — needs a manual one-time step, see "Cloud Build" below |

Deliberately **not** provisioned, and why:

- **NATS.** `CLAUDE.md`'s stack table marks it the local/self-host
  `EventBus` adapter, not a cloud one — there's nothing to provision on GCP
  for it. (Pub/Sub, its cloud counterpart, moved from this list to the
  table above — see "Pub/Sub" below for why.)
- **Cloud Storage (app data / blob store).** Distinct from the frontend
  bucket in the table above, which serves static HTML/JS/CSS, not
  application data. Object storage for uploads/snapshots is still marked
  "(cloud, deferred)" in `CLAUDE.md`'s stack table, and no service reads or
  writes a blob store yet — that reasoning is unchanged.
- **Cloud KMS.** `CLOUD_ROADMAP.md`'s Phase 2 entry names it for the RS256
  signing key, but `auth-service` doesn't consume it yet (see "Known
  limitations" — it generates an in-memory key today). Provisioning it now
  would sit unused.
- **Memorystore, GKE, Cloud Load Balancing.** All named `ADR-010` Tier S —
  rented per session, not resident. None of the three services needs a
  cluster yet (`collaboration-service`'s sticky-routing justification has
  "nothing to be sticky about" with no second concurrent editor —
  `ADR-010` §1). Cloud Load Balancing specifically is also the road not
  taken for frontend hosting — see below.

## Cost posture (`ADR-010`)

`ADR-010`'s budget is **≤ $10/month while learning, < $2/month with
everything destroyed, $0 preferred**, achieved by splitting the estate into
**Tier R** (resident, must idle at zero or sit in a free tier) and **Tier
S** (rented by the hour, `terraform apply → learn → terraform destroy`).

Every default in this module follows that discipline: `min_instance_count
= 0` on all three Cloud Run services, `ZONAL` (no HA replica) Cloud SQL, no
Cloud SQL automated backups by default, `PD_HDD` over SSD, the smallest
VPC connector size the API allows, and a single `e2-micro` for Redis
instead of Memorystore. Nothing here defaults to a larger tier than the
docs specify — see each module's `variables.tf` for the citation next to
every non-default-obvious choice.

### Postgres: which tier this actually is

Read this before treating Cloud SQL here as "the database." The docs draw
a real distinction this Terraform has to pick a side on:

- **`CLOUD_PORTABILITY.md` §1's Tier S column:** "Cloud SQL for
  PostgreSQL, **one instance per service** (ADR-003)."
- **`ADR-010` §3 / `CLOUD_ROADMAP.md`'s "two gaps":** the actual **Tier R
  (resident)** Postgres is an **external serverless provider — Neon or
  Supabase** — because "Cloud SQL cannot idle at zero," and it explicitly
  "sits outside the Terraform-managed estate and outside GCP IAM"
  (`ADR-010` "Consequences, Bad, and accepted").

**This module provisions the Tier S shape** — one Cloud SQL instance per
service, matching `ADR-003`'s base rule and the local `docker compose`
topology — because that is the only Postgres shape Terraform can actually
own; the Tier R external database is by design not a GCP/Terraform
resource. Treat these three instances as `apply`-for-a-session,
`destroy`-after, per `ADR-010`'s own discipline ("`terraform destroy`
becomes load-bearing rather than hygienic"), **or** point at a real
external Postgres for a genuinely resident deployment:

1. Sign up for Neon or Supabase, create three databases (or three
   projects), and get three connection strings.
2. Comment out the three `module "*_service_db"` blocks and the
   `secret_env` blocks that reference them in `main.tf`.
3. Add three `google_secret_manager_secret`/`_version` resources of your
   own holding the external DSNs, and point each Cloud Run module's
   `secret_env.database_url.secret_id` at them instead.

This module does not do that switch automatically — it would mean
choosing a specific external provider on the user's behalf, which is a
product decision, not an infrastructure default.

## Networking

Cloud Run reaches Cloud SQL and the Redis VM over **private IP through a
Serverless VPC Access connector**, not the Cloud SQL Auth Proxy sidecar.
`CLOUD_ROADMAP.md`'s only stated requirement is "Cloud SQL and Memorystore
on private IPs, reachable only from the [runtime's] ranges" (written for a
GKE cluster's ranges; the connector is Cloud Run's equivalent) — it does
not mandate the Auth Proxy specifically, and the connector needs one fewer
moving part (no per-container proxy sidecar to keep patched). `sslmode` on
the resulting DSN is `require`, not `verify-full`: the connection never
leaves the VPC, so there's no public CA chain to verify against without
also standing up the Auth Proxy's own certificate machinery.

## Frontend hosting

`web/` (React 19 + TS SPA, Vite — `web/package.json`'s `build` script is
`tsc -b && vite build`, output to `web/dist/`) needs to be served from
somewhere for the app to be reachable in a browser at all. Two ways to do
that were on the table:

1. **A GCS bucket configured for static website hosting** (what
   `modules/frontend-hosting` provisions).
2. **A backend bucket behind a global external HTTPS Load Balancer +
   Cloud CDN.**

**Picked (1), the bucket.** `ADR-010`'s own Decision table puts "Cloud
Storage" in **Tier R** (resident, "free tier or scale-to-zero") and "Cloud
Load Balancing"/"Cloud CDN" in **Tier S** (rented by the hour, never scales
to zero) — `CLOUD_ROADMAP.md`'s Tier R/S tables agree. A static-website
bucket costs nothing while idle, billed only for bytes actually stored and
served; an LB + CDN backend bills an hourly global-forwarding-rule charge
from the moment it exists, whether or not anyone visits it. That is the
same reasoning `ADR-008`/`CLOUD_PORTABILITY.md` already used to prefer
**Firebase Hosting** over Cloud CDN + LB for this exact slot ("Cloud CDN
needs a load balancer, which is never free" — `ADR-008`'s Decision table,
Static client row; `CLOUD_PORTABILITY.md`'s Static client row names
Firebase Hosting as the actual long-term cloud target, not a plain GCS
bucket).

This module provisions the bucket, not Firebase Hosting, because Firebase
needs the Firebase Management API enabled and a Firebase-enabled project —
an onboarding step separate from plain GCP project creation — and its
Terraform resources (`google_firebase_hosting_site` etc.) live behind the
`google-beta` provider, which nothing else in this estate needs. A plain
GCS static-website bucket gets the identical zero-idle-cost property using
only the `google` provider already configured here, and is a same-shape
swap for Firebase Hosting later if that onboarding happens.

**No CI/build step pushes `web/dist/` into the bucket yet** — the same kind
of gap as "no Dockerfile exists yet" for the backend services (see
"Setup"). Once credentials exist and the bucket has been applied, deploy a
build manually:

```bash
cd web && npm run build
gcloud storage rsync --recursive --delete-unmatched-files \
  dist/ "gs://$(terraform -chdir=../deploy/terraform output -raw frontend_bucket_name)"
```

(`gsutil -m rsync -r -d dist/ gs://BUCKET_NAME` is the older equivalent if
`gcloud storage` isn't available.)

## Pub/Sub

`CLAUDE.md`'s stack table already names Pub/Sub as the deferred cloud
adapter for the `EventBus` port (NATS JetStream locally, Pub/Sub in the
cloud — "one `EventBus` interface, two adapters"). `modules/pubsub`
provisions one topic + one pull subscription per outbox `event_type` that
actually exists in the Go code today, found by grepping the two publishing
services (`DATA_MODEL.md`'s "Outbox — one per publishing service" section)
rather than invented:

- **`collab.ops_flushed`** —
  `services/collaboration-service/internal/opstore/opstore.go`'s
  `OutboxEventOpAppended` constant (line 29), written to `collab.outbox` in
  the same transaction as every op append and batch append.
- `document-service` contributes **zero** topics: grepping
  `services/document-service` turns up no `outbox` migration and no
  `docs.outbox` table — only a forward-reference comment in `cmd/main.go`'s
  header ("... and (later) their outbox"). There is no real event type to
  provision yet; `docs/architecture/DATA_MODEL.md`'s outbox section
  describes the intended shape, but intent isn't a `event_type` string.

Subscriptions are **pull, not push**, matching a poller model — the outbox
pattern's own doc comment says "a poller publishes it," and `CLAUDE.md`'s
`EventBus` is consumed the same way, not via a webhook target.

**Be plain about what this is: nothing in the current Go code publishes to
these topics yet.** No poller exists, and no `PubSubBus` adapter is
implemented — `document-service` and `collaboration-service` write outbox
rows to Postgres and stop there. This is infrastructure provisioned ahead
of that Go-side work landing, not after it.

That is a real change from this README's original position (see the git
history of "Deliberately not provisioned" above), which reasoned that
provisioning a bus with no consumer was exactly the "nothing is built
speculatively" violation `CLAUDE.md`'s Phase-0-is-a-backlog principle warns
against. **The underlying Go-side fact hasn't changed** — still no
publisher, still no adapter. What changed is the ask: this was
deliberately requested as GCP learning exposure for a port `CLAUDE.md`
already names as the intended next cloud adapter, and Pub/Sub's own cost
shape makes the exception cheap rather than merely convenient — it sits in
`ADR-010`'s Tier R column (resident, zero cost while idle, 10 GiB/month
free per `CLOUD_ROADMAP.md`'s free-tier table), so provisioning it ahead of
the consumer doesn't cost anything while it waits, unlike (say)
provisioning Cloud KMS or Cloud SQL ahead of their consumers would. The one
operational risk of "provisioned, nothing consumes it" — Pub/Sub's default
31-day inactivity expiration silently deleting an unused subscription — is
turned off explicitly (`expiration_policy { ttl = "" }` in
`modules/pubsub/main.tf`) rather than left to surprise whoever builds the
consumer later.

## Cloud Build

Replaces the manual `docker build && docker push` steps in "Setup" with a
Cloud Build trigger per service. Cloud Build sits in `ADR-010`'s Tier R
column too — 120 build-minutes/day and 2,500 build-minutes/month free
(`CLOUD_ROADMAP.md`'s free-tier table), billed per build-minute beyond
that, nothing running or billing between builds.

**One templated `cloudbuild.yaml`, not three.** It lives at
`deploy/terraform/cloudbuild/cloudbuild.yaml`, not the repo root — kept
inside `deploy/terraform/` so this piece's footprint stays self-contained
in the same directory the rest of this task touches, rather than also
landing a file at the repo root. Each of the three
`google_cloudbuild_trigger` resources (`modules/cloud-build/main.tf`)
points its `filename` at that same path and supplies a different
`_SERVICE_NAME` substitution (plus `_REGION`/`_REPOSITORY`), so the build
steps (`docker build -f services/${_SERVICE_NAME}/Dockerfile
services/${_SERVICE_NAME}`, then push to Artifact Registry) are written
once. Each trigger's `included_files` is scoped to its own
`services/<name>/**`, so a change to one service doesn't rebuild all three.

**Two things this doesn't finish on its own:**

1. **The GitHub connection needs a manual, one-time console step.**
   `google_cloudbuildv2_connection` creates the connection resource, but it
   comes up in `PENDING_USER_OAUTH` state — linking it to an actual GitHub
   account means visiting a Google-hosted OAuth URL by hand, once. Terraform
   cannot complete that handshake headlessly; `terraform apply` alone does
   **not** finish wiring this up. See "Setup" for the exact sequence
   (create the connection, authorize it in a browser, then apply again for
   the repository + triggers).
2. **No `Dockerfile` exists per service yet** — already a known gap (see
   "Setup" §3 below). Every trigger's build step references
   `services/<name>/Dockerfile`, so none of these triggers can actually run
   a successful build until those Dockerfiles land. Writing them is out of
   this Terraform task's scope — someone else is adding them separately in
   parallel.

## Known limitations

These are real conflicts between what the current Go code does and what
this Terraform deploys it onto. None of them are things Terraform can fix
by itself — they're flagged here rather than silently built around.

**1. `collaboration-service`'s local WAL vs. Cloud Run's ephemeral disk.**
`CLOUD_PORTABILITY.md` §6 already treats this as expected: "Local WAL ...
In cluster it is an ephemeral volume, and correctness relies on it being
flushed before shutdown, not on it surviving the pod." On Cloud Run,
`SIGTERM` during a scale-down or instance replacement gives a shutdown
grace period; as long as `internal/session`'s flush-on-shutdown path (or
an equivalent hook wired into `cmd/main.go` — not present yet in the
skeleton `main.go` this Terraform deploys today) drains the WAL to
Postgres before exit, an orderly shutdown loses nothing. An **abrupt**
kill (OOM, host failure, `SIGKILL`) still loses whatever landed in the WAL
since the last batched flush (default interval 200ms per
`docs/porting/PROGRESS.md`'s flush-loop entry) — the same risk profile
`CLOUD_PORTABILITY.md` §6 already accepts for a GKE `emptyDir` volume, not
a new one this deployment introduces. Given `ADR-010` §1's framing —
`min-instances = 0` "is not wrong, it is idle" until real concurrent load
exists — this is an acceptable gap at current scale, not a blocker.
**If this service starts carrying real concurrent editors, revisit
`min_instance_count` and the shutdown-hook wiring before trusting it.**

**2. Cloud Run forwards one port; `auth-service` opens two.**
`services/auth-service/cmd/main.go` listens on gRPC (`:9006`,
`AuthService`) and HTTP (`:8006`, health + the public
`/.well-known/jwks.json` route) as two separate listeners in the same
process. `google_cloud_run_v2_service` forwards exactly one
`container_port` to the outside world. This module picks **HTTP (`:8006`)
as the ingress port**, because JWKS needs to be publicly fetchable and
there's no gateway yet to front gRPC separately — which means
**`AuthService`'s gRPC surface is not externally reachable through this
Cloud Run deployment as it stands.** Fixing this needs an application
change (multiplexing gRPC and HTTP on one listener via h2c, or standing up
`api-gateway` — out of this repo's scope per `ADR-011`), not a Terraform
one. `document-service` doesn't have this problem: its HTTP listener is
health-only, so gRPC is correctly the ingress port and health is probed
directly on `:8001` without needing to be public.

**3. `auth-service`'s RS256 signing key is in-memory, not KMS-backed.**
`keys.NewInMemoryStore()` in `cmd/main.go` generates a fresh RSA keypair
on every process start — there's no persistence and no `Cloud KMS` call.
`CLOUD_ROADMAP.md`'s Phase 2 entry already names Cloud KMS as the intended
fix. Until that lands in the Go code: every cold start (routine with
`min_instances = 0`) mints a new keypair, so `/.well-known/jwks.json`
content differs across instances and across restarts, and any
outstanding token becomes unverifiable the moment the instance that
signed it is recycled. This Terraform does not provision Cloud KMS,
because there is no consumer for it yet (see "Deliberately not
provisioned" above) — but it means **auth-service is not yet safely
deployable with `max_instance_count > 1` or with `min_instances = 0`'s
routine cold starts**, despite both being this module's defaults. Treat
this as blocking for anything beyond a single-instance demo until the Go
side wires up KMS.

**4. The `uuidv7()` migration landmine `ADR-008`/`CLOUD_PORTABILITY.md`
warned about is already present, not hypothetical.** Both docs describe
the *design* as safe ("ids are generated as UUIDv7 by the service ... the
column default is belt-and-braces for hand-written SQL only"), but a grep
of the actual migrations shows four columns still declared
`DEFAULT uuidv7()` — `docs.pages.id`, `auth.users.id`,
`auth.refresh_tokens.id`, `collab.outbox.id` (only `collab.ops.id` is
already default-less, matching the documented design). `uuidv7()` is a
**PostgreSQL 18** builtin; this module's Cloud SQL default is
`POSTGRES_17` (conservative — Cloud SQL's available major versions weren't
checked against a live project, per this task's constraints). **Applying
these migrations against a Postgres 17 instance will fail** with
`function uuidv7() does not exist`. This is a Go-migration fix (drop the
four `DEFAULT` clauses, matching `collab.ops.id`'s existing pattern), out
of `deploy/terraform`'s scope — flagging it here because it will otherwise
surface as an opaque migration failure on first boot, not as a Terraform
error.

**5. Three of the newer pieces are provisioned ahead of what they depend
on, not after it.** All three are flagged in their own sections above, but
listed together here because they're the same shape of gap: the Cloud
Build triggers reference `services/<name>/Dockerfile`, which doesn't exist
("Cloud Build" above); the Pub/Sub topics have no publisher or `PubSubBus`
adapter on the Go side ("Pub/Sub" above); and the frontend bucket has no
CI step pushing `web/dist/` into it, only a manual `gcloud storage rsync`
("Frontend hosting" above). None of these are things Terraform can fix by
itself — they're infrastructure staged ahead of application-side work
landing, not broken wiring.

## Judgment calls made where the docs left something unspecified

- **Cloud SQL machine tier.** `ADR-010`'s own cost table cites "~$0.02/hr
  at `db-f1-micro`," which this module defaults to, but Cloud SQL's
  shared-core tier availability for Postgres specifically has changed
  before and couldn't be checked against a live project. If `apply`
  rejects it, the smallest guaranteed-valid Postgres tier is a custom
  machine type such as `db-custom-1-3840` — see the comment in
  `modules/postgres/variables.tf`.
- **VPC connector vs. Cloud Run direct VPC egress.** Neither ADR mandates
  one over the other. The connector is used for broader Terraform-provider
  documentation coverage; a comment in `modules/network/main.tf` notes
  direct VPC egress as a viable, simpler alternative.
- **No Redis `AUTH` password.** `auth-service`'s `redis.NewClient` call
  sends no password today, so setting one on the VM would just break the
  connection. Network isolation (private IP + firewall to the connector's
  CIDR only) is the actual boundary at this scale.
- **Cloud Run CPU/memory (`1` vCPU / `512Mi`) and `max_instance_count = 2`
  per service.** Not specified anywhere in the docs; chosen as a
  conservative floor for Go binaries this small, consistent with
  `ADR-010`'s "don't over-provision" instruction rather than derived from
  a cited number.
- **`allow_unauthenticated = true` on all three services.** Follows from
  there being no `api-gateway` in scope (`ADR-011`) to front them with
  IAM — not stated explicitly anywhere as the deploy-time answer, but the
  only way any of the three is reachable at all today.
- **GCS bucket over Firebase Hosting for the frontend.** The docs'
  long-term answer for the SPA is Firebase Hosting (`ADR-008`,
  `CLOUD_PORTABILITY.md`); this module ships a plain GCS static-website
  bucket instead, for the `google-beta`-provider and Firebase-project-
  onboarding reasons explained in "Frontend hosting" above. Not stated
  anywhere as an acceptable substitute — a judgment call that the
  zero-idle-cost property mattered more here than matching the exact
  named product, given nothing in this repo yet depends on a
  Firebase-specific feature (custom domains, cache invalidation on
  publish) that the bucket can't also do.
- **GCS bucket `not_found_page = "index.html"` (SPA fallback) set ahead of
  there being a router.** `web/src/App.tsx` has no client-side routing yet;
  this is set defensively for when one lands, not because anything needs
  it today.
- **1st-gen `google_cloudbuild_trigger` + 2nd-gen
  `google_cloudbuildv2_connection`/`_repository`, not the older 1st-gen
  `github` source block.** Google's docs treat the 2nd-gen connection flow
  as the current recommended path for new GitHub integrations; the 1st-gen
  `github {}` trigger block still works but is being phased toward this
  model. Not cited in any Marginal doc — a judgment call based on avoiding
  building against a path Google is already deprecating.
- **`branch_pattern` defaults to `^master$`, not `^main$`.** This repo's
  actual default branch (`git status` at the time of writing) is `master`;
  matching that rather than the more common `main` default some Cloud
  Build examples use.
- **Cloud Build triggers use the default Cloud Build service account,
  scoped down to `roles/artifactregistry.writer` on just this repo's
  Artifact Registry repository** (`modules/cloud-build/main.tf`'s
  `google_artifact_registry_repository_iam_member`), rather than a
  dedicated custom service account per trigger. Not specified anywhere;
  chosen as the smaller diff for a CI setup that doesn't yet run (see
  "Cloud Build" above) — revisit if per-trigger identity ever matters here.

## Setup

### 1. One-time GCP setup (console or `gcloud`, before Terraform)

```bash
gcloud auth login
gcloud auth application-default login

gcloud projects create YOUR_PROJECT_ID
gcloud billing projects link YOUR_PROJECT_ID --billing-account=YOUR_BILLING_ACCOUNT_ID

# State bucket — must exist before `terraform init` (ADR-008)
gcloud storage buckets create gs://YOUR_STATE_BUCKET \
  --project=YOUR_PROJECT_ID \
  --location=us-central1 \
  --uniform-bucket-level-access
gcloud storage buckets update gs://YOUR_STATE_BUCKET --versioning
```

### 2. Configure and init

```bash
cd deploy/terraform

terraform init \
  -backend-config="bucket=YOUR_STATE_BUCKET" \
  -backend-config="prefix=track1"

cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars: project_id, billing_account_id,
# budget_notification_email, and the three *_image variables
```

### 3. Build and push images (manual, until Cloud Build's Dockerfiles exist)

```bash
gcloud auth configure-docker us-central1-docker.pkg.dev

# after `terraform apply` has created the Artifact Registry repo once —
# or create it standalone first with `terraform apply -target=google_artifact_registry_repository.images`
docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT_ID/marginal/document-service:latest \
  -f services/document-service/Dockerfile services/document-service
docker push us-central1-docker.pkg.dev/YOUR_PROJECT_ID/marginal/document-service:latest
# repeat for auth-service and collaboration-service
```

(No `Dockerfile` exists in any service directory yet — write one per
service before this step; out of this Terraform's scope. Once they exist
and step 5 below has connected Cloud Build, this manual step becomes
optional — push to `master` on a `services/<name>/**` path and the
matching trigger builds and pushes it instead.)

### 4. Plan and apply

```bash
terraform fmt -check
terraform plan -out=tfplan
terraform apply tfplan
```

This first `apply` also creates the Cloud Build GitHub connection in a
pending state (see step 5) — that's expected, not an error.

### 5. Connect Cloud Build to GitHub (one-time, manual)

`terraform apply` alone does not finish wiring up Cloud Build — the
`google_cloudbuildv2_connection` it just created needs a human to authorize
it once:

```bash
terraform show -json | jq -r '
  .values.root_module.child_modules[]
  | select(.address=="module.cloud_build")
  | .resources[]
  | select(.address|endswith("google_cloudbuildv2_connection.github"))
  | .values.installation_state.action_uri'
```

(Or open the GCP Console: Cloud Build → Repositories → the pending
connection → "Finish installation".) Open that URL in a browser, authorize
the GitHub App against the `genuinebnt/marginal` repo (or whatever
`github_owner`/`github_repo_name` were set to), then apply again so
`google_cloudbuildv2_repository` and the three triggers can actually be
created:

```bash
terraform apply
```

Until this step is done, `terraform apply` will keep failing on the
repository resource — that failure is the expected signal that the manual
authorization hasn't happened yet, not a bug in this module.

### 6. Deploy the frontend

```bash
cd web && npm run build
gcloud storage rsync --recursive --delete-unmatched-files \
  dist/ "gs://$(terraform -chdir=../deploy/terraform output -raw frontend_bucket_name)"
cd ..
terraform -chdir=deploy/terraform output -raw frontend_website_url
```

No CI step does this automatically yet (see "Frontend hosting" above) —
repeat this manually after every frontend change you want visible.

### 7. Verify

```bash
terraform output document_service_url
terraform output auth_service_url
terraform output collaboration_service_url

curl "$(terraform output -raw auth_service_url)/.well-known/jwks.json"
curl -I "$(terraform output -raw frontend_website_url)"
```

### 8. Tear down (Tier S discipline — `ADR-010`)

```bash
terraform destroy
```

`deletion_protection = false` on every Cloud SQL instance by default
specifically so this works without a manual console step first. The
frontend bucket has `force_destroy = true` for the same reason — otherwise
a non-empty bucket blocks `terraform destroy` until its objects are deleted
by hand.
