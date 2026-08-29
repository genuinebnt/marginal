-- +goose Up

-- collab.ops had no ordering column, and replay read it back
-- `ORDER BY created_at`.
--
-- `created_at` defaults to `now()`, which in Postgres is the TRANSACTION
-- start time — and internal/flush writes a whole drain batch in one
-- transaction (RFC-002 §7's batching). So every op in a batch shares one
-- timestamp, and replay ordered them arbitrarily.
--
-- That is not a cosmetic bug. It broke I0.2 — replay must reproduce the
-- projection — in the most direct way possible: a container's child could be
-- replayed before the container existed, and GET .../trace returned 500 with
-- "block not found". The Trace and History screens were dark on any page
-- whose seed batch happened to land the wrong way round.
--
-- DATA_MODEL.md has specified `seq bigint` since the schema was written; it
-- was simply never implemented. This is that column.
--
-- Global rather than per-page: gapless-per-page needs a counter per page,
-- which needs either a lock or a row per page on the write path that every
-- keystroke pays for. What ordering actually requires is MONOTONICITY, and a
-- sequence gives that without contention. A gap in a page's own numbers is
-- therefore expected here and is not a corruption signal — which is a
-- weakening of DATA_MODEL's wording, made deliberately and recorded rather
-- than quietly diverged from.
ALTER TABLE collab.ops ADD COLUMN seq BIGINT;

-- Backfill in the order the ops actually arrived. `id` is a server-generated
-- UUIDv7 assigned per op as it is accepted, so it is monotonic within a
-- process and breaks the created_at tie correctly — verified against the
-- corrupted rows this migration exists for, where it puts every container
-- back in front of its children.
-- +goose StatementBegin
WITH ordered AS (
  SELECT id, row_number() OVER (ORDER BY created_at, id) AS n FROM collab.ops
)
UPDATE collab.ops o SET seq = ordered.n FROM ordered WHERE o.id = ordered.id;
-- +goose StatementEnd

CREATE SEQUENCE collab.ops_seq_seq OWNED BY collab.ops.seq;
SELECT setval('collab.ops_seq_seq', COALESCE((SELECT MAX(seq) FROM collab.ops), 0) + 1, false);
ALTER TABLE collab.ops ALTER COLUMN seq SET DEFAULT nextval('collab.ops_seq_seq');
ALTER TABLE collab.ops ALTER COLUMN seq SET NOT NULL;

-- Replay reads one page in seq order; this is the index it reads through.
CREATE INDEX ops_page_id_seq_idx ON collab.ops (page_id, seq);

-- +goose Down
DROP INDEX IF EXISTS collab.ops_page_id_seq_idx;
ALTER TABLE collab.ops ALTER COLUMN seq DROP DEFAULT;
DROP SEQUENCE IF EXISTS collab.ops_seq_seq;
ALTER TABLE collab.ops DROP COLUMN IF EXISTS seq;
