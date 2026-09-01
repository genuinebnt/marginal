// Package spaces owns the permission boundary: who is in which space, and
// with what rank (ADR-013 §2, docs/api/spaces.md).
//
// The rules live HERE, not in the gRPC layer, for the same reason every
// other service in this repo keeps its logic out of api.go: an
// authorization rule that lives in a transport handler is a rule that is
// only enforced on the transport that happens to call it. This package is
// the only thing that decides whether a grant is allowed.
package spaces

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Role is a rank, and the ordering matters — `admin` may do everything
// `editor` may, which is a comparison rather than a set membership test.
type Role string

const (
	Viewer Role = "viewer"
	Editor Role = "editor"
	Admin  Role = "admin"
)

// rank orders the roles. Unknown roles rank BELOW viewer rather than
// panicking: a role string that reached the database is already past a
// CHECK constraint, and a value that somehow isn't one of the three should
// grant nothing rather than crash the service reading it.
func (r Role) rank() int {
	switch r {
	case Viewer:
		return 1
	case Editor:
		return 2
	case Admin:
		return 3
	default:
		return 0
	}
}

// Valid reports whether r is one of the three roles this repo has.
func (r Role) Valid() bool { return r.rank() > 0 }

// AtLeast is the whole authorization question: "is my rank sufficient".
func (r Role) AtLeast(min Role) bool { return r.rank() >= min.rank() }

var (
	// ErrNotAMember is returned for a space the caller is not in — and
	// callers turn it into NOT_FOUND, never PERMISSION_DENIED. A 403 on a
	// resource you cannot see confirms it exists, and space names say what
	// teams are working on (docs/api/spaces.md §3).
	ErrNotAMember = errors.New("spaces: not a member")
	// ErrInsufficientRole is a member without the rank — this one IS a
	// PERMISSION_DENIED, because "it exists" is already known to them.
	ErrInsufficientRole = errors.New("spaces: insufficient role")
	// ErrLastAdmin refuses to leave a space nobody can administer.
	ErrLastAdmin   = errors.New("spaces: a space must keep at least one admin")
	ErrInvalidRole = errors.New("spaces: unknown role")
	// ErrDefaultSpace guards the space every pre-v3.1.0 page was migrated
	// into: deleting it would orphan them.
	ErrDefaultSpace = errors.New("spaces: the default space cannot be removed")
)

// Membership is one row of auth.memberships, joined with the user's name
// for the screen that lists it.
type Membership struct {
	UserID      uuid.UUID
	SpaceID     uuid.UUID
	Role        Role
	DisplayName string
	Email       string
}

// Space is one row of auth.spaces, plus what the ASKING user may do in it.
type Space struct {
	ID        uuid.UUID
	Name      string
	IsDefault bool
	CreatedBy uuid.UUID
	YourRole  Role
	Members   int
}

// Store is this package's port onto the database — an interface declared
// at its point of use, so the rules below can be tested without Postgres.
type Store interface {
	RoleInSpace(ctx context.Context, userID, spaceID uuid.UUID) (Role, error)
	CountAdmins(ctx context.Context, spaceID uuid.UUID) (int, error)
	IsDefaultSpace(ctx context.Context, spaceID uuid.UUID) (bool, error)
	Upsert(ctx context.Context, m Membership, grantedBy uuid.UUID) error
	Delete(ctx context.Context, userID, spaceID uuid.UUID) error
}

// Authorize resolves the caller's role in a space and checks it against the
// minimum the operation needs.
//
// The two failure modes are deliberately different errors, and that
// difference is the security-relevant part: a non-member learns nothing
// about whether the space exists, while a member who lacks the rank gets a
// straight answer because they already know it does.
func Authorize(ctx context.Context, store Store, caller, spaceID uuid.UUID, min Role) (Role, error) {
	role, err := store.RoleInSpace(ctx, caller, spaceID)
	if err != nil {
		return "", err
	}
	if !role.Valid() {
		return "", ErrNotAMember
	}
	if !role.AtLeast(min) {
		return role, ErrInsufficientRole
	}
	return role, nil
}

// Grant adds or changes someone's role. Admin-only.
//
// The last-admin rule applies to DEMOTION as much as to removal: an admin
// demoting themselves to editor while they are the only admin leaves the
// same unadministrable space that removing them would, and it is the
// easier of the two mistakes to make by accident.
func Grant(ctx context.Context, store Store, caller, spaceID, target uuid.UUID, role Role) error {
	if !role.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidRole, role)
	}
	if _, err := Authorize(ctx, store, caller, spaceID, Admin); err != nil {
		return err
	}

	current, err := store.RoleInSpace(ctx, target, spaceID)
	if err != nil {
		return err
	}
	if current == Admin && role != Admin {
		if err := requireAnotherAdmin(ctx, store, spaceID); err != nil {
			return err
		}
	}

	return store.Upsert(ctx, Membership{UserID: target, SpaceID: spaceID, Role: role}, caller)
}

// Revoke removes someone from a space. Admin-only, and never the last one.
func Revoke(ctx context.Context, store Store, caller, spaceID, target uuid.UUID) error {
	if _, err := Authorize(ctx, store, caller, spaceID, Admin); err != nil {
		return err
	}

	current, err := store.RoleInSpace(ctx, target, spaceID)
	if err != nil {
		return err
	}
	if !current.Valid() {
		// Removing somebody who is not a member is not an error — the
		// caller's intent ("they should not be in this space") already
		// holds. Making it fail would mean a client has to check first,
		// and then race with anyone else doing the same.
		return nil
	}
	if current == Admin {
		if err := requireAnotherAdmin(ctx, store, spaceID); err != nil {
			return err
		}
	}
	return store.Delete(ctx, target, spaceID)
}

func requireAnotherAdmin(ctx context.Context, store Store, spaceID uuid.UUID) error {
	admins, err := store.CountAdmins(ctx, spaceID)
	if err != nil {
		return err
	}
	if admins <= 1 {
		return ErrLastAdmin
	}
	return nil
}
