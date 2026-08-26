package sessions

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/auth-service/internal/domain"
	"marginal/auth-service/internal/keys"
)

func mustStore(t *testing.T) keys.Store {
	t.Helper()
	store, err := keys.NewInMemoryStore()
	require.NoError(t, err)
	return store
}

func TestIssueVerifyRoundTrip(t *testing.T) {
	store := mustStore(t)
	userID := domain.NewUserID()

	token, jti, err := Issue(store, userID)
	require.NoError(t, err)

	claims, err := Verify(store, token)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.Sub)
	assert.Equal(t, jti, claims.Jti)
}

func TestVerifyRejectsTokenFromAnUnknownKey(t *testing.T) {
	store := mustStore(t)
	other := mustStore(t) // a different keypair/kid entirely

	token, _, err := Issue(other, domain.NewUserID())
	require.NoError(t, err)

	_, err = Verify(store, token)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	store := mustStore(t)
	private, kid := store.SigningKey()

	claims := jwt.RegisteredClaims{
		Subject:   domain.NewUserID().String(),
		ID:        domain.NewJti().String(),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)), // expired well outside any leeway
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(private)
	require.NoError(t, err)

	_, err = Verify(store, signed)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// TestVerifyRejectsAlgorithmConfusion is the classic JWT vulnerability
// (docs/architecture/lld/auth-service.md §9): a verifier that reads the
// algorithm from the token's own header, rather than pinning it, can be
// tricked into treating an attacker-chosen key as an HMAC secret. Signing
// with "none" must also fail.
func TestVerifyRejectsAlgorithmConfusion(t *testing.T) {
	store := mustStore(t)

	claims := jwt.RegisteredClaims{
		Subject:   domain.NewUserID().String(),
		ID:        domain.NewJti().String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenLifetime)),
	}

	none := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = Verify(store, signed)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// TestVerifyRejectsHS256SignedWithThePublicKeyBytes is the named attack
// itself, not just the "none" variant above: if a verifier ever read alg
// from the token and dispatched on it, signing an HS256 token using the
// RSA public key's own bytes as the HMAC secret would verify successfully
// against a keyfunc that just returns "the key for this kid" — because
// jwt.WithValidMethods pins RS256 before the keyfunc callback is ever
// consulted for this token, it must be rejected regardless.
func TestVerifyRejectsHS256SignedWithThePublicKeyBytes(t *testing.T) {
	store := mustStore(t)
	private, kid := store.SigningKey()
	pubDER := x509.MarshalPKCS1PublicKey(&private.PublicKey)

	claims := jwt.RegisteredClaims{
		Subject:   domain.NewUserID().String(),
		ID:        domain.NewJti().String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenLifetime)),
	}
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	forged.Header["kid"] = kid
	signed, err := forged.SignedString(pubDER)
	require.NoError(t, err)

	_, err = Verify(store, signed)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestVerifyRejectsGarbage(t *testing.T) {
	store := mustStore(t)
	_, err := Verify(store, "not.a.jwt")
	assert.ErrorIs(t, err, ErrInvalidToken)
}
