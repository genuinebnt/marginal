package keys

import (
	"encoding/base64"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigningKeyKidMatchesVerificationKey(t *testing.T) {
	store, err := NewInMemoryStore()
	require.NoError(t, err)

	private, kid := store.SigningKey()
	public, ok := store.VerificationKey(kid)
	require.True(t, ok)
	assert.True(t, public.Equal(&private.PublicKey))
}

func TestVerificationKeyRejectsUnknownKid(t *testing.T) {
	store, err := NewInMemoryStore()
	require.NoError(t, err)

	_, ok := store.VerificationKey("some-other-kid")
	assert.False(t, ok)
}

func TestJWKSEncodesTheSamePublicKeyTheSigningKeyUses(t *testing.T) {
	store, err := NewInMemoryStore()
	require.NoError(t, err)

	_, kid := store.SigningKey()
	jwks := store.JWKS()
	require.Len(t, jwks.Keys, 1)
	jwk := jwks.Keys[0]

	assert.Equal(t, kid, jwk.Kid)
	assert.Equal(t, "RSA", jwk.Kty)
	assert.Equal(t, "RS256", jwk.Alg)
	assert.Equal(t, "sig", jwk.Use)

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	require.NoError(t, err)
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	require.NoError(t, err)

	public, ok := store.VerificationKey(kid)
	require.True(t, ok)
	assert.Equal(t, public.N, new(big.Int).SetBytes(nBytes))
	assert.Equal(t, public.E, int(new(big.Int).SetBytes(eBytes).Int64()))
}
