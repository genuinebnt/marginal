# Remote state in a GCS bucket with versioning — ADR-008: "Terraform state
# lives in a Cloud Storage bucket with versioning enabled ... configure
# the backend before creating anything else, or the first real resource is
# stranded in local state."
#
# Left empty deliberately rather than hardcoding a bucket name: this file
# is committed, and a bucket name is per-user/per-project. Create the
# bucket first (README "Setup"), then run:
#
#   terraform init \
#     -backend-config="bucket=YOUR_STATE_BUCKET" \
#     -backend-config="prefix=track1"
#
# `terraform init -reconfigure` re-runs this if the backend config changes
# later (e.g. moving to a different bucket).
terraform {
  backend "gcs" {}
}
