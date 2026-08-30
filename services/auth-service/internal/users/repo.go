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

// List returns every user, newest first — § 18 ADMIN's PEOPLE
// panel.
//
// Unpaginated, matching the query's own reasoning: a self-hosted
// instance's people list is short by construction, and a cursor
// API nobody can exercise is worse than none. The row carries no
// password hash, so the User values returned here have an empty
// PasswordHash — which is correct, not a gap: an admin listing
// has no use for it.
func List(ctx context.Context, q *authrepo.Queries) ([]User, error) {
	rows, err := q.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("users: list: %w", err)
	}
	out := make([]User, 0, len(rows))
	for _, row := range rows {
		out = append(out, User{
			ID:          domain.UserID(fromPgUUID(row.ID)),
			Email:       domain.EmailFromStored(row.Email),
			DisplayName: domain.DisplayNameFromStored(row.DisplayName),
			CursorColor: domain.CursorColorFromStored(row.CursorColor),
			CreatedAt:   row.CreatedAt.Time,
		})
	}
	return out, nil
}

// CountActiveSessions counts refresh tokens that are neither
// revoked nor expired — "signed in somewhere".
//
// Deliberately not a count of live WebSocket connections, which
// is the other thing "sessions" could mean and is
// collaboration-service's number. § 18 says which it means.
func CountActiveSessions(ctx context.Context, q *authrepo.Queries) (int64, error) {
	n, err := q.CountActiveSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("users: count active sessions: %w", err)
	}
	return n, nil
}
