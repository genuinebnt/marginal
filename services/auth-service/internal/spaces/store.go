package spaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
