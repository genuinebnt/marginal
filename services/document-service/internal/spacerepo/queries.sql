-- docs.space_members — a PROJECTION of auth.memberships, never a second
-- source of truth (DATA_MODEL.md § docs.space_members). It decides what a
-- reader can SEE; auth-service decides what they can DO.

-- name: UpsertSpaceMember :exec
-- The guard is on granted_at, not on arrival order. Core NATS gives no
-- ordering guarantee across publishes, so an older grant can arrive after
-- a newer revoke — comparing the EVENT's own timestamp makes this converge
-- on the same answer whatever order the bus delivers in.
INSERT INTO docs.space_members (user_id, space_id, role, granted_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, space_id) DO UPDATE
  SET role = EXCLUDED.role, granted_at = EXCLUDED.granted_at
  WHERE docs.space_members.granted_at < EXCLUDED.granted_at;

-- name: DeleteSpaceMemberIfOlder :exec
-- The same guard, and it is needed on this side too: a revoke that arrives
-- after the grant that supersedes it must not delete the newer row.
DELETE FROM docs.space_members
WHERE user_id = $1 AND space_id = $2 AND granted_at < $3;

-- name: SpacesForUser :many
SELECT space_id FROM docs.space_members WHERE user_id = $1;

-- name: ClearSpaceMembers :exec
-- The reconcile's first half. Truncate-and-load in ONE transaction: a
-- projection that is partly old and partly new is worse than one that is
-- uniformly stale, because nothing about it is trustworthy.
DELETE FROM docs.space_members;

-- name: CountSpaceMembers :one
SELECT count(*) FROM docs.space_members;
