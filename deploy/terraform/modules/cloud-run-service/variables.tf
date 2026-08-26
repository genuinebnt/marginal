variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "service_name" {
  type        = string
  description = "Cloud Run service name — matches the Go module's service name, e.g. \"document-service\"."
}

variable "image" {
  type        = string
  description = "Full Artifact Registry image reference including tag or digest. No CI/build pipeline exists yet (ADR-011 scope) — build and push this externally before apply."
}

variable "container_port" {
  type        = number
  description = "The one port Cloud Run forwards ingress traffic to."
}

variable "probe_port" {
  type        = number
  description = "Port the startup/liveness probes hit. Can differ from container_port — Cloud Run v2 probes the container directly, not through the public ingress path."
}

variable "health_path" {
  type    = string
  default = "/health"
}

variable "cpu" {
  type    = string
  default = "1"
}

variable "memory" {
  type    = string
  default = "512Mi"
}

variable "max_instance_count" {
  type        = number
  default     = 2
  description = "Kept low deliberately — ADR-010 is a cost-bounded posture, not a scale target, at this stage."
}

variable "vpc_connector_id" {
  type = string
}

variable "env" {
  type        = map(string)
  default     = {}
  description = "Plain (non-secret) environment variables."
}

variable "secret_env" {
  description = "Map keyed arbitrarily; each value names the env var to inject and the Secret Manager secret ID to source it from (always version \"latest\"). The service account is granted secretAccessor on exactly these secrets and no others."
  type = map(object({
    env_name  = string
    secret_id = string
  }))
  default = {}
}

variable "extra_project_roles" {
  type        = list(string)
  default     = []
  description = "Additional project-level IAM roles for this service's runtime SA, beyond the per-secret accessor grants this module already creates. Empty for all three Track 1 services today — none of them touch Cloud Storage, Pub/Sub, or KMS yet (see README)."
}

variable "allow_unauthenticated" {
  type        = bool
  default     = true
  description = "True because no api-gateway exists in this repo's scope (ADR-011) to front these services — each Cloud Run URL is the only ingress. Tighten to false and front with IAM/a gateway once one exists."
}
