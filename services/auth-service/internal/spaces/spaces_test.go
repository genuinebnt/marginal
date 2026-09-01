package spaces

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeStore is the membership table, in a map. The rules under test are
// about which changes are ALLOWED, not about SQL, so a real database here
// would test pgx.
type fakeStore struct {
	roles   map[[2]uuid.UUID]Role
	deleted [][2]uuid.UUID
	upserts []Membership
}

func newStore() *fakeStore { return &fakeStore{roles: map[[2]uuid.UUID]Role{}} }

func key(u, s uuid.UUID) [2]uuid.UUID { return [2]uuid.UUID{u, s} }

func (f *fakeStore) RoleInSpace(_ context.Context, u, s uuid.UUID) (Role, error) {
	return f.roles[key(u, s)], nil
}
func (f *fakeStore) CountAdmins(_ context.Context, s uuid.UUID) (int, error) {
	n := 0
	for k, r := range f.roles {
		if k[1] == s && r == Admin {
			n++
		}
	}
	return n, nil
}
func (f *fakeStore) IsDefaultSpace(context.Context, uuid.UUID) (bool, error) { return false, nil }
func (f *fakeStore) Upsert(_ context.Context, m Membership, _ uuid.UUID) error {
	f.roles[key(m.UserID, m.SpaceID)] = m.Role
	f.upserts = append(f.upserts, m)
	return nil
}
func (f *fakeStore) Delete(_ context.Context, u, s uuid.UUID) error {
	delete(f.roles, key(u, s))
	f.deleted = append(f.deleted, key(u, s))
	return nil
}

func TestRankIsAnOrdering(t *testing.T) {
	if !Admin.AtLeast(Editor) || !Admin.AtLeast(Viewer) || !Editor.AtLeast(Viewer) {
		t.Error("a higher rank must satisfy a lower minimum")
	}
	if Viewer.AtLeast(Editor) || Editor.AtLeast(Admin) {
		t.Error("a lower rank must not satisfy a higher minimum")
	}
	if Role("owner").Valid() || Role("").Valid() {
		t.Error("an unknown role must not be valid")
	}
	// The important half: an unknown role grants NOTHING, rather than
	// ranking above something by accident.
	if Role("superuser").AtLeast(Viewer) {
		t.Error("an unrecognised role satisfied a real minimum")
	}
}

// The distinction that matters for information leakage: a non-member must
// not be able to tell a space they cannot see from one that does not exist.
func TestANonMemberGetsNotAMemberNotInsufficientRole(t *testing.T) {
	store := newStore()
	space, stranger := uuid.New(), uuid.New()

	_, err := Authorize(context.Background(), store, stranger, space, Viewer)
	if !errors.Is(err, ErrNotAMember) {
		t.Fatalf("err = %v, want ErrNotAMember — a 403 here would confirm the space exists", err)
	}
}

func TestAMemberWithoutTheRankGetsInsufficientRole(t *testing.T) {
	store := newStore()
	space, viewer := uuid.New(), uuid.New()
	store.roles[key(viewer, space)] = Viewer

	_, err := Authorize(context.Background(), store, viewer, space, Admin)
	if !errors.Is(err, ErrInsufficientRole) {
		t.Fatalf("err = %v, want ErrInsufficientRole — they already know it exists", err)
	}
}

func TestOnlyAnAdminMayGrant(t *testing.T) {
	space, editor, target := uuid.New(), uuid.New(), uuid.New()
	for _, caller := range []Role{Viewer, Editor} {
		store := newStore()
		store.roles[key(editor, space)] = caller
		err := Grant(context.Background(), store, editor, space, target, Viewer)
		if !errors.Is(err, ErrInsufficientRole) {
			t.Errorf("%s could grant a role (err = %v)", caller, err)
		}
		if len(store.upserts) != 0 {
			t.Errorf("%s's refused grant still wrote a row", caller)
		}
	}
}

func TestAnAdminMayGrantAndItIsAnUpsert(t *testing.T) {
	store := newStore()
	space, admin, target := uuid.New(), uuid.New(), uuid.New()
	store.roles[key(admin, space)] = Admin
	ctx := context.Background()

	if err := Grant(ctx, store, admin, space, target, Viewer); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	// The same call again with a different role is a CHANGE, not a
	// duplicate-key failure — "add Ada as viewer" and "change Ada to
	// editor" are one intent (docs/api/spaces.md §1).
	if err := Grant(ctx, store, admin, space, target, Editor); err != nil {
		t.Fatalf("re-granting a different role: %v", err)
	}
	if got := store.roles[key(target, space)]; got != Editor {
		t.Fatalf("role = %q, want editor", got)
	}
}

// The rule that most needs a test, because the easy version of it only
// covers removal.
func TestTheLastAdminCannotBeDemoted(t *testing.T) {
	store := newStore()
	space, admin := uuid.New(), uuid.New()
	store.roles[key(admin, space)] = Admin

	err := Grant(context.Background(), store, admin, space, admin, Editor)
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin — demoting yourself leaves the same unadministrable space that removing yourself would", err)
	}
	if got := store.roles[key(admin, space)]; got != Admin {
		t.Fatalf("role changed to %q despite the refusal", got)
	}
}

func TestTheLastAdminCannotBeRemoved(t *testing.T) {
	store := newStore()
	space, admin := uuid.New(), uuid.New()
	store.roles[key(admin, space)] = Admin

	if err := Revoke(context.Background(), store, admin, space, admin); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
	if len(store.deleted) != 0 {
		t.Fatal("the refused revoke still deleted a row")
	}
}

func TestASecondAdminMakesDemotionLegal(t *testing.T) {
	store := newStore()
	space, first, second := uuid.New(), uuid.New(), uuid.New()
	store.roles[key(first, space)] = Admin
	store.roles[key(second, space)] = Admin

	if err := Grant(context.Background(), store, first, space, first, Editor); err != nil {
		t.Fatalf("demoting one of two admins: %v", err)
	}
	if got := store.roles[key(first, space)]; got != Editor {
		t.Fatalf("role = %q, want editor", got)
	}
}

// Revoking somebody who is not there already satisfies the caller's intent.
// Making it an error means every client checks first, and then races with
// anyone else doing the same.
func TestRevokingANonMemberIsNotAnError(t *testing.T) {
	store := newStore()
	space, admin, stranger := uuid.New(), uuid.New(), uuid.New()
	store.roles[key(admin, space)] = Admin

	if err := Revoke(context.Background(), store, admin, space, stranger); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if len(store.deleted) != 0 {
		t.Error("deleted a row for somebody who had none")
	}
}

func TestAnUnknownRoleIsRefusedBeforeAnyAuthorizationHappens(t *testing.T) {
	store := newStore()
	space, admin, target := uuid.New(), uuid.New(), uuid.New()
	store.roles[key(admin, space)] = Admin

	if err := Grant(context.Background(), store, admin, space, target, Role("owner")); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("err = %v, want ErrInvalidRole", err)
	}
	if len(store.upserts) != 0 {
		t.Error("an unknown role still wrote a row")
	}
}

// A non-member must not be able to grant themselves in — the check runs
// against the CALLER's role, not the target's.
func TestAStrangerCannotGrantThemselvesIn(t *testing.T) {
	store := newStore()
	space, stranger := uuid.New(), uuid.New()

	err := Grant(context.Background(), store, stranger, space, stranger, Admin)
	if !errors.Is(err, ErrNotAMember) {
		t.Fatalf("err = %v, want ErrNotAMember", err)
	}
	if len(store.upserts) != 0 {
		t.Fatal("a stranger wrote themselves a membership")
	}
}
