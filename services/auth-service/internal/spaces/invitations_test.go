package spaces

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeInvites extends the membership double with the invitation half, so
// these rules are testable without Postgres — exactly as Grant/Revoke are.
type fakeInvites struct {
	*fakeStore
	rows map[uuid.UUID]*Invitation
	// pending mirrors the partial unique index: one open invitation per
	// (space, user). A double invite must conflict here the way it does in
	// the database, or the test would pass over a rule the schema owns.
	pending map[[2]uuid.UUID]uuid.UUID
}

func newInvites() *fakeInvites {
	return &fakeInvites{fakeStore: newStore(), rows: map[uuid.UUID]*Invitation{},
		pending: map[[2]uuid.UUID]uuid.UUID{}}
}

func (f *fakeInvites) CreateInvitation(
	_ context.Context, id, spaceID, userID, invitedBy uuid.UUID, role Role,
) (Invitation, error) {
	k := key(spaceID, userID)
	if _, open := f.pending[k]; open {
		return Invitation{}, ErrAlreadyInvited
	}
	inv := Invitation{ID: id, SpaceID: spaceID, UserID: userID, Role: role, InvitedBy: invitedBy}
	f.rows[id] = &inv
	f.pending[k] = id
	return inv, nil
}

func (f *fakeInvites) Answer(_ context.Context, id, userID uuid.UUID, accept bool) (Invitation, bool, error) {
	inv, ok := f.rows[id]
	if !ok || inv.UserID != userID || inv.RespondedAt != nil {
		return Invitation{}, false, nil
	}
	now := time.Unix(0, 0).UTC()
	inv.RespondedAt, inv.Accepted = &now, accept
	delete(f.pending, key(inv.SpaceID, inv.UserID))
	return *inv, true, nil
}

func (f *fakeInvites) Pending(context.Context, uuid.UUID) ([]Invitation, error) {
	return nil, nil
}

func TestInviteIsAdminOnly(t *testing.T) {
	st := newInvites()
	space, editor, target := uuid.New(), uuid.New(), uuid.New()
	st.roles[key(editor, space)] = Editor

	_, err := Invite(context.Background(), st, editor, space, target, Viewer)
	if !errors.Is(err, ErrInsufficientRole) {
		t.Fatalf("an editor must not be able to invite: %v", err)
	}
}

func TestInviteFromANonMemberIsNotFound(t *testing.T) {
	st := newInvites()
	space, stranger, target := uuid.New(), uuid.New(), uuid.New()

	_, err := Invite(context.Background(), st, stranger, space, target, Viewer)
	// ErrNotAMember, which the gRPC layer renders as NOT_FOUND — a 403
	// would confirm the space exists to somebody who cannot see it.
	if !errors.Is(err, ErrNotAMember) {
		t.Fatalf("want ErrNotAMember, got %v", err)
	}
}

// The rule that keeps "invite" and "promote" from being the same button.
func TestInviteRefusesSomebodyAlreadyInTheSpace(t *testing.T) {
	st := newInvites()
	space, admin, member := uuid.New(), uuid.New(), uuid.New()
	st.roles[key(admin, space)] = Admin
	st.roles[key(member, space)] = Viewer

	_, err := Invite(context.Background(), st, admin, space, member, Admin)
	if !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("want ErrAlreadyMember, got %v", err)
	}
	if len(st.upserts) != 0 {
		t.Fatal("a refused invitation must not have changed anybody's role")
	}
}

func TestInvitingTwiceConflicts(t *testing.T) {
	st := newInvites()
	space, admin, target := uuid.New(), uuid.New(), uuid.New()
	st.roles[key(admin, space)] = Admin

	if _, err := Invite(context.Background(), st, admin, space, target, Editor); err != nil {
		t.Fatal(err)
	}
	if _, err := Invite(context.Background(), st, admin, space, target, Editor); !errors.Is(err, ErrAlreadyInvited) {
		t.Fatalf("want ErrAlreadyInvited, got %v", err)
	}
}

// The point of the whole feature: an invitation grants NOTHING until it is
// accepted. This is the test that would fail first if somebody "simplified"
// invitations into a membership with a flag.
func TestAnInvitationGrantsNothingUntilAccepted(t *testing.T) {
	st := newInvites()
	space, admin, target := uuid.New(), uuid.New(), uuid.New()
	st.roles[key(admin, space)] = Admin

	inv, err := Invite(context.Background(), st, admin, space, target, Editor)
	if err != nil {
		t.Fatal(err)
	}
	if role, _ := st.RoleInSpace(context.Background(), target, space); role.Valid() {
		t.Fatalf("an unanswered invitation granted %q", role)
	}

	if _, err := Respond(context.Background(), st, target, inv.ID, true); err != nil {
		t.Fatal(err)
	}
	role, _ := st.RoleInSpace(context.Background(), target, space)
	if role != Editor {
		t.Fatalf("after accepting, role is %q, want editor", role)
	}
}

func TestDecliningGrantsNothingAndIsRecorded(t *testing.T) {
	st := newInvites()
	space, admin, target := uuid.New(), uuid.New(), uuid.New()
	st.roles[key(admin, space)] = Admin
	inv, _ := Invite(context.Background(), st, admin, space, target, Editor)

	got, err := Respond(context.Background(), st, target, inv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if role, _ := st.RoleInSpace(context.Background(), target, space); role.Valid() {
		t.Fatalf("declining granted %q", role)
	}
	if got.RespondedAt == nil || got.Accepted {
		t.Fatal("a decline must be recorded as answered-and-refused, not deleted")
	}
}

// Somebody else's invitation, a second answer, and an id that never existed
// must be indistinguishable — otherwise the error is an oracle for whether
// an invitation exists.
func TestOnlyTheInvitedMayAnswerAndOnlyOnce(t *testing.T) {
	st := newInvites()
	space, admin, target, other := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	st.roles[key(admin, space)] = Admin
	inv, _ := Invite(context.Background(), st, admin, space, target, Editor)

	if _, err := Respond(context.Background(), st, other, inv.ID, true); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("somebody else accepted it: %v", err)
	}
	if role, _ := st.RoleInSpace(context.Background(), other, space); role.Valid() {
		t.Fatal("answering somebody else's invitation granted a role")
	}
	if _, err := Respond(context.Background(), st, target, uuid.New(), true); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("an unknown id must answer the same way: %v", err)
	}
	if _, err := Respond(context.Background(), st, target, inv.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := Respond(context.Background(), st, target, inv.ID, true); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("a second answer must not silently succeed: %v", err)
	}
}

func TestAnInvitationCannotCarryAnInvalidRole(t *testing.T) {
	st := newInvites()
	space, admin := uuid.New(), uuid.New()
	st.roles[key(admin, space)] = Admin

	if _, err := Invite(context.Background(), st, admin, space, uuid.New(), Role("owner")); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("want ErrInvalidRole, got %v", err)
	}
}
