-- +goose Up
-- § 20's invite row, and ADR-013's now-decided invitation flow.
--
-- Its own table, NOT a column on auth.memberships. A membership with a
-- nullable `accepted_at` would put the invitation on the hot path of every
-- role check in the system, where forgetting the predicate once silently
-- grants access to somebody who never accepted. A separate table cannot be
-- forgotten: a role check that does not join it sees exactly what it saw
-- before invitations existed.
CREATE TABLE auth.space_invitations (
    -- Application-side, as every other id here.
    id          UUID PRIMARY KEY,
    space_id    UUID NOT NULL REFERENCES auth.spaces(id) ON DELETE CASCADE,
    -- Somebody who already has an account. Inviting an address that has
    -- none is a different feature (it needs email delivery and a signup
    -- that consumes a token), and § 20 depicts neither.
    user_id     UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('viewer','editor','admin')),
    invited_by  UUID NOT NULL REFERENCES auth.users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Answered, and how. A DECLINE stores the refusal rather than deleting
    -- the row: a deleted invitation cannot be told apart from one that was
    -- never sent, which makes "why can I not see that space" unanswerable.
    responded_at TIMESTAMPTZ,
    accepted     BOOLEAN
);

-- At most one PENDING invitation per person per space. A partial unique
-- index rather than a plain one, so a declined invitation can be followed
-- by a fresh one — being asked again after saying no is normal, being
-- asked twice at once is not.
CREATE UNIQUE INDEX space_invitations_one_pending
    ON auth.space_invitations (space_id, user_id)
    WHERE responded_at IS NULL;

CREATE INDEX space_invitations_for_user
    ON auth.space_invitations (user_id) WHERE responded_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS auth.space_invitations;
