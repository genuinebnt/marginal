package spaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	authrepo "marginal/auth-service/internal/authrepo/gen"
	"marginal/auth-service/internal/outbox"
)

// PostgresStore implements Store over auth.memberships.
//
// It is deliberately thin: every rule lives in spaces.go, and this only
// knows how to read and write rows. A store that decided anything would be
// a second place authorization happens.
type PostgresStore struct {
	q *authrepo.Queries
}

func NewPostgresStore(q *authrepo.Queries) *PostgresStore { return &PostgresStore{q: q} }

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func (s *PostgresStore) RoleInSpace(ctx context.Context, userID, spaceID uuid.UUID) (Role, error) {
	role, err := s.q.RoleInSpace(ctx, authrepo.RoleInSpaceParams{
		UserID: pgUUID(userID), SpaceID: pgUUID(spaceID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Not a member. Distinguished from an error, because "you are not
		// in this space" is an answer rather than a failure.
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("spaces: reading role: %w", err)
	}
	return Role(role), nil
}

func (s *PostgresStore) CountAdmins(ctx context.Context, spaceID uuid.UUID) (int, error) {
	n, err := s.q.CountAdmins(ctx, pgUUID(spaceID))
	if err != nil {
		return 0, fmt.Errorf("spaces: counting admins: %w", err)
	}
	return int(n), nil
}

func (s *PostgresStore) IsDefaultSpace(ctx context.Context, spaceID uuid.UUID) (bool, error) {
	sp, err := s.q.GetSpace(ctx, pgUUID(spaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("spaces: reading space: %w", err)
	}
	return sp.IsDefault, nil
}

func (s *PostgresStore) Upsert(ctx context.Context, m Membership, grantedBy uuid.UUID) error {
	_, err := s.q.UpsertMembership(ctx, authrepo.UpsertMembershipParams{
		UserID: pgUUID(m.UserID), SpaceID: pgUUID(m.SpaceID),
		Role: string(m.Role), GrantedBy: pgUUID(grantedBy),
	})
	if err != nil {
		return fmt.Errorf("spaces: upserting membership: %w", err)
	}
	return nil
}

func (s *PostgresStore) Delete(ctx context.Context, userID, spaceID uuid.UUID) error {
	if _, err := s.q.DeleteMembership(ctx, authrepo.DeleteMembershipParams{
		UserID: pgUUID(userID), SpaceID: pgUUID(spaceID),
	}); err != nil {
		return fmt.Errorf("spaces: deleting membership: %w", err)
	}
	return nil
}

func invitationFrom(r authrepo.AuthSpaceInvitation) Invitation {
	inv := Invitation{
		ID: uuid.UUID(r.ID.Bytes), SpaceID: uuid.UUID(r.SpaceID.Bytes),
		UserID: uuid.UUID(r.UserID.Bytes), Role: Role(r.Role),
		InvitedBy: uuid.UUID(r.InvitedBy.Bytes), CreatedAt: r.CreatedAt.Time,
	}
	if r.RespondedAt.Valid {
		t := r.RespondedAt.Time
		inv.RespondedAt = &t
	}
	if r.Accepted != nil {
		inv.Accepted = *r.Accepted
	}
	return inv
}

func (s *PostgresStore) CreateInvitation(
	ctx context.Context, id, spaceID, userID, invitedBy uuid.UUID, role Role,
) (Invitation, error) {
	row, err := s.q.CreateInvitation(ctx, authrepo.CreateInvitationParams{
		ID: pgUUID(id), SpaceID: pgUUID(spaceID), UserID: pgUUID(userID),
		Role: string(role), InvitedBy: pgUUID(invitedBy),
	})
	if err != nil {
		// The partial unique index (one PENDING per person per space) makes
		// a double invite a conflict rather than a second row.
		if isUniqueViolation(err) {
			return Invitation{}, ErrAlreadyInvited
		}
		return Invitation{}, fmt.Errorf("spaces: creating invitation: %w", err)
	}
	return invitationFrom(row), nil
}

func (s *PostgresStore) Answer(
	ctx context.Context, id, userID uuid.UUID, accept bool,
) (Invitation, bool, error) {
	row, err := s.q.RespondToInvitation(ctx, authrepo.RespondToInvitationParams{
		ID: pgUUID(id), UserID: pgUUID(userID), Accepted: &accept,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing open, and belonging to this caller, with that id. Not an
		// error here — Respond turns it into one answer for all three cases
		// so that a probe cannot tell them apart.
		return Invitation{}, false, nil
	}
	if err != nil {
		return Invitation{}, false, fmt.Errorf("spaces: answering invitation: %w", err)
	}
	return invitationFrom(row), true, nil
}

func (s *PostgresStore) Pending(ctx context.Context, userID uuid.UUID) ([]Invitation, error) {
	rows, err := s.q.ListPendingInvitationsForUser(ctx, pgUUID(userID))
	if err != nil {
		return nil, fmt.Errorf("spaces: listing invitations: %w", err)
	}
	out := make([]Invitation, 0, len(rows))
	for _, r := range rows {
		inv := invitationFrom(authrepo.AuthSpaceInvitation{
			ID: r.ID, SpaceID: r.SpaceID, UserID: r.UserID, Role: r.Role,
			InvitedBy: r.InvitedBy, CreatedAt: r.CreatedAt,
			RespondedAt: r.RespondedAt, Accepted: r.Accepted,
		})
		// The two joined columns § 20's row needs to say its sentence.
		inv.SpaceName, inv.InvitedByName = r.SpaceName, r.InvitedByName
		out = append(out, inv)
	}
	return out, nil
}

// isUniqueViolation isolates the one Postgres error code this package has
// a rule for. 23505 is a CONFLICT with an existing row, which here means
// exactly one thing — there is already a pending invitation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Service is the transactional wrapper: it runs a rule from spaces.go and
// the outbox write for it in ONE transaction.
//
// That pairing is the whole reason this type exists rather than the gRPC
// layer calling Grant directly. A membership row and the event announcing
// it must land together or not at all — a grant that is durable without its
// event leaves document-service filtering reads against a projection that
// will never learn about it, and no retry will fix that because the write
// already succeeded.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Grant applies the rule and publishes auth.role_granted, atomically.
func (s *Service) Grant(ctx context.Context, caller, spaceID, target uuid.UUID, role Role) error {
	return s.inTx(ctx, func(q *authrepo.Queries) error {
		if err := Grant(ctx, NewPostgresStore(q), caller, spaceID, target, role); err != nil {
			return err
		}
		return outbox.WriteRoleEvent(ctx, q, outbox.EventRoleGranted, outbox.RolePayload{
			UserID: target, SpaceID: spaceID, Role: string(role), GrantedAt: time.Now().UTC(),
		})
	})
}

// Revoke applies the rule and publishes auth.role_revoked, atomically.
func (s *Service) Revoke(ctx context.Context, caller, spaceID, target uuid.UUID) error {
	return s.inTx(ctx, func(q *authrepo.Queries) error {
		if err := Revoke(ctx, NewPostgresStore(q), caller, spaceID, target); err != nil {
			return err
		}
		return outbox.WriteRoleEvent(ctx, q, outbox.EventRoleRevoked, outbox.RolePayload{
			UserID: target, SpaceID: spaceID, GrantedAt: time.Now().UTC(),
		})
	})
}

// CreateSpace makes a space and its first admin — the creator — in one
// transaction. A space created without an admin is a space nobody can ever
// administer, and the last-admin rule would then have nothing to protect.
func (s *Service) CreateSpace(ctx context.Context, creator uuid.UUID, name string) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("spaces: generating space id: %w", err)
	}
	err = s.inTx(ctx, func(q *authrepo.Queries) error {
		if _, err := q.CreateSpace(ctx, authrepo.CreateSpaceParams{
			ID: pgUUID(id), Name: name, CreatedBy: pgUUID(creator),
		}); err != nil {
			return fmt.Errorf("spaces: creating space: %w", err)
		}
		if _, err := q.UpsertMembership(ctx, authrepo.UpsertMembershipParams{
			UserID: pgUUID(creator), SpaceID: pgUUID(id),
			Role: string(Admin), GrantedBy: pgUUID(creator),
		}); err != nil {
			return fmt.Errorf("spaces: seeding creator membership: %w", err)
		}
		return outbox.WriteRoleEvent(ctx, q, outbox.EventRoleGranted, outbox.RolePayload{
			UserID: creator, SpaceID: id, Role: string(Admin), GrantedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (s *Service) inTx(ctx context.Context, fn func(*authrepo.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("spaces: beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(authrepo.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("spaces: committing: %w", err)
	}
	return nil
}

// Invite records an invitation and publishes auth.member_invited, atomically
// — the same pairing Grant and Revoke have, for the same reason.
func (s *Service) Invite(
	ctx context.Context, caller, spaceID, target uuid.UUID, role Role,
) (Invitation, error) {
	var inv Invitation
	err := s.inTx(ctx, func(q *authrepo.Queries) error {
		var err error
		inv, err = Invite(ctx, NewPostgresStore(q), caller, spaceID, target, role)
		if err != nil {
			return err
		}
		return outbox.WriteInviteEvent(ctx, q, outbox.InvitePayload{
			InvitationID: inv.ID, UserID: inv.UserID, SpaceID: inv.SpaceID,
			Role: string(inv.Role), InvitedBy: inv.InvitedBy, InvitedAt: time.Now().UTC(),
		})
	})
	return inv, err
}

// Respond answers an invitation, and on ACCEPT writes the membership and
// publishes auth.role_granted in the same transaction.
//
// The event is the same one an admin's Grant publishes, deliberately:
// document-service's projection learns about an accepted invitation through
// the path it already has, rather than needing a second consumer that would
// have to be kept in agreement with the first.
func (s *Service) Respond(
	ctx context.Context, caller, invitationID uuid.UUID, accept bool,
) (Invitation, error) {
	var inv Invitation
	err := s.inTx(ctx, func(q *authrepo.Queries) error {
		var err error
		inv, err = Respond(ctx, NewPostgresStore(q), caller, invitationID, accept)
		if err != nil || !accept {
			return err
		}
		return outbox.WriteRoleEvent(ctx, q, outbox.EventRoleGranted, outbox.RolePayload{
			UserID: inv.UserID, SpaceID: inv.SpaceID, Role: string(inv.Role),
			GrantedAt: time.Now().UTC(),
		})
	})
	return inv, err
}

func (s *Service) PendingInvitations(ctx context.Context, userID uuid.UUID) ([]Invitation, error) {
	return NewPostgresStore(authrepo.New(s.pool)).Pending(ctx, userID)
}
