output "instance_name" {
  value = google_sql_database_instance.this.name
}

output "private_ip_address" {
  value = google_sql_database_instance.this.private_ip_address
}

output "connection_name" {
  value = google_sql_database_instance.this.connection_name
}

# Secret ID only — never the DSN itself. The Cloud Run module grants its
# service account accessor on exactly this secret and nothing else,
# mirroring DATA_MODEL.md's "the grant is the boundary" rule at the
# secrets layer instead of the schema layer.
output "database_url_secret_id" {
  value = google_secret_manager_secret.database_url.secret_id
}

output "database_url_secret_name" {
  value = google_secret_manager_secret.database_url.name
}
