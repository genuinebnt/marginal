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

-- name: MarkNotificationRead :execrows
-- Scoped by user_id as well as id: the id alone is a bearer token for
-- someone else's inbox row, and "already read" must stay idempotent rather
-- than move the timestamp forward on a second click.
UPDATE notify.notifications
SET read_at = NOW()
WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllNotificationsRead :execrows
-- "Clear the inbox" — the one bulk action § 20 offers. Returns how many rows
-- it actually cleared so the UI can say what happened instead of assuming.
UPDATE notify.notifications
SET read_at = NOW()
WHERE user_id = $1 AND read_at IS NULL;

-- name: CountUnreadForUser :one
-- The bell's badge. A COUNT rather than a length over ListForUser: the badge
-- is drawn on every screen and must not depend on a list limit.
SELECT COUNT(*) FROM notify.notifications
WHERE user_id = $1 AND read_at IS NULL;
