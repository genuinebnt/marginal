-- name: InsertUser :one
-- id is generated application-side (uuid v7), same reasoning as
-- document-service's pages: keeping id generation out of the database
-- keeps the domain layer in control of it.
INSERT INTO auth.users (id, email, password_hash, display_name, cursor_color)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, email, password_hash, display_name, cursor_color, created_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, display_name, cursor_color, created_at
FROM auth.users
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, display_name, cursor_color, created_at
FROM auth.users
WHERE id = $1;

-- name: InsertRefreshToken :exec
INSERT INTO auth.refresh_tokens (id, user_id, token_hash, parent_id, expires_at)
VALUES ($1, $2, $3, sqlc.narg(parent_id), $4);

-- name: FindRefreshTokenByHash :one
-- Returns the row regardless of revoked/expired state — the rotation
-- state machine (internal/sessions) decides what each state means.
-- pgx.ErrNoRows means "no row with that hash exists at all", a bad token,
-- not a theft signal.
SELECT id, user_id, token_hash, parent_id, expires_at, revoked_at, created_at
FROM auth.refresh_tokens
WHERE token_hash = $1;

-- name: RevokeRefreshToken :exec
UPDATE auth.refresh_tokens SET revoked_at = NOW()
WHERE id = $1 AND revoked_at IS NULL;

-- name: FindRefreshTokenRootID :one
-- Reuse detection's fix starts here: walk parent_id from any_id up to the
-- chain's root (the row with no parent). Not a loop in application code —
-- one recursive CTE. RevokeRefreshTokenChain (below) does the other half:
-- walking back down from the root to revoke every descendant.
WITH RECURSIVE ancestors(rt_id, rt_parent_id) AS (
    SELECT rt.id, rt.parent_id FROM auth.refresh_tokens rt WHERE rt.id = $1
    UNION ALL
    SELECT rt.id, rt.parent_id
    FROM auth.refresh_tokens rt
    JOIN ancestors a ON rt.id = a.rt_parent_id
)
SELECT rt_id FROM ancestors WHERE rt_parent_id IS NULL;

-- name: RevokeRefreshTokenChain :execrows
-- Revokes root_id and every descendant reachable from it via parent_id —
-- the whole rotation family, since presenting a consumed token means the
-- entire chain is compromised, not just that one token.
WITH RECURSIVE descendants(rt_id) AS (
    SELECT rt.id FROM auth.refresh_tokens rt WHERE rt.id = $1
    UNION ALL
    SELECT rt.id
    FROM auth.refresh_tokens rt
    JOIN descendants d ON rt.parent_id = d.rt_id
)
UPDATE auth.refresh_tokens SET revoked_at = NOW()
WHERE id IN (SELECT rt_id FROM descendants) AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokensForUser :execrows
UPDATE auth.refresh_tokens SET revoked_at = NOW()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: InsertOutboxEvent :one
-- id is generated application-side, same reasoning as every other id in
-- this repo.
INSERT INTO auth.outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ClaimUnpublishedOutboxEvents :many
-- FOR UPDATE SKIP LOCKED (DATA_MODEL.md § Outbox): a second poller
-- instance skips rows the first already claimed instead of blocking on
-- them, so at-least-once delivery holds even with more than one replica
-- of this service running the poller loop.
SELECT * FROM auth.outbox
WHERE published_at IS NULL
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxEventsPublished :exec
UPDATE auth.outbox SET published_at = NOW()
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListUsers :many
-- § 18 ADMIN's PEOPLE panel. Newest first, and password_hash is
-- NOT selected — an admin list has no reason to carry it, and a
-- column that never leaves the repo cannot leak from a screen.
--
-- Unpaginated on purpose: this is a self-hosted instance whose
-- whole point is that the people list is short. A LIMIT here
-- would be a cursor API nobody can exercise.
SELECT id, email, display_name, cursor_color, created_at
FROM auth.users
ORDER BY created_at DESC;

-- name: CountActiveSessions :one
-- Refresh tokens that are neither revoked nor expired — the
-- honest definition of "signed in somewhere". Not WebSocket
-- connections, which is what § 18's SESSIONS readout could be
-- mistaken for; the screen says which it means.
SELECT COUNT(*) FROM auth.refresh_tokens
WHERE revoked_at IS NULL AND expires_at > NOW();

