package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"marginal/auth-service/internal/authrepo/gen"
	"marginal/auth-service/internal/domain"
)

// The rotation rule, from the OAuth 2.0 Security BCP
// (docs/architecture/lld/auth-service.md §5): every refresh consumes the
// old token and issues a new one. Presenting a token that has already
// been consumed means it was stolen — so revoke the entire chain, not
// just that token.

var (
	// ErrRefreshTokenInvalid is "no row with that hash exists at all" —
	// a bad token, not a theft signal. Nothing to revoke.
	ErrRefreshTokenInvalid = errors.New("sessions: refresh token invalid")
	// ErrRefreshTokenExpired is a naturally expired, never-revoked token —
	// not a theft signal either.
	ErrRefreshTokenExpired = errors.New("sessions: refresh token expired")
	// ErrRefreshTokenReused means the presented token had already been
	// consumed by a prior rotation — its entire family is now revoked.
	// Both the legitimate client and any attacker are logged out; there's
	// no way to tell which one presented it, so the safe action is the
	// same for both.
	ErrRefreshTokenReused = errors.New("sessions: refresh token reused")
)

// RotationResult carries what the caller needs to log — user id and
// (only set when reuse was detected) how many tokens the family
// revocation touched, for the WARN-level log line
// docs/api/auth.md's status table requires.
type RotationResult struct {
	UserID        domain.UserID
	FamilyRevoked int64 // only meaningful when the error is ErrRefreshTokenReused
}

// Rotate implements the state machine: hash the presented token, look it
// up, and either detect reuse (revoke the family), reject an expired
// token, or perform a legitimate rotation (revoke the old row, insert a
// new one, both in q's transaction — the caller opens and commits it,
// same reasoning as users.Insert).
func Rotate(ctx context.Context, q *authrepo.Queries, presented string) (newPlaintext string, result RotationResult, err error) {
	hash := HashRefreshToken(presented)
	row, err := FindByHash(ctx, q, hash)
	if errors.Is(err, ErrNotFound) {
		return "", RotationResult{}, ErrRefreshTokenInvalid
	}
	if err != nil {
		return "", RotationResult{}, err
	}

	if row.RevokedAt != nil {
		n, revokeErr := RevokeChain(ctx, q, row.ID)
		if revokeErr != nil {
			return "", RotationResult{}, fmt.Errorf("sessions: revoking reused token's family: %w", revokeErr)
		}
		return "", RotationResult{UserID: row.UserID, FamilyRevoked: n}, ErrRefreshTokenReused
	}

	if time.Now().After(row.ExpiresAt) {
		return "", RotationResult{UserID: row.UserID}, ErrRefreshTokenExpired
	}

	newPlaintext, newHash, err := NewRefreshToken()
	if err != nil {
		return "", RotationResult{}, fmt.Errorf("sessions: generating new refresh token: %w", err)
	}
	if err := Revoke(ctx, q, row.ID); err != nil {
		return "", RotationResult{}, err
	}
	newID := uuid.New()
	if err := Insert(ctx, q, NewToken{
		ID:        newID,
		UserID:    row.UserID,
		TokenHash: newHash,
		ParentID:  &row.ID,
		ExpiresAt: time.Now().Add(RefreshTokenLifetime),
	}); err != nil {
		return "", RotationResult{}, err
	}

	return newPlaintext, RotationResult{UserID: row.UserID}, nil
}
