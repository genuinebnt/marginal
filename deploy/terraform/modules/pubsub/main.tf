# One Pub/Sub topic + pull subscription per outbox event type that actually
# exists in the Go code today (CLAUDE.md's stack table: "one EventBus
# interface, two adapters" — NatsBus locally, PubSubBus in the cloud;
# ADR-010 §2). This is infrastructure provisioned AHEAD of the Go-side
# PubSubBus adapter and a poller landing — see README "Known limitations"
# for the honest state of what actually publishes today (nothing does yet).
#
# Real outbox event type found by grepping the Go code (not invented):
#   - "collab.ops_flushed" — services/collaboration-service/internal/
#     opstore/opstore.go's OutboxEventOpAppended constant (line 29), written
#     to collab.outbox in the same transaction as every op append and batch
#     append (opstore.go InsertOutboxEvent/InsertOutboxEventBatch calls;
#     DATA_MODEL.md's "Outbox — one per publishing service" section).
#
# document-service has NO outbox table or migration yet — grepping
# services/document-service turns up only a forward-reference comment in
# cmd/main.go's header ("... and (later) their outbox"), no
# migrations/*.sql defines docs.outbox, and nothing writes to one. It
# contributes zero topics here. Add its event type(s) to outbox_event_types
# the day that migration and the Go write path actually land — do not
# pre-guess names for a table that doesn't exist.

resource "google_pubsub_topic" "outbox_events" {
  for_each = toset(var.outbox_event_types)
  project  = var.project_id
  name     = "${var.name_prefix}-${each.value}"
}

# Pull, not push — matching a poller model (CLAUDE.md's EventBus is read by
# a poller draining the outbox, not by a webhook target; DATA_MODEL.md's
# outbox comment: "a poller publishes it"). No consumer exists yet (see
# README), so this subscription would otherwise be a resource with no
# traffic ever flowing through it — the one real risk of that state is
# Pub/Sub's default 31-day inactivity expiration silently deleting the
# subscription before any Go code catches up to consume it.
# expiration_policy { ttl = "" } (never expire) turns that off explicitly
# rather than relying on — and being surprised by — the default.
resource "google_pubsub_subscription" "outbox_events_pull" {
  for_each = toset(var.outbox_event_types)
  project  = var.project_id
  name     = "${var.name_prefix}-${each.value}-sub"
  topic    = google_pubsub_topic.outbox_events[each.value].id

  ack_deadline_seconds = 30

  expiration_policy {
    ttl = "" # never expire — see header comment
  }
}
