// Package keys is RS256 keypair management and the JWKS document
// (docs/architecture/lld/auth-service.md §6). Asymmetric on purpose: the
// gateway verifies with a public key it caches, so a compromised gateway
// can never mint tokens — HS256 would require sharing the signing secret
// with every verifier.
package keys

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"github.com/google/uuid"
)

// Store is the seam a real KMS-backed implementation would sit behind
// later — small, declared at its point of use. VerificationKeys is
// plural because during a rotation two keys are valid at once: tokens
// signed with the old kid must keep verifying until they expire (LLD §6's
// five-step sequence — not implemented by this in-memory Store, which
// only ever holds one key for the process lifetime; see docs/api/auth.md
// § Deferred).
type Store interface {
	SigningKey() (key *rsa.PrivateKey, kid string)
	VerificationKey(kid string) (*rsa.PublicKey, bool)
	JWKS() JWKS
}

// InMemoryStore generates one RSA keypair at process start. Good enough
// for this track — a real deployment would load the private key from
// Secret Manager (docs/architecture/lld/auth-service.md §11.1) and never
// generate one at random per process.
type InMemoryStore struct {
	kid     string
	private *rsa.PrivateKey
	public  *rsa.PublicKey
}

func NewInMemoryStore() (*InMemoryStore, error) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("keys: generating RSA key: %w", err)
	}
	return &InMemoryStore{
		kid:     uuid.NewString(),
		private: private,
		public:  &private.PublicKey,
	}, nil
}

func (s *InMemoryStore) SigningKey() (*rsa.PrivateKey, string) { return s.private, s.kid }

func (s *InMemoryStore) VerificationKey(kid string) (*rsa.PublicKey, bool) {
	if kid != s.kid {
		return nil, false
	}
	return s.public, true
}

func (s *InMemoryStore) JWKS() JWKS {
	return JWKS{Keys: []JWK{jwkFromRSAPublicKey(s.kid, s.public)}}
}
