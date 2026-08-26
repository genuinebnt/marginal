# Root module: wires the three Track 1 services (CLAUDE.md's service
# table — document-service:8001, auth-service:8006, collaboration-
# service:8002) onto Cloud Run, per ADR-008/ADR-010's Tier R posture.
# See README.md for the full picture, cost posture, and known
# limitations — this file is deliberately unabstracted (three explicit
# module blocks per resource type, not a for_each map) because three is
# too few instances for the indirection to pay for itself yet (repo-wide
# "duplicate on the second use, extract on the third" rule).

locals {
  required_apis = [
    "run.googleapis.com",
    "sqladmin.googleapis.com",
    "secretmanager.googleapis.com",
    "vpcaccess.googleapis.com",
    "servicenetworking.googleapis.com",
    "compute.googleapis.com",
    "artifactregistry.googleapis.com",
    "billingbudgets.googleapis.com",
    "iam.googleapis.com",
    "monitoring.googleapis.com",
    "storage.googleapis.com",    # frontend-hosting module's GCS bucket
    "pubsub.googleapis.com",     # pubsub module's outbox topics/subscriptions
    "cloudbuild.googleapis.com", # cloud-build module's triggers + repo connection
  ]
}

resource "google_project_service" "apis" {
  for_each           = toset(local.required_apis)
  project            = var.project_id
  service            = each.value
  disable_on_destroy = false # disabling APIs on destroy has broken re-applies before; leaving them enabled costs nothing
}

resource "google_artifact_registry_repository" "images" {
  project       = var.project_id
  location      = var.region
  repository_id = var.artifact_registry_repository_id
  format        = "DOCKER"
  depends_on    = [google_project_service.apis]
}

module "network" {
  source         = "./modules/network"
  project_id     = var.project_id
  region         = var.region
  network_name   = "${var.name_prefix}-vpc"
  subnet_cidr    = var.subnet_cidr
  connector_cidr = var.connector_cidr
  depends_on     = [google_project_service.apis]
}

# --- Postgres: one Cloud SQL instance per service (CLOUD_PORTABILITY.md
# §1's Tier S column — see modules/postgres/main.tf's header comment for
# why this is Tier S in the docs' own terms, and README for how to switch
# to an external Tier R Postgres instead) ---

module "document_service_db" {
  source                  = "./modules/postgres"
  project_id              = var.project_id
  region                  = var.region
  service_name            = "document-service"
  network_id              = module.network.network_id
  private_vpc_connection  = module.network.private_vpc_connection
  postgres_version        = var.postgres_version
  tier                    = var.postgres_tier
  disk_size_gb            = var.postgres_disk_size_gb
  backup_enabled          = var.postgres_backup_enabled
}

module "auth_service_db" {
  source                  = "./modules/postgres"
  project_id              = var.project_id
  region                  = var.region
  service_name            = "auth-service"
  network_id              = module.network.network_id
  private_vpc_connection  = module.network.private_vpc_connection
  postgres_version        = var.postgres_version
  tier                    = var.postgres_tier
  disk_size_gb            = var.postgres_disk_size_gb
  backup_enabled          = var.postgres_backup_enabled
}

module "collaboration_service_db" {
  source                  = "./modules/postgres"
  project_id              = var.project_id
  region                  = var.region
  service_name            = "collaboration-service"
  network_id              = module.network.network_id
  private_vpc_connection  = module.network.private_vpc_connection
  postgres_version        = var.postgres_version
  tier                    = var.postgres_tier
  disk_size_gb            = var.postgres_disk_size_gb
  backup_enabled          = var.postgres_backup_enabled
}

# --- Redis: one shared e2-micro VM (Tier R answer — see
# modules/redis-vm/main.tf's header). auth-service uses it for the jti
# blocklist and lockout counters, collaboration-service for presence/
# lease/instance-registry state (DATA_MODEL.md's schema table).
# document-service has no Redis dependency and isn't wired to it. ---

module "redis" {
  source        = "./modules/redis-vm"
  project_id    = var.project_id
  zone          = var.zone
  name_prefix   = var.name_prefix
  network_id    = module.network.network_id
  subnetwork_id = module.network.subnetwork_id
}

# --- Cloud Run: one service each ---

