-- name: InsertOp :one
-- ON CONFLICT DO NOTHING makes this safe to retry: the flush loop that
-- drains the local WAL into Postgres (docs/porting/PROGRESS.md "Next") can
-- redeliver an op that was already committed here before a crash — RFC-002
-- §4 rule 5 names `id` as exactly the dedup key for this reason. A caller
-- gets back either the freshly-inserted row or, on a duplicate, no row at
-- all (sqlc.narg-free :one still returns pgx.ErrNoRows in that case) —
-- InsertOp in collabrepo.go turns that into "already flushed, not an
-- error."
INSERT INTO collab.ops (
    id, page_id, actor_id, actor_kind, undo_group,
    encoding_version, kind, payload, vector_clock
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: InsertOpBatch :batchone
-- Same statement as InsertOp, queued via pgx.Batch instead of one
-- round-trip each — RFC-002 §7's "batching is what makes the write volume
-- survivable": internal/flush's drain loop queues every pending op from
-- one flush interval into a single Batch, sent (and, for the matching
-- outbox events, committed) as one pipelined network round trip rather
-- than N.
INSERT INTO collab.ops (
    id, page_id, actor_id, actor_kind, undo_group,
    encoding_version, kind, payload, vector_clock
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: ListOpsForPage :many
-- ORDER BY seq, not created_at. `created_at` defaults to now(), which is the
-- TRANSACTION start time, and internal/flush writes a whole drain batch in
-- one transaction — so every op in a batch shared a timestamp and replay
-- ordered them arbitrarily. That broke I0.2 directly: a container's child
-- could replay before the container existed, and .../trace answered 500.
SELECT * FROM collab.ops
WHERE page_id = $1
ORDER BY seq ASC;

-- name: InsertOutboxEvent :one
-- id is generated application-side (uuid.Must(uuid.NewV7()) in opstore.go),
-- not a Postgres-side DEFAULT — see the migration's own comment on why.
INSERT INTO collab.outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: InsertOutboxEventBatch :batchone
-- Same statement as InsertOutboxEvent, batched for the same reason as
-- InsertOpBatch.
INSERT INTO collab.outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ClaimUnpublishedOutboxEvents :many
-- FOR UPDATE SKIP LOCKED (DATA_MODEL.md § Outbox): a second poller
-- instance skips rows the first already claimed instead of blocking on
-- them, so at-least-once delivery holds even with more than one replica
-- of this service running the poller loop — same pattern as auth-service's
-- own identically-named query.
SELECT * FROM collab.outbox
WHERE published_at IS NULL
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxEventsPublished :exec
UPDATE collab.outbox SET published_at = NOW()
WHERE id = ANY(sqlc.arg(ids)::uuid[]);


-- name: OutboxDepth :one
-- § 16 PERF's QUEUE DEPTH, measured rather than drawn: how many
-- events are waiting, and how long the oldest one has waited.
--
-- Two numbers rather than one because they answer different
-- questions: a depth of 400 that drains in 200 ms is a healthy
-- burst, and a depth of 3 whose oldest row is four minutes old is a
-- poller that has stopped. Reporting only the count would call the
-- second one fine.
SELECT
    COUNT(*) AS depth,
    COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)::float8 AS oldest_seconds
FROM collab.outbox
WHERE published_at IS NULL;

-- name: OpLogStats :one
-- Op-log size and how far behind the newest op is. `lag_seconds`
-- is time since the last accepted op, which on an idle instance is
-- large and healthy — the screen says so rather than colouring it.
SELECT
    COUNT(*) AS ops,
    COALESCE(EXTRACT(EPOCH FROM (NOW() - MAX(created_at))), 0)::float8 AS lag_seconds,
    COUNT(DISTINCT page_id) AS pages
FROM collab.ops;

-- name: OpsPerHour :many
-- § 18 ADMIN's sparkline: accepted ops per hour for the last
-- 14 hours, oldest first, with empty hours present as zero.
--
-- generate_series rather than GROUP BY alone, because a quiet
-- hour has no rows and a sparkline that silently omits it draws
-- a busy day where there was a gap. The zero is the point.
WITH hours AS (
    SELECT generate_series(
        date_trunc('hour', NOW()) - INTERVAL '13 hours',
        date_trunc('hour', NOW()),
        INTERVAL '1 hour'
    ) AS hour
)
SELECT h.hour::timestamptz AS hour, COUNT(o.id) AS ops
FROM hours h
LEFT JOIN collab.ops o
  ON date_trunc('hour', o.created_at) = h.hour
GROUP BY h.hour
ORDER BY h.hour ASC;

-- name: DatabaseSize :one
-- What this service's own database costs on disk. Per service,
-- because the architecture is database-per-service — a single
-- "DB SIZE" for the instance would be a number no one owns.
SELECT pg_database_size(current_database())::bigint AS bytes;

-- name: AuditOps :many
-- § 18b AUDIT LOG's content rows, read straight out of the op
-- log rather than written beside it.
--
-- This is the whole claim the screen makes: there is no code
-- path that edits a page without producing the row that says so,
-- because the row IS the op. A second, separately-written audit
-- table could drift from what actually happened; a projection
-- cannot.
--
-- The payload is deliberately NOT selected. An audit row says
-- who did what to which page; the text they typed is the
-- document's business, and an admin surface that quietly
-- includes it is a different, more invasive feature than the one
-- anybody asked for.
SELECT id, page_id, actor_id, actor_kind, kind, undo_group, seq, created_at
FROM collab.ops
WHERE (sqlc.narg(kinds)::text[] IS NULL OR kind = ANY(sqlc.narg(kinds)::text[]))
ORDER BY seq DESC
LIMIT sqlc.arg(row_limit);

-- name: AuditCounts :many
-- Every op kind and how many there are, for § 18b's EVENTS BY
-- CLASS panel. Classification into content/destructive happens
-- in Go, over this — the database should not know what the
-- product considers destructive.
SELECT kind, COUNT(*) AS n
FROM collab.ops
GROUP BY kind
ORDER BY n DESC;
