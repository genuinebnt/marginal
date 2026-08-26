output "url" {
  value = google_cloud_run_v2_service.this.uri
}

output "service_account_email" {
  value = google_service_account.run_sa.email
}

output "name" {
  value = google_cloud_run_v2_service.this.name
}
