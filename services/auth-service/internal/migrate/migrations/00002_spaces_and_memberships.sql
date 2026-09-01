-- +goose Up
-- ADR-013 §2: the space is the permission boundary. See
-- docs/architecture/DATA_MODEL.md § auth Schema for the full reasoning.

CREATE TABLE auth.spaces (
    -- App-generated, like every other id here (see 00001's note on why not
    -- a Postgres-side uuidv7() default).
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    -- The space every pre-v3.1.0 page was migrated into. It cannot be
    -- deleted: it is what "the workspace" meant before spaces existed, and
    -- deleting it would orphan every page written before this migration.
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,
    created_by  UUID NOT NULL REFERENCES auth.users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- At most one default. A partial unique index rather than a CHECK, because
-- the constraint is across rows: "only one row may be true", which no
-- row-level check can express.
CREATE UNIQUE INDEX spaces_one_default ON auth.spaces (is_default) WHERE is_default;

CREATE TABLE auth.memberships (
    user_id     UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    space_id    UUID NOT NULL REFERENCES auth.spaces(id) ON DELETE CASCADE,
    -- The role lives on the MEMBERSHIP, not on the user: a person is an
    -- admin of one space and a viewer of another, and a column on
    -- auth.users could not say that.
    role        TEXT NOT NULL CHECK (role IN ('viewer','editor','admin')),
    granted_by  UUID REFERENCES auth.users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, space_id)
);

CREATE INDEX memberships_by_space ON auth.memberships (space_id);

-- The migration that preserves today's behaviour exactly (ADR-013's
-- "one shared pool is a migration, not a default").
--
-- One space, every existing user an EDITOR of it. Before this, anyone who
-- registered could edit anything; making everyone an editor means nobody's
-- access changes on the day the model does. Tightening it is then a
-- deliberate, separate act rather than a side effect of a schema change.
--
-- created_by is the earliest registered user — an arbitrary but stable
-- choice, and the column is NOT NULL. Skipped entirely when there are no
-- users yet (a fresh database), where the service creates the default space
-- on first registration instead.
-- +goose StatementBegin
DO $$
DECLARE
    founder UUID;
    space   UUID := '00000000-0000-7000-8000-00000000d0c5';
BEGIN
    SELECT id INTO founder FROM auth.users ORDER BY created_at, id LIMIT 1;
    IF founder IS NULL THEN
        RETURN;
    END IF;

    INSERT INTO auth.spaces (id, name, is_default, created_by)
    VALUES (space, 'Workspace', TRUE, founder);

    INSERT INTO auth.memberships (user_id, space_id, role, granted_by)
    SELECT id, space, 'editor', founder FROM auth.users;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE auth.memberships;
DROP TABLE auth.spaces;
