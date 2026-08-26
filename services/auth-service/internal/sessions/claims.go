// Package sessions owns the lifecycle of a login — issuing, refreshing,
// and revoking tokens — not just a data type (matches
// docs/architecture/lld/auth-service.md §5's "why sessions/ and not
// tokens/").
package sessions

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"marginal/auth-service/internal/domain"
	"marginal/auth-service/internal/keys"
)

// AccessTokenLifetime and ClockSkewLeeway are docs/api/auth.md's decided
// gaps (§2).
const (
	AccessTokenLifetime = 15 * time.Minute
	ClockSkewLeeway     = 60 * time.Second
)

// Claims is short-lived, RS256-signed, never stored server-side — the
// gateway verifies it locally against the cached public key, no
// per-request RPC (ADR-007). kid lives in the JWT header, not here.
type Claims struct {
	Sub domain.UserID
	Jti domain.Jti
	Exp time.Time
	Iat time.Time
	Nbf time.Time
}

var ErrInvalidToken = errors.New("sessions: invalid access token")

// Issue signs a new access token for sub, returning the token string and
// the jti it was issued under (the blocklist key, if it's ever revoked
// before expiry).
func Issue(store keys.Store, sub domain.UserID) (string, domain.Jti, error) {
	now := time.Now().UTC()
	jti := domain.NewJti()

	claims := jwt.RegisteredClaims{
		Subject:   sub.String(),
		ID:        jti.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenLifetime)),
	}

	private, kid := store.SigningKey()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	signed, err := token.SignedString(private)
	if err != nil {
		return "", domain.Jti{}, fmt.Errorf("sessions: signing token: %w", err)
	}
	return signed, jti, nil
}

// Verify checks tokenString's signature, algorithm, and standard claims.
// The Validation is constructed explicitly, field by field — the LLD's
// note that a library's Default is a security decision inherited silently
// otherwise. alg is pinned to RS256 here, never read from the token: the
// classic JWT vulnerability is a verifier that trusts the token's own
// header for which algorithm to use.
func Verify(store keys.Store, tokenString string) (Claims, error) {
	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims,
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			pub, ok := store.VerificationKey(kid)
			if !ok {
				return nil, fmt.Errorf("sessions: unknown kid %q", kid)
			}
			return pub, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithLeeway(ClockSkewLeeway),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || !token.Valid {
		return Claims{}, ErrInvalidToken
	}

	sub, err := domain.ParseUserID(claims.Subject)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	jti, err := domain.ParseJti(claims.ID)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}

	return Claims{
		Sub: sub,
		Jti: jti,
		Exp: claims.ExpiresAt.Time,
		Iat: claims.IssuedAt.Time,
		Nbf: claims.NotBefore.Time,
	}, nil
}