-- name: AuditAuthEvents :many
-- § 18b AUDIT LOG's auth rows — derived, like the content rows,
-- rather than written beside the thing they describe.
--
-- Three event kinds, all read from state that already exists:
-- a user row IS the registration, a refresh token row IS a
-- sign-in, and its revoked_at IS a sign-out. Nothing here is an
-- event somebody remembered to emit, which is why it cannot
-- disagree with what happened.
--
-- What is deliberately absent: failed sign-in attempts. Nothing
-- records them, and a row saying so would be an invention. The
-- screen says the gap out loud rather than leaving the reader to
-- assume there were none.
(
    SELECT id, 'auth.register'::text AS kind, id AS user_id, created_at AS at
    FROM auth.users
)
UNION ALL
(
    SELECT * FROM (
        (
            SELECT id, 'auth.signin'::text AS kind, user_id, created_at AS at
            FROM auth.refresh_tokens
            ORDER BY created_at DESC
            LIMIT sqlc.arg(row_limit)
        )
        UNION ALL
        (
            SELECT id, 'auth.signout'::text, user_id, revoked_at
            FROM auth.refresh_tokens
            WHERE revoked_at IS NOT NULL
            ORDER BY revoked_at DESC
            LIMIT sqlc.arg(row_limit)
        )
    ) recent
)
ORDER BY at DESC;

-- ── spaces & memberships (v3.1.0, ADR-013) ───────────────────────────────

-- name: ListSpacesForUser :many
-- Only the spaces the caller is in. `your_role` is the caller's own role,
-- which is why this joins memberships rather than listing spaces and
-- looking roles up after: a space you are not in must not appear at all.
SELECT s.id, s.name, s.is_default, s.created_by, s.created_at,
       m.role AS your_role,
       (SELECT count(*) FROM auth.memberships WHERE space_id = s.id) AS members
FROM auth.spaces s
JOIN auth.memberships m ON m.space_id = s.id AND m.user_id = $1
ORDER BY s.is_default DESC, s.created_at;

-- name: CreateSpace :one
INSERT INTO auth.spaces (id, name, created_by) VALUES ($1, $2, $3)
RETURNING id, name, is_default, created_by, created_at;

-- name: GetSpace :one
SELECT id, name, is_default, created_by, created_at FROM auth.spaces WHERE id = $1;

-- name: DefaultSpace :one
SELECT id, name, is_default, created_by, created_at FROM auth.spaces WHERE is_default;

-- name: RoleInSpace :one
-- The authorization question, asked as one indexed lookup on the primary
-- key. Absent means "not a member", which callers turn into NOT_FOUND
-- rather than PERMISSION_DENIED (docs/api/spaces.md §3).
SELECT role FROM auth.memberships WHERE user_id = $1 AND space_id = $2;

-- name: ListMembers :many
SELECT m.user_id, m.space_id, m.role, m.created_at,
       u.display_name, u.email
FROM auth.memberships m
JOIN auth.users u ON u.id = m.user_id
WHERE m.space_id = $1
ORDER BY m.role, u.display_name;

-- name: ListAllMemberships :many
-- document-service's periodic reconcile. Ordered so a consumer can page
-- deterministically if this ever grows past one response.
SELECT m.user_id, m.space_id, m.role, m.created_at,
       u.display_name, u.email
FROM auth.memberships m
JOIN auth.users u ON u.id = m.user_id
ORDER BY m.space_id, m.user_id;

-- name: UpsertMembership :one
-- An upsert because "change Ada to viewer" and "add Ada as viewer" are one
-- intent (docs/api/spaces.md §1).
INSERT INTO auth.memberships (user_id, space_id, role, granted_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, space_id) DO UPDATE SET role = EXCLUDED.role, granted_by = EXCLUDED.granted_by
RETURNING user_id, space_id, role, created_at;

-- name: DeleteMembership :execrows
DELETE FROM auth.memberships WHERE user_id = $1 AND space_id = $2;

-- name: CountAdmins :one
-- "A space always has at least one admin" is a claim about the SET of
-- remaining rows, so it is checked in the transaction rather than by a
-- constraint (DATA_MODEL.md says why a trigger is worse).
SELECT count(*) FROM auth.memberships WHERE space_id = $1 AND role = 'admin';

-- name: CreateDefaultSpace :one
-- The bootstrap path: a fresh database has no users when 00002 runs, so it
-- creates no space either, and the FIRST registration has to. The partial
-- unique index on is_default means a race produces a constraint violation
-- rather than two default spaces.
INSERT INTO auth.spaces (id, name, is_default, created_by)
VALUES ($1, $2, TRUE, $3)
RETURNING id, name, is_default, created_by, created_at;
