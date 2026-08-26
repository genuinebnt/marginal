output "document_service_url" {
  value = module.document_service_run.url
}

output "auth_service_url" {
  value = module.auth_service_run.url
}

output "collaboration_service_url" {
  value = module.collaboration_service_run.url
}

output "artifact_registry_repository" {
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${var.artifact_registry_repository_id}"
  description = "Push images here before the first apply, e.g. ${var.region}-docker.pkg.dev/${var.project_id}/${var.artifact_registry_repository_id}/document-service:TAG"
}

output "redis_internal_ip" {
  value = module.redis.internal_ip
}

output "document_service_db_connection_name" {
  value = module.document_service_db.connection_name
}

output "auth_service_db_connection_name" {
  value = module.auth_service_db.connection_name
}

output "collaboration_service_db_connection_name" {
  value = module.collaboration_service_db.connection_name
}

output "document_service_run_sa" {
  value = module.document_service_run.service_account_email
}

output "auth_service_run_sa" {
  value = module.auth_service_run.service_account_email
}

output "collaboration_service_run_sa" {
  value = module.collaboration_service_run.service_account_email
}

output "frontend_bucket_name" {
  value = module.frontend_hosting.bucket_name
}

output "frontend_website_url" {
  value = module.frontend_hosting.website_url
}

output "pubsub_topic_ids" {
  value = module.pubsub.topic_ids
}

output "pubsub_subscription_ids" {
  value = module.pubsub.subscription_ids
}

output "cloudbuild_connection_id" {
  value       = module.cloud_build.connection_id
  description = "Read this, then check its installation_state.action_uri (terraform state show <this resource>, or the GCP Console) to find the one-time manual GitHub authorization link — see README Setup."
}

output "cloudbuild_trigger_ids" {
  value = module.cloud_build.trigger_ids
}
