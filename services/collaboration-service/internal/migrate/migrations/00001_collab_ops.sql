-- +goose Up
CREATE SCHEMA IF NOT EXISTS collab;

-- APPEND ONLY. No UPDATE. No DELETE. This is the source of truth
-- (DATA_MODEL.md § The op log).
CREATE TABLE collab.ops (
    id               UUID PRIMARY KEY,      -- UUIDv7, assigned application-side (internal/oplog.New) before insert — the dedup key
    -- NO foreign key: docs.pages belongs to document-service, this table
    -- belongs to collaboration-service (DATA_MODEL.md — database per
    -- service, no cross-schema joins).
    page_id          UUID NOT NULL,

    -- NO foreign key, deliberately: agent/plugin/system actors have no row
    -- in auth.users and never will (DATA_MODEL.md § Why actor_id has no
    -- foreign key) — do not "fix" this in review.
    actor_id         UUID NOT NULL,
    actor_kind       TEXT NOT NULL DEFAULT 'user'
                     CHECK (actor_kind IN ('user','agent','plugin','system')),

    -- One user gesture, one group. NULL means a group of one.
    undo_group       UUID,

    -- Encoding version, present from op #1 (RFC-002 §4) — history replay
    -- must decode ops written by every prior release, forever.
    encoding_version SMALLINT NOT NULL,
    kind             TEXT NOT NULL,         -- 'InsertText', 'DeleteText', ...
    payload          JSONB NOT NULL,        -- the op itself, incl. inversion data (deleted text)
    vector_clock     JSONB NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON collab.ops (page_id, created_at);
CREATE INDEX ON collab.ops (page_id, actor_id, created_at); -- per-user undo

-- One outbox per publishing service (DATA_MODEL.md § Outbox) — this is
-- collaboration-service's. document-service will eventually consume
-- collab.ops_flushed events from it to materialise docs.blocks; nothing
-- reads this table yet (no NATS wiring in this repo's scope this entry),
-- but the row is written in the SAME transaction as the op insert, so
-- publishing has something durable to catch up on once a consumer exists.
CREATE TABLE collab.outbox (
    -- Generated application-side (uuid.Must(uuid.NewV7()) in opstore.go) —
    -- every other id in this repo is app-generated too; a Postgres-side
    -- DEFAULT would also tie this schema to PG18's native uuidv7(), which
    -- Cloud SQL doesn't offer (deploy/terraform).
    id            UUID PRIMARY KEY,
    aggregate_id  UUID NOT NULL, -- page_id
    event_type    TEXT NOT NULL,
    payload       JSONB NOT NULL DEFAULT '{}',
    published_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON collab.outbox (created_at) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS collab.outbox;
DROP TABLE IF EXISTS collab.ops;
DROP SCHEMA IF EXISTS collab;
