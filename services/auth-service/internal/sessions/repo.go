package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"marginal/auth-service/internal/authrepo/gen"
	"marginal/auth-service/internal/domain"
)

var ErrNotFound = errors.New("sessions: refresh token not found")

// NewToken is Insert's input — see users/repo.go's doc comment for why
// these are plain functions over *authrepo.Queries rather than a Repo
// interface: registration and rotation both need this insert to commit
// in the same transaction as another table's write, so the transaction
// is the caller's (internal/authservice) to manage.
type NewToken struct {
	ID        uuid.UUID
	UserID    domain.UserID
	TokenHash [32]byte
	ParentID  *uuid.UUID
	ExpiresAt time.Time
}

func Insert(ctx context.Context, q *authrepo.Queries, nt NewToken) error {
	var parentID pgtype.UUID
	if nt.ParentID != nil {
		parentID = pgtype.UUID{Bytes: *nt.ParentID, Valid: true}
	}
	err := q.InsertRefreshToken(ctx, authrepo.InsertRefreshTokenParams{
		ID:        pgtype.UUID{Bytes: nt.ID, Valid: true},
		UserID:    pgtype.UUID{Bytes: uuid.UUID(nt.UserID), Valid: true},
		TokenHash: nt.TokenHash[:],
		ExpiresAt: pgtype.Timestamptz{Time: nt.ExpiresAt, Valid: true},
		ParentID:  parentID,
	})
	if err != nil {
		return fmt.Errorf("sessions: insert refresh token: %w", err)
	}
	return nil
}

func FindByHash(ctx context.Context, q *authrepo.Queries, hash [32]byte) (StoredToken, error) {
	row, err := q.FindRefreshTokenByHash(ctx, hash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredToken{}, ErrNotFound
	}
	if err != nil {
		return StoredToken{}, fmt.Errorf("sessions: find refresh token: %w", err)
	}
	return fromRow(row), nil
}

// Revoke marks a single row revoked — legitimate rotation revokes only
// the token just presented, not its whole family (that's RevokeChain,
// for the reuse-detected case).
func Revoke(ctx context.Context, q *authrepo.Queries, id uuid.UUID) error {
	if err := q.RevokeRefreshToken(ctx, pgtype.UUID{Bytes: id, Valid: true}); err != nil {
		return fmt.Errorf("sessions: revoke: %w", err)
	}
	return nil
}

// RevokeChain finds anyID's root (walking parent_id up) and revokes every
// descendant of that root — the fix for refresh-token reuse: presenting
// an already-consumed token means the whole chain is compromised.
func RevokeChain(ctx context.Context, q *authrepo.Queries, anyID uuid.UUID) (int64, error) {
	rootPg, err := q.FindRefreshTokenRootID(ctx, pgtype.UUID{Bytes: anyID, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("sessions: finding chain root: %w", err)
	}
	n, err := q.RevokeRefreshTokenChain(ctx, rootPg)
	if err != nil {
		return 0, fmt.Errorf("sessions: revoking chain: %w", err)
	}
	return n, nil
}

func RevokeAllForUser(ctx context.Context, q *authrepo.Queries, userID domain.UserID) (int64, error) {
	n, err := q.RevokeAllRefreshTokensForUser(ctx, pgtype.UUID{Bytes: uuid.UUID(userID), Valid: true})
	if err != nil {
		return 0, fmt.Errorf("sessions: revoke all for user: %w", err)
	}
	return n, nil
}

func fromRow(row authrepo.AuthRefreshToken) StoredToken {
	var parentID *uuid.UUID
	if row.ParentID.Valid {
		id := uuid.UUID(row.ParentID.Bytes)
		parentID = &id
	}
	var revokedAt *time.Time
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		revokedAt = &t
	}
	var hash [32]byte
	copy(hash[:], row.TokenHash)

	return StoredToken{
		ID:        uuid.UUID(row.ID.Bytes),
		UserID:    domain.UserID(uuid.UUID(row.UserID.Bytes)),
		TokenHash: hash,
		ParentID:  parentID,
		ExpiresAt: row.ExpiresAt.Time,
		RevokedAt: revokedAt,
		CreatedAt: row.CreatedAt.Time,
	}
}
