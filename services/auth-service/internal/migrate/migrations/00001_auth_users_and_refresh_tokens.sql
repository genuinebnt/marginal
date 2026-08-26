-- +goose Up
CREATE SCHEMA IF NOT EXISTS auth;

CREATE TABLE auth.users (
    id             UUID PRIMARY KEY DEFAULT uuidv7(),
    email          TEXT NOT NULL UNIQUE,
    -- Argon2id PHC string. Never a raw hash, never a separate salt column:
    -- the PHC format carries algorithm, parameters, and salt together, so
    -- parameters can be upgraded without a schema migration.
    password_hash  TEXT NOT NULL,
    display_name   TEXT NOT NULL,
    -- Assigned at signup so collaborators are visually distinguishable.
    cursor_color   TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auth.refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id     UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    -- SHA-256 of the token, never the token itself: a database leak must
    -- not yield usable credentials.
    token_hash  BYTEA NOT NULL UNIQUE,
    -- Rotation chain. On refresh the old row is revoked and a new one
    -- issued with parent_id set. Reuse of a revoked token means theft —
    -- revoke the entire chain (internal/sessions's rotation logic).
    parent_id   UUID REFERENCES auth.refresh_tokens(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON auth.refresh_tokens (user_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS auth.refresh_tokens;
DROP TABLE IF EXISTS auth.users;
DROP SCHEMA IF EXISTS auth;
