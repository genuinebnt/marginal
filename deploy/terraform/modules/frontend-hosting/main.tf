# GCS bucket configured for static website hosting — serves web/ (React 19 +
# TS + Vite SPA, `npm run build` outputs to web/dist/ per web/package.json)
# directly to browsers. No app server, no container, no Cloud Run service:
# it's a static bundle (ADR-004's no-SSR decision, referenced by
# CLOUD_PORTABILITY.md's "Static client" row).
#
# Picked over a global external HTTPS Load Balancer + Cloud CDN backend
# bucket (the other option this repo's docs considered for this exact slot):
# ADR-010's own Decision table puts "Cloud Storage" in **Tier R** (resident,
# "free tier or scale-to-zero") and "Cloud Load Balancing"/"Cloud CDN" in
# **Tier S** (rented by the hour, never scales to zero — CLOUD_ROADMAP.md's
# Tier R/S tables agree). A static-website bucket costs nothing while idle,
# billed only for bytes stored and served; an LB+CDN backend bills an hourly
# forwarding-rule charge from the moment it exists, whether or not anyone
# visits. That's the same reasoning ADR-008/CLOUD_PORTABILITY.md already
# used to prefer **Firebase Hosting** over Cloud CDN+LB for this exact slot
# ("Cloud CDN needs a load balancer, which is never free" — ADR-008
# "Decision" table, Static client row; CLOUD_PORTABILITY.md's Static client
# row names Firebase Hosting as the actual cloud target).
#
# This module does not provision Firebase Hosting itself: it needs the
# Firebase Management API enabled and a Firebase-enabled project (a
# onboarding step separate from plain GCP project creation), and its
# Terraform resources (google_firebase_hosting_site etc.) live behind the
# google-beta provider, which nothing else in this estate needs. A plain GCS
# static-website bucket gets the identical zero-idle-cost property using
# only the `google` provider already configured here, and is a same-shape
# swap for Firebase Hosting later if that onboarding happens — see README
# "Judgment calls."

resource "google_storage_bucket" "frontend" {
  project                     = var.project_id
  name                        = var.bucket_name
  location                    = var.location
  uniform_bucket_level_access = true
  # Rebuildable from web/dist/ on every deploy (see README "Setup" for the
  # gcloud storage rsync command) — nothing here is data worth protecting on
  # destroy, matching ADR-010's "terraform destroy becomes load-bearing"
  # framing for the rest of this estate.
  force_destroy = true

  website {
    main_page_suffix = "index.html"
    # SPA fallback. No client-side router exists in web/ yet (web/src/App.tsx
    # is still the Vite scaffold as of this writing) so nothing depends on
    # this today, but a deep link into a client-routed page needs index.html
    # back, not a bucket 404, the day one lands — set now so this doesn't
    # need revisiting then.
    not_found_page = "index.html"
  }
}

# Static website hosting on GCS requires the objects themselves to be
# world-readable — there is no LB or CDN in front of this bucket to gate
# access (see main.tf header). This is the GCS equivalent of the three
# Cloud Run services' allow_unauthenticated = true (root variables.tf): same
# reasoning, nothing else fronts it yet.
resource "google_storage_bucket_iam_member" "public_read" {
  bucket = google_storage_bucket.frontend.name
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}
