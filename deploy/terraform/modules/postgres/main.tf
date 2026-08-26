# One Cloud SQL instance per service — CLOUD_PORTABILITY.md §1's Tier S
# column is explicit: "Cloud SQL for PostgreSQL, one instance per service
# (ADR-003)". This module is instantiated once per Track 1 service from
# the root module; it never provisions the shared, schema-per-service
# instance ADR-010 §3 describes for Tier R, because that instance is meant
# to be an *external* serverless Postgres (Neon/Supabase) that sits
# outside GCP entirely (ADR-010 "Consequences" / CLOUD_ROADMAP.md §
# "The two gaps") — this module has nothing to provision for that path.
# See deploy/terraform/README.md "Postgres: which tier this actually is"
# for the full tradeoff and how to switch to the external-DB path.
#
# Database per service is already true trivially here: with one instance
# per service, the network is the isolation boundary (DATA_MODEL.md "With
# one instance per service the network enforces the boundary... this
# would be defence in depth rather than the primary mechanism"), so this
# module does not attempt the schema+role dance DATA_MODEL.md describes
# for the shared Tier R instance — one login role owns its one database.

resource "random_password" "app_user" {
  length  = 32
  special = false # keep the DSN free of URL-unsafe characters
}

resource "google_sql_database_instance" "this" {
  project             = var.project_id
  name                = "${var.service_name}-pg"
  region              = var.region
  database_version    = var.postgres_version
  deletion_protection = var.deletion_protection

  depends_on = [var.private_vpc_connection]

  settings {
    # ADR-010's own cost table cites ~$0.02/hr "at db-f1-micro" for Cloud
    # SQL. Verify this tier is still offered for Postgres at apply time —
    # Cloud SQL's shared-core tier list has changed before, and this repo
    # has no live credentials to check against. If db-f1-micro is
    # rejected, the smallest guaranteed-valid Postgres tier is a custom
    # machine type such as db-custom-1-3840.
    tier              = var.tier
    availability_type = var.availability_type # ZONAL — no HA replica; ADR-010 says don't over-provision Tier S/rented infra
    disk_size         = var.disk_size_gb
    disk_type         = var.disk_type
    disk_autoresize   = false # a runaway autoresize is exactly the kind of surprise bill ADR-008's budget alert exists to catch, but capping it here stops it before the alert would even fire

    ip_configuration {
      ipv4_enabled    = false # private IP only — CLOUD_ROADMAP.md § Step 2: "Cloud SQL ... on private IPs"
      private_network = var.network_id
    }

    backup_configuration {
      enabled = var.backup_enabled # off by default: this instance is Tier S in the docs' own terms — rented for a session and destroyed, not relied on for durability (ADR-010 §3)
    }
  }
}

resource "google_sql_database" "app" {
  project  = var.project_id
  name     = var.database_name
  instance = google_sql_database_instance.this.name
}

resource "google_sql_user" "app" {
  project  = var.project_id
  name     = var.database_user
  instance = google_sql_database_instance.this.name
  password = random_password.app_user.result
}

# The service's connection string, as a single secret — these Go services
# take one DATABASE_URL env var (see services/*/cmd/main.go), not split
# host/password fields, so the whole DSN needs Secret Manager treatment
# per CLOUD_PORTABILITY.md §3 ("Never in config.yaml ... Cloud: Secret
# Manager"), not just the password component.
resource "google_secret_manager_secret" "database_url" {
  project   = var.project_id
  secret_id = "${var.service_name}-database-url"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "database_url" {
  secret = google_secret_manager_secret.database_url.id
  # sslmode=require, not verify-full: this connects over the private VPC
  # path, not the public internet, and Cloud SQL's private IP does not
  # ship a certificate matching a public CA chain the client can verify
  # against without also provisioning the Cloud SQL Auth Proxy machinery
  # this module deliberately doesn't use (see README "Networking").
  secret_data = "postgres://${google_sql_user.app.name}:${random_password.app_user.result}@${google_sql_database_instance.this.private_ip_address}:5432/${google_sql_database.app.name}?sslmode=require"
}
