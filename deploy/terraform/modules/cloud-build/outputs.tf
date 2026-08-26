output "connection_id" {
  value = google_cloudbuildv2_connection.github.id
}

output "trigger_ids" {
  value = { for k, t in google_cloudbuild_trigger.service : k => t.id }
}
