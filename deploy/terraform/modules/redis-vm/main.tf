# Redis on a single free-tier e2-micro VM, not Memorystore.
#
# CLOUD_ROADMAP.md's Tier R table names this explicitly: "Compute Engine
# e2-micro ... the escape hatch for anything stateful that must persist —
# NATS JetStream or Redis." Memorystore is Tier S ("provisioned and always
# billing" — ADR-010 §1/CLOUD_ROADMAP §2 "The two gaps" #2): it has no
# scale-to-zero, so it does not belong in a resident deployment at this
# project's current scale. auth-service's lockout/blocklist and
# collaboration-service's presence/lease/instance-registry state
# (DATA_MODEL.md's schema table) are the only Redis consumers, and neither
# needs Memorystore's HA/clustering — a single instance is what local
# compose already runs.
#
# The e2-micro free-tier limit is one instance per month in
# us-west1/us-central1/us-east1 (CLOUD_ROADMAP.md's Tier R table) — this
# module does not enforce that region restriction; the root module's
# default region satisfies it, and a comment there flags it if changed.

resource "google_compute_instance" "redis" {
  project      = var.project_id
  name         = "${var.name_prefix}-redis"
  zone         = var.zone
  machine_type = var.machine_type
  tags         = ["redis-vm", "iap-ssh"]

  boot_disk {
    initialize_params {
      # Container-Optimized OS: runs exactly one container via the
      # gce-container-declaration metadata key below, no OS/package
      # management needed for a single-purpose cache box.
      image = "projects/cos-cloud/global/images/family/cos-stable"
      size  = 10
      type  = "pd-standard"
    }
  }

  network_interface {
    network    = var.network_id
    subnetwork = var.subnetwork_id
    # No access_config block ⇒ no public IP. Reached only from the VPC
    # connector's range (network module's firewall rule) and, for
    # operator inspection, via IAP TCP forwarding (also that module).
  }

  metadata = {
    gce-container-declaration = yamlencode({
      spec = {
        containers = [{
          name  = "redis"
          image = "redis:7-alpine"
          # No --requirepass: the app's redis.NewClient call
          # (services/auth-service/cmd/main.go) sends no password today,
          # and network isolation (private IP + firewall) is the actual
          # boundary at this scale — matching DATA_MODEL.md's own
          # "the grant is the boundary" reasoning applied one layer down.
          # Add AUTH once this VM is reachable from anything broader than
          # this one VPC.
          stdin = false
          tty   = false
        }]
        restartPolicy = "Always"
      }
    })
    # No persistent disk is attached for Redis's own data directory —
    # a reboot loses the lockout/blocklist and presence state. Both are
    # self-healing caches, not correctness-critical durable state (the op
    # log in Postgres is what correctness depends on — DATA_MODEL.md §1),
    # so this is an accepted, deliberate gap at this scale rather than a
    # silent one.
  }

  # COS ships the Konlet container agent needed to read
  # gce-container-declaration; nothing else to install.
  allow_stopping_for_update = true
}
