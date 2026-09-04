package spaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Invitations — ADR-013's decided flow, docs/api/spaces.md § 5.
//
// The rule that shapes everything here: AN INVITATION IS NOT A MEMBERSHIP.
// It lives in its own table, so a role check that does not know invitations
// exist behaves exactly as it did before they did. The alternative — a
// membership carrying `accepted_at` — puts the invitation on the hot path of
// every authorization decision in the system, where forgetting the predicate
// once grants access to somebody who never accepted.

var (
	// ErrAlreadyMember: an invitation is not a way to change somebody's
	// role. "Invite" and "promote" are different intents, and an admin
	// should not be able to perform the second by aiming at the first.
	ErrAlreadyMember = errors.New("spaces: already a member")
	// ErrAlreadyAnswered covers both "you answered this already" and "this
	// is not yours to answer" — deliberately one error, because telling
	// them apart would confirm that somebody else's invitation exists.
	ErrAlreadyAnswered = errors.New("spaces: invitation is not open to you")
	// ErrAlreadyInvited: one pending invitation per person per space,
	// enforced by a partial unique index rather than by a read-then-write
	// that two admins could both pass.
	ErrAlreadyInvited = errors.New("spaces: already invited")
)

// Invitation is one pending or answered invite.
type Invitation struct {
	ID            uuid.UUID
	SpaceID       uuid.UUID
	SpaceName     string
	UserID        uuid.UUID
	Role          Role
	InvitedBy     uuid.UUID
	InvitedByName string
	CreatedAt     time.Time
	RespondedAt   *time.Time
	Accepted      bool
}

// InvitationStore is the invitation half of the persistence port, declared
// separately from Store so the membership rules keep the surface they had.
type InvitationStore interface {
	Store
	CreateInvitation(ctx context.Context, id, spaceID, userID, invitedBy uuid.UUID, role Role) (Invitation, error)
	// Answer applies the response ONLY to an open invitation belonging to
	// this user. Reports (Invitation{}, false, nil) when there was nothing
	// to answer — a second click and somebody else's id look identical
	// from here, which is intended.
	Answer(ctx context.Context, id, userID uuid.UUID, accept bool) (Invitation, bool, error)
	Pending(ctx context.Context, userID uuid.UUID) ([]Invitation, error)
}

// Invite records an invitation. Admin of that space only — the same check
// Grant makes, deliberately in the same shape so the two cannot drift.
//
// This does NOT grant anything. Accepting does.
func Invite(
	ctx context.Context, store InvitationStore, caller, spaceID, target uuid.UUID, role Role,
) (Invitation, error) {
	if !role.Valid() {
		return Invitation{}, fmt.Errorf("%w: %q", ErrInvalidRole, role)
	}
	if _, err := Authorize(ctx, store, caller, spaceID, Admin); err != nil {
		return Invitation{}, err
	}
	current, err := store.RoleInSpace(ctx, target, spaceID)
	if err != nil {
		return Invitation{}, err
	}
	if current.Valid() {
		return Invitation{}, ErrAlreadyMember
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Invitation{}, fmt.Errorf("spaces: generating invitation id: %w", err)
	}
	return store.CreateInvitation(ctx, id, spaceID, target, caller, role)
}

// Respond answers an invitation, and on accept performs the grant.
//
// The grant does NOT go through Grant(), and that is worth stating plainly
// rather than looking like an oversight: Grant authorizes its CALLER as an
// admin of the space, and the person accepting is by definition not one
// yet. The authority here is the invitation itself, which an admin created
// under exactly that check. So the membership is written directly, from a
// role that was already validated at invite time and cannot be chosen by
// the person accepting.
func Respond(
	ctx context.Context, store InvitationStore, caller, invitationID uuid.UUID, accept bool,
) (Invitation, error) {
	inv, ok, err := store.Answer(ctx, invitationID, caller, accept)
	if err != nil {
		return Invitation{}, err
	}
	if !ok {
		return Invitation{}, ErrAlreadyAnswered
	}
	if !accept {
		// Declining stores the refusal rather than deleting the row: a
		// deleted invitation cannot be told apart from one never sent.
		return inv, nil
	}
	if err := store.Upsert(ctx,
		Membership{UserID: inv.UserID, SpaceID: inv.SpaceID, Role: inv.Role},
		inv.InvitedBy, // granted BY the person who invited, not by the accepter
	); err != nil {
		return Invitation{}, err
	}
	return inv, nil
}
