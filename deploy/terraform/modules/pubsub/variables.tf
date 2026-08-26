variable "project_id" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "outbox_event_types" {
  type        = list(string)
  description = "Real outbox event_type values found in the Go code (see main.tf header for citations) — one topic + one pull subscription per entry. Do not add speculative names ahead of a real write path."
  default     = ["collab.ops_flushed"]
}
