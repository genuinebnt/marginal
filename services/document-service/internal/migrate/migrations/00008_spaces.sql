-- +goose Up
-- ADR-013 §2. docs.pages gains its permission boundary, and this service
-- gains the local projection it filters reads against.

-- Added nullable, backfilled, then made NOT NULL. Adding a NOT NULL column
-- with no default to a table that already has rows fails; a DEFAULT would
-- silently give every FUTURE page the default space too, which is exactly
-- the mistake this column exists to prevent.
--
-- Plain statements rather than a DO block, deliberately: sqlc parses these
-- migrations to learn the schema and cannot see through PL/pgSQL. Written
-- procedurally, every query naming the new column fails to generate with
-- "column does not exist" — true of sqlc's model, false of the database.
ALTER TABLE docs.pages ADD COLUMN space_id UUID;

-- The default space's id is FIXED and shared with auth-service's own
-- migration. Two services must agree which space every pre-existing page
-- belongs to and cannot join across schemas to find out (DATA_MODEL.md §1);
-- deriving it from an event leaves pages spaceless while space_id is NOT
-- NULL, and looking it up means a network call inside a schema migration.
-- A constant is honest about being a coordination point.
UPDATE docs.pages SET space_id = '00000000-0000-7000-8000-00000000d0c5' WHERE space_id IS NULL;

ALTER TABLE docs.pages ALTER COLUMN space_id SET NOT NULL;

-- Every read filters by "the spaces you are in" before anything else, so
-- this is on the hot path of listing, searching and the graph.
CREATE INDEX pages_by_space ON docs.pages (space_id) WHERE deleted_at IS NULL;

-- A PROJECTION of auth.memberships, never a second source of truth
-- (DATA_MODEL.md § docs.space_members). It decides what you can SEE; auth
-- decides what you can DO.
CREATE TABLE docs.space_members (
    user_id     UUID NOT NULL,
    space_id    UUID NOT NULL,
    role        TEXT NOT NULL,
    -- The EVENT's timestamp, not NOW(). Two events for one user can arrive
    -- out of order — core NATS gives no ordering guarantee across publishes
    -- — and last-write-wins by arrival would let a revoke be undone by a
    -- grant that happened before it.
    granted_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, space_id)
);

CREATE INDEX space_members_by_user ON docs.space_members (user_id);

-- +goose Down
DROP TABLE docs.space_members;
DROP INDEX docs.pages_by_space;
ALTER TABLE docs.pages DROP COLUMN space_id;
