-- +goose Up

-- v2.6.0's page-delete saga (ARCHITECTURE.md §5 § What this actually is at
-- this repo's scope, DATA_MODEL.md § Page deletions).
--
-- docs.pages.lifecycle_state already says WHAT a page is; it cannot say how
-- far a delete got. That progress belongs to the operation rather than the
-- page: it has its own retry count, and once the page is purged the row is
-- history, not state. Keeping it off docs.pages also keeps the table the
-- editor blocks on from widening for state almost every row has no use for.
CREATE TABLE docs.page_deletions (
    page_id      UUID PRIMARY KEY REFERENCES docs.pages(id) ON DELETE CASCADE,
    requested_by UUID NOT NULL,          -- auth.users(id), no FK: cross-schema

    -- Steps completed so far, in completion order. A step appends its own
    -- name exactly once; the sweeper resumes at the first name NOT present.
    -- An array rather than a column per step so that adding a step
    -- (embeddings, blobs — v4) is not a migration on a hot table: the set of
    -- steps is a property of the code's version, not the schema's.
    steps_done   TEXT[] NOT NULL DEFAULT '{}',

    -- Bumped on every resume, so ui-mockups § 23c's "resumed once" is a
    -- recorded fact rather than an inference from timestamps.
    attempts     INT NOT NULL DEFAULT 1,

    -- Set when the last step lands. Until then the page is 'deleting' and
    -- the sweeper owns it; after, it is 'deleted' and restorable.
    completed_at TIMESTAMPTZ,

    -- Forward-only compensation: a step that keeps failing is retried, never
    -- rolled back, so this exists to make a stuck saga diagnosable rather
    -- than merely slow.
    last_error   TEXT,

    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The sweeper's claim query: every in-flight saga, oldest first. Partial on
-- completed_at IS NULL because a finished saga is never claimed again, and
-- the finished rows will outnumber the live ones almost immediately.
CREATE INDEX docs_page_deletions_inflight_idx
    ON docs.page_deletions (started_at)
    WHERE completed_at IS NULL;

-- document-service's first outbox. It has only ever consumed events
-- (collab.ops_flushed, via internal/blockproj) — the saga is the first
-- thing it needs to PUBLISH, and docs.page_deleted must go out in the same
-- transaction that marks the page 'deleting', or a crash between the two
-- leaves a page nothing will ever come to release.
--
-- Column-for-column identical to collab.outbox (00001_collab_ops.sql) on
-- purpose: marginal/outboxpoll's Poller is shared by all three publishers
-- now, and it plugs in per-service sqlc queries rather than per-service
-- shapes. A fourth column here would be a fork of that contract.
CREATE TABLE docs.outbox (
    -- App-generated (uuid.NewV7), like every other id in this repo; a
    -- Postgres-side DEFAULT would tie this schema to PG18's native
    -- uuidv7(), which Cloud SQL doesn't offer (deploy/terraform).
    id            UUID PRIMARY KEY,
    aggregate_id  UUID NOT NULL, -- page_id
    event_type    TEXT NOT NULL,
    payload       JSONB NOT NULL DEFAULT '{}',
    published_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON docs.outbox (created_at) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE docs.outbox;
DROP INDEX docs.docs_page_deletions_inflight_idx;
DROP TABLE docs.page_deletions;
