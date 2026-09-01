-- +goose Up
-- 00002 left the default space with NO admin, which violates the rule
-- ADR-013 §2 states and internal/spaces enforces: a space always has at
-- least one admin.
--
-- It made every existing user an EDITOR, which was right for preserving
-- access — nobody's ability to read or write changed on the day the model
-- did. But it meant nobody could ever manage that space's membership,
-- because granting a role requires admin and there was no admin to start
-- from. The rule was correct and the migration that had to satisfy it was
-- written first; this is the correction.
--
-- The founder (earliest registered user, the same one 00002 recorded as
-- created_by) is promoted. An arbitrary choice, but a stable one, and the
-- alternative — promoting everybody — would hand every account the ability
-- to remove every other.
UPDATE auth.memberships m
SET role = 'admin'
FROM auth.spaces s
WHERE s.is_default
  AND m.space_id = s.id
  AND m.user_id = s.created_by;

-- +goose Down
UPDATE auth.memberships m
SET role = 'editor'
FROM auth.spaces s
WHERE s.is_default AND m.space_id = s.id AND m.user_id = s.created_by;