module "document_service_run" {
  source                 = "./modules/cloud-run-service"
  project_id             = var.project_id
  region                 = var.region
  service_name           = "document-service"
  image                  = var.document_service_image
  vpc_connector_id       = module.network.vpc_connector_id
  allow_unauthenticated  = var.allow_unauthenticated
  # gRPC (PageService) is the actual traffic surface (ADR-007); HTTP :8001
  # is health-probe-only (CLAUDE.md's service table) and is used here only
  # as the probe target, never as ingress.
  container_port = 9001
  probe_port     = 8001
  env = {
    DOCUMENT_SERVICE_GRPC_ADDR = ":9001"
    DOCUMENT_SERVICE_HTTP_ADDR = ":8001"
  }
  secret_env = {
    database_url = {
      env_name  = "DATABASE_URL"
      secret_id = module.document_service_db.database_url_secret_id
    }
  }
}

module "auth_service_run" {
  source                = "./modules/cloud-run-service"
  project_id            = var.project_id
  region                = var.region
  service_name          = "auth-service"
  image                 = var.auth_service_image
  vpc_connector_id      = module.network.vpc_connector_id
  allow_unauthenticated = var.allow_unauthenticated
  # Ingress is the HTTP listener (:8006), not gRPC (:9006) — see README
  # "Known limitations": auth-service's JWKS route and health check need
  # to be publicly reachable and Cloud Run forwards only one container
  # port, so gRPC (AuthService) is NOT externally reachable through this
  # Cloud Run service as currently deployed.
  container_port = 8006
  probe_port     = 8006
  env = {
    AUTH_SERVICE_GRPC_ADDR = ":9006"
    AUTH_SERVICE_HTTP_ADDR = ":8006"
    REDIS_ADDR             = "${module.redis.internal_ip}:6379"
  }
  secret_env = {
    database_url = {
      env_name  = "DATABASE_URL"
      secret_id = module.auth_service_db.database_url_secret_id
    }
  }
}

module "collaboration_service_run" {
  source                = "./modules/cloud-run-service"
  project_id            = var.project_id
  region                = var.region
  service_name          = "collaboration-service"
  image                 = var.collaboration_service_image
  vpc_connector_id      = module.network.vpc_connector_id
  allow_unauthenticated = var.allow_unauthenticated
  # Only listener that exists today is HTTP :8002 (skeleton — see
  # services/collaboration-service/cmd/main.go); that is both ingress and
  # probe target for now. Revisit once a gRPC/WebSocket transport is
  # wired in — it may need its own container_port the same way
  # auth-service does.
  container_port = 8002
  probe_port     = 8002
  secret_env = {
    database_url = {
      env_name  = "DATABASE_URL"
      secret_id = module.collaboration_service_db.database_url_secret_id
    }
  }
  env = {
    REDIS_ADDR = "${module.redis.internal_ip}:6379"
  }
}

# --- Frontend hosting: GCS static-website bucket for web/dist/ (the React
# 19 + TS + Vite SPA). See modules/frontend-hosting/main.tf for why a bucket
# was picked over Cloud CDN + Load Balancer, and README "What this
# provisions" / cost table. ---

module "frontend_hosting" {
  source      = "./modules/frontend-hosting"
  project_id  = var.project_id
  bucket_name = coalesce(var.frontend_bucket_name, "${var.project_id}-web")
  location    = var.frontend_bucket_location
}

# --- Pub/Sub: one topic + pull subscription per real outbox event_type
# found in the Go code today (see modules/pubsub/main.tf header for the
# grep citations). Nothing publishes to these yet — see README "Known
# limitations." ---

module "pubsub" {
  source             = "./modules/pubsub"
  project_id         = var.project_id
  name_prefix        = var.name_prefix
  outbox_event_types = var.outbox_event_types
}

# --- Cloud Build: one trigger per service, replacing the README's manual
# docker build/push steps. Needs a one-time manual console step to finish
# the GitHub connection, and won't build successfully until each service's
# Dockerfile exists — see modules/cloud-build/main.tf header and README
# "Known limitations." ---

module "cloud_build" {
  source                          = "./modules/cloud-build"
  project_id                      = var.project_id
  region                          = var.region
  name_prefix                     = var.name_prefix
  artifact_registry_repository_id = google_artifact_registry_repository.images.repository_id
  github_owner                    = var.github_owner
  github_repo_name                = var.github_repo_name
}
