-- +goose Up
CREATE SCHEMA IF NOT EXISTS docs;
CREATE EXTENSION IF NOT EXISTS ltree;

CREATE TABLE docs.pages (
    -- Generated application-side (uuid.Must(uuid.NewV7())) — repo.Create
    -- needs the id before the INSERT to compute path's own final LTREE
    -- label, and a Postgres-side DEFAULT would also tie this schema to
    -- PG18's native uuidv7(), which Cloud SQL doesn't offer (deploy/terraform).
    id              UUID PRIMARY KEY,
    created_by      UUID NOT NULL, -- auth.users(id); no FK, cross-schema (DATA_MODEL.md §1)
    title           TEXT NOT NULL,
    parent_id       UUID REFERENCES docs.pages(id),
    -- Materialised ancestry, e.g. 'root.a1b2.c3d4'. Subtree queries via <@
    -- without a recursive CTE on the hot path.
    path            LTREE NOT NULL,
    -- Fractional index (services/document-service/internal/sortkey): a
    -- page is reordered by writing ONE row, never by renumbering siblings.
    sort_key        TEXT NOT NULL,
    -- Saga state (ARCHITECTURE.md §5). A crash mid-delete resumes, not restarts.
    lifecycle_state TEXT NOT NULL DEFAULT 'active'
        CHECK (lifecycle_state IN ('active', 'deleting', 'deleted')),
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ON docs.pages USING GIST (path);
CREATE INDEX ON docs.pages (parent_id, sort_key) WHERE deleted_at IS NULL;
-- Title uniqueness is NOT enforced: duplicates are a diagnostic
-- (RFC-003 DuplicateTitle), not a constraint violation.
CREATE INDEX ON docs.pages (lower(title)) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS docs.pages;
DROP SCHEMA IF EXISTS docs;
