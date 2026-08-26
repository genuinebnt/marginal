package sessions

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/google/uuid"

	"marginal/auth-service/internal/domain"
)

// RefreshTokenLifetime is docs/api/auth.md's decided gap (§2).
const RefreshTokenLifetime = 30 * 24 * time.Hour

// StoredToken is auth.refresh_tokens as read back — RevokedAt nil means
// active. The database stores TokenHash, never the plaintext (a database
// leak must not yield usable credentials, DATA_MODEL.md §3).
type StoredToken struct {
	ID        uuid.UUID
	UserID    domain.UserID
	TokenHash [32]byte
	ParentID  *uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// NewRefreshToken generates an opaque, cryptographically random token —
// never a JWT, never structured. Returns the plaintext to hand to the
// caller and the SHA-256 hash to store; SHA-256 is correct here and
// Argon2 would be wrong — a 256-bit random token has no entropy problem
// to solve, and per-refresh Argon2 would be a self-inflicted DoS
// (docs/architecture/lld/auth-service.md §9).
func NewRefreshToken() (plaintext string, hash [32]byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", [32]byte{}, err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(raw)
	hash = sha256.Sum256([]byte(plaintext))
	return plaintext, hash, nil
}

func HashRefreshToken(plaintext string) [32]byte {
	return sha256.Sum256([]byte(plaintext))
}
