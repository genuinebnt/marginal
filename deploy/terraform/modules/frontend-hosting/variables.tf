variable "project_id" {
  type = string
}

variable "bucket_name" {
  type        = string
  description = "Globally unique GCS bucket name (bucket names are unique across all of GCS, not just this project). A custom domain additionally requires the bucket to be named exactly after that domain (e.g. app.example.com) and a CNAME to c.storage.googleapis.com — not set up by this module, since no domain exists yet to point."
}

variable "location" {
  type        = string
  description = "GCS bucket location: a region (e.g. us-central1) or a multi-region (e.g. US). Multi-region costs slightly more per GB-month but serves from whichever Google edge is closest without a CDN in front — a reasonable trade for a bucket this small."
  default     = "US"
}
