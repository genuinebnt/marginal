-- name: InsertNotification :one
-- ON CONFLICT DO NOTHING on source_event_id makes this safe against NATS's
-- at-least-once redelivery — a redelivered auth.user_registered event
-- must not create a second welcome notification.
INSERT INTO notify.notifications (id, user_id, source_event_id, kind, message)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (source_event_id) DO NOTHING
RETURNING *;

-- name: ListNotificationsForUser :many
SELECT * FROM notify.notifications
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2;
