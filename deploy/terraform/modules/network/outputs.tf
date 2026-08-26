output "network_id" {
  value = google_compute_network.this.id
}

output "network_name" {
  value = google_compute_network.this.name
}

output "subnetwork_id" {
  value = google_compute_subnetwork.this.id
}

output "vpc_connector_id" {
  value = google_vpc_access_connector.this.id
}

# Consumers (the postgres module) must depend_on this so private-IP Cloud
# SQL instances aren't created before the peering exists.
output "private_vpc_connection" {
  value = google_service_networking_connection.private_services
}
