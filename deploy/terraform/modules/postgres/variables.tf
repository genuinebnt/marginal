variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "service_name" {
  type        = string
  description = "Owning service, e.g. \"document-service\" — used to name the instance, database, user, and secret."
}

variable "database_name" {
  type        = string
  description = "Logical database name inside the instance. The service's migrate.Up call creates its own schema inside this database."
  default     = "app"
}

variable "database_user" {
  type        = string
  description = "Login role the service connects as. This instance has no separate migrate-vs-runtime role split (DATA_MODEL.md's reason for that split is the shared Tier R instance; with one instance per service the network already isolates)."
  default     = "app"
}

variable "network_id" {
  type        = string
  description = "VPC self_link/id to attach the private IP to."
}

variable "private_vpc_connection" {
  description = "The network module's google_service_networking_connection resource — depended on so the peering exists before this instance requests a private IP."
}

variable "postgres_version" {
  type        = string
  description = "Cloud SQL Postgres major version. Defaults conservatively below the version the app's migrations were written against (PostgreSQL 18, for the uuidv7() builtin) — see README's uuidv7() note before changing this."
  default     = "POSTGRES_17"
}

variable "tier" {
  type    = string
  default = "db-f1-micro"
}

variable "availability_type" {
  type    = string
  default = "ZONAL"
}

variable "disk_size_gb" {
  type    = number
  default = 10
}

variable "disk_type" {
  type    = string
  default = "PD_HDD" # cheaper than SSD; this instance's write volume at demo scale doesn't need SSD IOPS
}

variable "backup_enabled" {
  type        = bool
  default     = false
  description = "Off by default — ADR-010 treats Cloud SQL as a rented Tier S resource, not the thing durability is staked on. Turn on if this instance is being used as a real resident database rather than a learning/demo session."
}

variable "deletion_protection" {
  type        = bool
  default     = false
  description = "Off by default so `terraform destroy` actually works — ADR-008/ADR-010 both make `terraform destroy` load-bearing, not hygienic. Turn on only for a genuinely long-lived instance."
}
