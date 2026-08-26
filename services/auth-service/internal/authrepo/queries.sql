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

-- name: CountUsers :one
-- Bootstrap only (internal/bootstrap) — always called inside the
-- pg_advisory_xact_lock transaction, never on its own.
SELECT count(*) FROM auth.users;

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
