output "topic_ids" {
  value = { for k, t in google_pubsub_topic.outbox_events : k => t.id }
}

output "subscription_ids" {
  value = { for k, s in google_pubsub_subscription.outbox_events_pull : k => s.id }
}
