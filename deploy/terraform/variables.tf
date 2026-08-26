# --- Required, no defaults: per-user/per-project values ---

variable "project_id" {
  type        = string
  description = "GCP project to deploy into. Create it and link billing before the first apply."
}

variable "billing_account_id" {
  type        = string
  description = "Billing account ID (gcloud billing accounts list) — used only for the $10 budget alert (ADR-008 'Cost guardrails are part of the deliverable')."
}

variable "budget_notification_email" {
  type        = string
  description = "Email address for the budget alert's notification channel."
}

variable "document_service_image" {
  type        = string
  description = "Full Artifact Registry reference for document-service, e.g. REGION-docker.pkg.dev/PROJECT/marginal/document-service:TAG. No CI/build pipeline exists yet — build and push this yourself before apply."
}

variable "auth_service_image" {
  type        = string
  description = "Full Artifact Registry reference for auth-service."
}

variable "collaboration_service_image" {
  type        = string
  description = "Full Artifact Registry reference for collaboration-service."
}

# --- Defaults that follow directly from the docs ---

variable "region" {
  type        = string
  default     = "us-central1"
  description = "One of us-west1/us-central1/us-east1 keeps the Redis e2-micro VM inside the always-free tier (CLOUD_ROADMAP.md's Tier R table)."
}

variable "zone" {
  type        = string
  default     = "us-central1-a"
}

variable "name_prefix" {
  type    = string
  default = "marginal"
}

variable "artifact_registry_repository_id" {
  type    = string
  default = "marginal"
}

# --- Cost posture (ADR-010) — see modules/postgres/variables.tf and
# modules/redis-vm for the per-resource defaults these compose with ---

variable "budget_amount_usd" {
  type        = number
  default     = 10
  description = "ADR-008: '$10 budget alert' is the first Terraform resource that should exist."
}

variable "allow_unauthenticated" {
  type        = bool
  default     = true
  description = "Applies to all three services. True because no api-gateway exists in this repo's scope (ADR-011) — see README 'Known limitations'."
}

# --- Networking ---

variable "subnet_cidr" {
  type    = string
  default = "10.10.0.0/24"
}

variable "connector_cidr" {
  type    = string
  default = "10.10.1.0/28"
}

# --- Postgres, forwarded to the postgres module for all three services ---

variable "postgres_version" {
  type    = string
  default = "POSTGRES_17"
}

variable "postgres_tier" {
  type    = string
  default = "db-f1-micro"
}

variable "postgres_disk_size_gb" {
  type    = number
  default = 10
}

variable "postgres_backup_enabled" {
  type    = bool
  default = false
}

# --- Frontend hosting (modules/frontend-hosting) ---

variable "frontend_bucket_name" {
  type        = string
  default     = null
  description = "GCS bucket name for the static frontend (web/dist/). Defaults to \"${project_id}-web\" if unset — project IDs are already globally unique so this is too — but override if that collides, or if you plan to eventually point a custom domain at it (bucket name must then exactly match the domain)."
}

variable "frontend_bucket_location" {
  type        = string
  default     = "US"
  description = "See modules/frontend-hosting/variables.tf: a region or a multi-region."
}

# --- Pub/Sub (modules/pubsub) — real outbox event types only, see
# modules/pubsub/main.tf for the grep citations behind the default ---

variable "outbox_event_types" {
  type        = list(string)
  default     = ["collab.ops_flushed"]
  description = "One Pub/Sub topic + pull subscription per entry. Add a name here only once the corresponding Go outbox write path actually exists — do not pre-guess."
}

# --- Cloud Build (modules/cloud-build) ---

variable "github_owner" {
  type        = string
  default     = "genuinebnt"
  description = "GitHub org/user for the watched repo. Defaults to this repo's actual origin as of writing (git@github.com:genuinebnt/marginal.git)."
}

variable "github_repo_name" {
  type    = string
  default = "marginal"
}
