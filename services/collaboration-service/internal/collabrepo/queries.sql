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
SELECT * FROM collab.ops
WHERE page_id = $1
ORDER BY created_at ASC;

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

