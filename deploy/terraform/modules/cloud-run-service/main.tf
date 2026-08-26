# One Cloud Run v2 service, its own least-privilege service account, and
# (optionally) public invoker access. Instantiated once per Track 1
# service from the root module.
#
# min_instance_count = 0 always — CLOUD_ROADMAP.md §1 / ADR-010 §1: "every
# service, min = 0 ... a single Cloud Run instance with min-instances = 0
# is not wrong — it is idle" until a second concurrent editor gives
# collaboration-service's stateful justification actual load.

resource "google_service_account" "run_sa" {
  project      = var.project_id
  account_id   = "${var.service_name}-run"
  display_name = "Cloud Run runtime identity for ${var.service_name}"
}

# Least-privilege, per secret: DATA_MODEL.md's "the grant is the boundary"
# rule (there for cross-schema Postgres access) applied at the Secret
# Manager layer — a service can only read the secrets it was explicitly
# handed, never another service's DATABASE_URL.
resource "google_secret_manager_secret_iam_member" "accessor" {
  for_each  = var.secret_env
  project   = var.project_id
  secret_id = each.value.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.run_sa.email}"
}

resource "google_project_iam_member" "extra_roles" {
  for_each = toset(var.extra_project_roles)
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.run_sa.email}"
}

resource "google_cloud_run_v2_service" "this" {
  project  = var.project_id
  name     = var.service_name
  location = var.region
  # No LB/gateway exists yet (api-gateway is out of scope for this repo —
  # CLAUDE.md's service table, ADR-011): each service's Cloud Run URL is
  # the only ingress, so it must accept traffic from the internet.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.run_sa.email

    scaling {
      min_instance_count = 0
      max_instance_count = var.max_instance_count
    }

    vpc_access {
      connector = var.vpc_connector_id
      # Only RFC1918-bound traffic (Cloud SQL, the Redis VM) goes through
      # the connector; everything else egresses directly. Cheaper and
      # keeps the connector's own scaling out of the request's critical
      # path for calls that don't need it.
      egress = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = var.image

      ports {
        # The single port Cloud Run forwards traffic to. See
        # README "Known limitations" — for auth-service this is the
        # HTTP listener (JWKS + health), not the gRPC one, because Cloud
        # Run v2 forwards exactly one container port and the app opens
        # two separate listeners (services/auth-service/cmd/main.go).
        container_port = var.container_port
      }

      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
      }

      dynamic "env" {
        for_each = var.env
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = var.secret_env
        content {
          name = env.value.env_name
          value_source {
            secret_key_ref {
              secret  = env.value.secret_id
              version = "latest"
            }
          }
        }
      }

      # Readiness must check the dependency, not just process liveness
      # (CLOUD_PORTABILITY.md §5) — these hit each service's /health,
      # which itself is probe-only today (no dependency check wired into
      # any of the three main.go files yet; see README "Known
      # limitations"). Probing here is still correct: it is the contract
      # Cloud Run expects, and the check deepens without a Terraform
      # change once the Go handlers do.
      startup_probe {
        http_get {
          path = var.health_path
          port = var.probe_port
        }
        initial_delay_seconds = 0
        period_seconds         = 5
        failure_threshold      = 6
      }

      liveness_probe {
        http_get {
          path = var.health_path
          port = var.probe_port
        }
        period_seconds = 15
      }
    }
  }

  # Traffic split is meaningless with one revision and no canarying set up
  # yet; keep it simple — all traffic to latest.
  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }
}

resource "google_cloud_run_v2_service_iam_member" "public" {
  count    = var.allow_unauthenticated ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.this.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
