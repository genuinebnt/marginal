# Cloud Build CI: replaces the README's manual `docker build && docker push`
# steps (Setup §3) with one trigger per Track 1 service, each running the
# templated ../../cloudbuild/cloudbuild.yaml against its own
# services/<name>/Dockerfile and pushing into the Artifact Registry repo the
# root module already provisions. Cloud Build is Tier R (ADR-010's Decision
# table: "free tier or scale-to-zero") — 120 build-minutes/day and 2,500
# build-minutes/mo free (CLOUD_ROADMAP.md's free-tier table), billed per
# build-minute beyond that, nothing running or billing between builds.
#
# IMPORTANT — this does not finish wiring itself up non-interactively:
#
# 1. google_cloudbuildv2_connection below creates the *connection* resource
#    in PENDING_USER_OAUTH state. Linking it to an actual GitHub account
#    requires visiting a console OAuth URL by hand, ONCE — Google's GitHub
#    App installation flow, which Terraform cannot complete headlessly.
#    After the first `apply` that creates this resource, read
#    `terraform state show` (or the GCP Console's Cloud Build > Repositories
#    page) for the connection's installation_state.action_uri and open it
#    in a browser. google_cloudbuildv2_repository below will fail to apply
#    until that manual step is done — see README Setup.
# 2. Every trigger's build step references services/<name>/Dockerfile,
#    which does not exist in this repo yet (already a known gap — README
#    "Setup" §3, "no Dockerfile exists yet"). These triggers will fail every
#    build until those Dockerfiles land. Writing them is out of this
#    Terraform task's scope — someone else is adding them separately.

data "google_project" "this" {
  project_id = var.project_id
}

resource "google_cloudbuildv2_connection" "github" {
  project  = var.project_id
  location = var.region
  name     = "${var.name_prefix}-github"

  # Empty on purpose — see header comment #1. Google fills in
  # installation_state once the manual console OAuth step completes; this
  # module never sets app_installation_id itself, which would imply reusing
  # a pre-existing installation this project doesn't have.
  github_config {}
}

resource "google_cloudbuildv2_repository" "this" {
  project           = var.project_id
  location          = var.region
  name              = var.github_repo_name
  parent_connection = google_cloudbuildv2_connection.github.name
  remote_uri        = "https://github.com/${var.github_owner}/${var.github_repo_name}.git"
}

# Cloud Build's default service account (PROJECT_NUMBER@cloudbuild.
# gserviceaccount.com) needs write access to push into the Artifact
# Registry repo the root module provisions. Scoped to just that repo, not
# project-wide — matching modules/cloud-run-service/main.tf's per-secret
# least-privilege grant pattern for the same reason.
resource "google_artifact_registry_repository_iam_member" "cloudbuild_writer" {
  project    = var.project_id
  location   = var.region
  repository = var.artifact_registry_repository_id
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${data.google_project.this.number}@cloudbuild.gserviceaccount.com"
}

resource "google_cloudbuild_trigger" "service" {
  for_each = toset(var.service_names)
  project  = var.project_id
  location = var.region
  name     = "${var.name_prefix}-${each.value}-build"

  repository_event_config {
    repository = google_cloudbuildv2_repository.this.id
    push {
      branch = var.branch_pattern
    }
  }

  # Only rebuild a service when its own directory changes — three services
  # sharing one push trigger apiece would otherwise all rebuild on every
  # commit, burning free build-minutes on services nothing touched.
  included_files = ["services/${each.value}/**"]

  filename = var.cloudbuild_config_path

  substitutions = {
    _SERVICE_NAME = each.value
    _REGION       = var.region
    _REPOSITORY   = var.artifact_registry_repository_id
  }
}
