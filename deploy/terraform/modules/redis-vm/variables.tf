variable "project_id" {
  type = string
}

variable "zone" {
  type        = string
  description = "GCE zone, e.g. \"us-central1-a\". Must be in a region that offers the e2-micro always-free tier (us-west1, us-central1, us-east1 — CLOUD_ROADMAP.md's Tier R table) to actually be free."
}

variable "name_prefix" {
  type    = string
  default = "marginal"
}

variable "machine_type" {
  type    = string
  default = "e2-micro"
}

variable "network_id" {
  type = string
}

variable "subnetwork_id" {
  type = string
}
