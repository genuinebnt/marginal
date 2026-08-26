# VPC for the Track 1 estate: one subnet, one Serverless VPC Access
# connector, and Private Services Access so Cloud SQL can get a private
# IP. CLOUD_ROADMAP.md § Step 2 calls for "Cloud SQL and Memorystore on
# private IPs, reachable only from the cluster's ranges" — written for a
# GKE cluster, but Cloud Run's equivalent of "the cluster's ranges" is a
# VPC connector, which is what this module builds.

resource "google_compute_network" "this" {
  project                 = var.project_id
  name                    = var.network_name
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "this" {
  project       = var.project_id
  name          = "${var.network_name}-subnet"
  region        = var.region
  network       = google_compute_network.this.id
  ip_cidr_range = var.subnet_cidr
}

# Serverless VPC Access connector — how Cloud Run reaches the private-IP
# Cloud SQL instances and the Redis VM. Cloud Run v2 also supports direct
# VPC egress without a connector; the connector is used here because it is
# the more broadly documented path and keeps this module portable to
# Cloud Run v1 workloads too, per CLOUD_PORTABILITY.md's "same trait,
# different implementation" framing — swap this for direct VPC egress on
# the `google_cloud_run_v2_service` resource if the connector's own
# minimum-instance cost becomes worth avoiding.
resource "google_vpc_access_connector" "this" {
  project       = var.project_id
  name          = "${var.network_name}-conn"
  region        = var.region
  network       = google_compute_network.this.name
  ip_cidr_range = var.connector_cidr
  # Smallest connector footprint the API allows — this estate has no
  # sustained east-west traffic yet (no service calls another service
  # over gRPC in Track 1's current code), so scale stays at the floor
  # (ADR-010: don't provision beyond what's demonstrably needed).
  min_instances = 2
  max_instances = 3
  machine_type  = "e2-micro"
}

# Private Services Access: reserves a range for Google-managed services
# (Cloud SQL's private IP lives here) and peers it into this VPC. Required
# before any google_sql_database_instance can set private_network.
resource "google_compute_global_address" "private_services_range" {
  project       = var.project_id
  name          = "${var.network_name}-psa-range"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.this.id
}

resource "google_service_networking_connection" "private_services" {
  network                 = google_compute_network.this.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_services_range.name]
}

# Lets the operator reach the Redis VM for inspection without a public IP
# on the instance itself — IAP's fixed range is the standard way in.
resource "google_compute_firewall" "allow_iap_ssh" {
  project       = var.project_id
  name          = "${var.network_name}-allow-iap-ssh"
  network       = google_compute_network.this.name
  direction     = "INGRESS"
  source_ranges = ["35.235.240.0/20"] # Google's fixed IAP TCP forwarding range
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
  target_tags = ["iap-ssh"]
}

# Lets the VPC connector's range reach the Redis VM. Cloud SQL's private
# IP is reachable via the service-networking peering above, which already
# scopes traffic to this VPC — no separate firewall rule needed for it.
resource "google_compute_firewall" "allow_connector_to_redis" {
  project       = var.project_id
  name          = "${var.network_name}-allow-connector-redis"
  network       = google_compute_network.this.name
  direction     = "INGRESS"
  source_ranges = [var.connector_cidr]
  allow {
    protocol = "tcp"
    ports    = ["6379"]
  }
  target_tags = ["redis-vm"]
}
