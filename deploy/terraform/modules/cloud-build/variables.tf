variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "artifact_registry_repository_id" {
  type        = string
  description = "The root module's google_artifact_registry_repository.images.repository_id — pass the resource attribute, not a literal, so Terraform orders the IAM grant after the repo exists."
}

variable "github_owner" {
  type        = string
  description = "GitHub org/user owning the watched repo. Defaults to this repo's actual origin as of writing (git@github.com:genuinebnt/marginal.git)."
  default     = "genuinebnt"
}

variable "github_repo_name" {
  type    = string
  default = "marginal"
}

variable "branch_pattern" {
  type        = string
  default     = "^master$"
  description = "Regex Cloud Build matches against the pushed branch name. Defaults to this repo's actual current default branch (master, not main) — update if that ever changes."
}

variable "cloudbuild_config_path" {
  type        = string
  description = "Path, relative to the repo root, to the cloudbuild.yaml Cloud Build reads at build time. Lives under deploy/terraform/cloudbuild/ rather than the repo root so this piece's footprint stays inside deploy/terraform/ — see README."
  default     = "deploy/terraform/cloudbuild/cloudbuild.yaml"
}

variable "service_names" {
  type        = list(string)
  default     = ["document-service", "auth-service", "collaboration-service"]
  description = "One trigger per service; each references the same templated cloudbuild.yaml with a different _SERVICE_NAME substitution and its own included_files path filter."
}
