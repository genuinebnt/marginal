-- +goose Up
CREATE SCHEMA IF NOT EXISTS notify;

CREATE TABLE notify.notifications (
    -- Generated application-side, same reasoning as every other id in
    -- this repo — see e.g. auth.users.id's comment.
    id               UUID PRIMARY KEY,
    -- NO foreign key: auth.users belongs to auth-service's own database
    -- (DATA_MODEL.md — database per service, no cross-schema joins).
    user_id          UUID NOT NULL,
    -- The auth.outbox row id that produced this notification — the
    -- dedup key. NATS delivers at-least-once (RFC-002 §4 rule 5's
    -- reasoning applies here too), so a redelivered auth.user_registered
    -- event must not create a second welcome notification.
    source_event_id  UUID NOT NULL UNIQUE,
    -- 'welcome' is the only kind this repo produces (DATA_MODEL.md §10:
    -- auth.user_registered is the one event topic Track 1 can actually
    -- emit — every other notification-worthy topic needs sharing/RBAC/
    -- deactivation, none of which are in scope). TEXT with an implicit
    -- open set, not an ENUM — same reasoning collab.ops.actor_kind and
    -- collab.ops.kind already use: extending a CHECK-less TEXT column is
    -- ordinary DDL, extending an ENUM is a special-cased transaction.
    kind             TEXT NOT NULL,
    message          TEXT NOT NULL,
    read_at          TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON notify.notifications (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS notify.notifications;
DROP SCHEMA IF EXISTS notify;
