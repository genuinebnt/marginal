output "bucket_name" {
  value = google_storage_bucket.frontend.name
}

output "website_url" {
  value       = "http://${google_storage_bucket.frontend.name}.storage.googleapis.com/"
  description = "GCS's documented static-website endpoint (also reachable over https:// — *.storage.googleapis.com carries a valid wildcard cert). Serves nothing until web/dist/ has been synced into the bucket — see README Setup."
}
