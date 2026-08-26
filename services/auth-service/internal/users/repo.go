package users

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"marginal/auth-service/internal/authrepo/gen"
	"marginal/auth-service/internal/domain"
)

var (
	ErrNotFound   = errors.New("users: not found")
	ErrEmailTaken = errors.New("users: email already registered")
)

const uniqueViolation = "23505"

// Insert and the other functions here take *authrepo.Queries directly
// (not a Repo interface owning its own pool) because registration must
// commit together with the caller's first refresh-token insert
// (internal/authservice) — the transaction is the caller's to manage, and
// authrepo.Queries.WithTx already gives a tx-scoped handle for exactly
// that. There is also only one real backing store, so a Repo abstraction
// here would have no second implementation to justify it.
func Insert(ctx context.Context, q *authrepo.Queries, nu NewUser) (User, error) {
	row, err := q.InsertUser(ctx, authrepo.InsertUserParams{
		ID:           toPgUUID(nu.ID),
		Email:        nu.Email.String(),
		PasswordHash: nu.PasswordHash.String(),
		DisplayName:  nu.DisplayName.String(),
		CursorColor:  nu.CursorColor.String(),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("users: insert: %w", err)
	}
	return fromRow(row), nil
}

func FindByEmail(ctx context.Context, q *authrepo.Queries, email domain.Email) (User, error) {
	row, err := q.GetUserByEmail(ctx, email.String())
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("users: find by email: %w", err)
	}
	return fromRow(row), nil
}

func FindByID(ctx context.Context, q *authrepo.Queries, id domain.UserID) (User, error) {
	row, err := q.GetUserByID(ctx, toPgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("users: find by id: %w", err)
	}
	return fromRow(row), nil
}

// Count is bootstrap-only (internal/authservice's Register), always
// called inside the pg_advisory_xact_lock transaction, never on its own.
func Count(ctx context.Context, q *authrepo.Queries) (int64, error) {
	n, err := q.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("users: count: %w", err)
	}
	return n, nil
}

func fromRow(row authrepo.AuthUser) User {
	return User{
		ID:           domain.UserID(fromPgUUID(row.ID)),
		Email:        domain.EmailFromStored(row.Email),
		PasswordHash: domain.NewPasswordHash(row.PasswordHash),
		DisplayName:  domain.DisplayNameFromStored(row.DisplayName),
		CursorColor:  domain.CursorColorFromStored(row.CursorColor),
		CreatedAt:    row.CreatedAt.Time,
	}
}

func toPgUUID(id domain.UserID) pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.UUID(id), Valid: true}
}

func fromPgUUID(id pgtype.UUID) uuid.UUID { return uuid.UUID(id.Bytes) }
