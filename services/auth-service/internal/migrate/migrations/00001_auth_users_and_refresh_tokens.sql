-- +goose Up
CREATE SCHEMA IF NOT EXISTS auth;

CREATE TABLE auth.users (
    -- Generated application-side (domain.NewUserID) — every other id in
    -- this repo is app-generated too; a Postgres-side DEFAULT would also
    -- tie this schema to PG18's native uuidv7(), which Cloud SQL doesn't
    -- offer (deploy/terraform).
    id             UUID PRIMARY KEY,
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
    -- The Jti generated at issuance (domain.NewJti) — app-generated, same
    -- reasoning as auth.users.id above.
    id          UUID PRIMARY KEY,
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

-- One outbox per publishing service (DATA_MODEL.md § Outbox) — auth's own.
-- auth.user_registered is the one event in DATA_MODEL.md §10's topic table
-- this repo can actually produce (every other auth.* topic needs
-- sharing/RBAC/deactivation, none in Track 1 scope) — its one consumer is
-- notification-service (internal/notify's NATS subscriber).
CREATE TABLE auth.outbox (
    -- Generated application-side, same reasoning as every other id in
    -- this repo (see auth.users.id's comment above).
    id            UUID PRIMARY KEY,
    aggregate_id  UUID NOT NULL, -- user_id
    event_type    TEXT NOT NULL,
    payload       JSONB NOT NULL DEFAULT '{}',
    published_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON auth.outbox (created_at) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS auth.outbox;
DROP TABLE IF EXISTS auth.refresh_tokens;
DROP TABLE IF EXISTS auth.users;
DROP SCHEMA IF EXISTS auth;
