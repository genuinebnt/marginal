package authverify

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type keypair struct {
	private *rsa.PrivateKey
	kid     string
}

func newKeypair(t *testing.T, kid string) keypair {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	return keypair{private: k, kid: kid}
}

// jwksServer serves the keys it is given, counting how many times it is
// asked — the count is what several tests below are actually about.
func jwksServer(t *testing.T, pairs ...keypair) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		keys := make([]map[string]string, 0, len(pairs))
		for _, p := range pairs {
			keys = append(keys, map[string]string{
				"kty": "RSA", "use": "sig", "alg": "RS256", "kid": p.kid,
				"n": base64.RawURLEncoding.EncodeToString(p.private.PublicKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.private.PublicKey.E)).Bytes()),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func sign(t *testing.T, p keypair, claims jwt.RegisteredClaims, method jwt.SigningMethod, kidHeader any) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	if kidHeader != nil {
		tok.Header["kid"] = kidHeader
	}
	var (
		s   string
		err error
	)
	if method == jwt.SigningMethodNone {
		s, err = tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	} else {
		s, err = tok.SignedString(p.private)
	}
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	return s
}

func validClaims(sub uuid.UUID) jwt.RegisteredClaims {
	now := time.Now().UTC()
	return jwt.RegisteredClaims{
		Subject:   sub.String(),
		ID:        uuid.NewString(),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
	}
}

func TestAGenuineTokenYieldsItsSubject(t *testing.T) {
	kp := newKeypair(t, "k1")
	srv, _ := jwksServer(t, kp)
	v := New(srv.URL)

	sub := uuid.New()
	got, err := v.Subject(context.Background(), sign(t, kp, validClaims(sub), jwt.SigningMethodRS256, "k1"))
	if err != nil {
		t.Fatalf("Subject: %v", err)
	}
	if got != sub {
		t.Fatalf("subject = %s, want %s", got, sub)
	}
}

// The classic JWT vulnerability: a verifier that reads the token's own
// header to decide how to check it. An attacker re-signs with alg:none, or
// with HMAC using the public key as the secret, and the verifier obliges.
func TestATokenThatNamesItsOwnAlgorithmIsRefused(t *testing.T) {
	kp := newKeypair(t, "k1")
	srv, _ := jwksServer(t, kp)
	v := New(srv.URL)
	sub := uuid.New()

	none := sign(t, kp, validClaims(sub), jwt.SigningMethodNone, "k1")
	if _, err := v.Subject(context.Background(), none); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("alg:none accepted (err = %v)", err)
	}

	// HS256 signed with something the attacker controls.
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims(sub))
	hs.Header["kid"] = "k1"
	forged, err := hs.SignedString([]byte("public-knowledge"))
	if err != nil {
		t.Fatalf("signing hs256: %v", err)
	}
	if _, err := v.Subject(context.Background(), forged); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("HS256 accepted (err = %v)", err)
	}
}

func TestATokenSignedByAnotherKeyIsRefused(t *testing.T) {
	real, attacker := newKeypair(t, "k1"), newKeypair(t, "k1")
	srv, _ := jwksServer(t, real)
	v := New(srv.URL)

	// Same kid, different key — the signature must fail even though the
	// header names a key this verifier really does know.
	forged := sign(t, attacker, validClaims(uuid.New()), jwt.SigningMethodRS256, "k1")
	if _, err := v.Subject(context.Background(), forged); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("token signed by an unknown key accepted (err = %v)", err)
	}
}

func TestAnExpiredTokenIsRefused(t *testing.T) {
	kp := newKeypair(t, "k1")
	srv, _ := jwksServer(t, kp)
	v := New(srv.URL)

	old := time.Now().UTC().Add(-2 * time.Hour)
	claims := jwt.RegisteredClaims{
		Subject:   uuid.NewString(),
		ID:        uuid.NewString(),
		IssuedAt:  jwt.NewNumericDate(old),
		NotBefore: jwt.NewNumericDate(old),
		ExpiresAt: jwt.NewNumericDate(old.Add(15 * time.Minute)),
	}
	if _, err := v.Subject(context.Background(), sign(t, kp, claims, jwt.SigningMethodRS256, "k1")); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired token accepted (err = %v)", err)
	}
}

// A token with no expiry never stops working. jwt.WithExpirationRequired is
// what makes that a rejection rather than a permanent credential.
func TestATokenWithNoExpiryIsRefused(t *testing.T) {
	kp := newKeypair(t, "k1")
	srv, _ := jwksServer(t, kp)
	v := New(srv.URL)

	claims := jwt.RegisteredClaims{Subject: uuid.NewString(), ID: uuid.NewString(),
		IssuedAt: jwt.NewNumericDate(time.Now().UTC())}
	if _, err := v.Subject(context.Background(), sign(t, kp, claims, jwt.SigningMethodRS256, "k1")); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("token with no exp accepted (err = %v)", err)
	}
}

func TestASubjectThatIsNotAUUIDIsRefused(t *testing.T) {
	kp := newKeypair(t, "k1")
	srv, _ := jwksServer(t, kp)
	v := New(srv.URL)

	claims := validClaims(uuid.New())
	claims.Subject = "admin"
	if _, err := v.Subject(context.Background(), sign(t, kp, claims, jwt.SigningMethodRS256, "k1")); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("non-uuid subject accepted (err = %v)", err)
	}
}

// An unknown kid must cost at most ONE fetch, however many tokens carry it.
// Otherwise a stream of tokens with random kids turns this service into a
// load generator pointed at auth-service.
func TestAFloodOfUnknownKidsCausesOneRefreshNotOnePerToken(t *testing.T) {
	kp := newKeypair(t, "k1")
	srv, hits := jwksServer(t, kp)
	v := New(srv.URL, WithCacheTTL(time.Hour))

	// Prime the cache with a real request.
	if _, err := v.Subject(context.Background(), sign(t, kp, validClaims(uuid.New()), jwt.SigningMethodRS256, "k1")); err != nil {
		t.Fatalf("priming: %v", err)
	}
	primed := hits.Load()

	other := newKeypair(t, "unknown")
	for i := 0; i < 25; i++ {
		_, _ = v.Subject(context.Background(), sign(t, other, validClaims(uuid.New()), jwt.SigningMethodRS256, "does-not-exist"))
	}
	if extra := hits.Load() - primed; extra > 1 {
		t.Fatalf("25 tokens with an unknown kid caused %d fetches, want at most 1", extra)
	}
}

// A cached key must keep working while auth-service is unreachable. The
// alternative is that a blip in one service logs everybody out of another.
func TestAKnownKeyStillVerifiesWhenTheJWKSIsUnreachable(t *testing.T) {
	kp := newKeypair(t, "k1")
	srv, _ := jwksServer(t, kp)
	now := time.Now().UTC()
	v := New(srv.URL, WithCacheTTL(time.Minute), WithClock(func() time.Time { return now }))

	sub := uuid.New()
	if _, err := v.Subject(context.Background(), sign(t, kp, validClaims(sub), jwt.SigningMethodRS256, "k1")); err != nil {
		t.Fatalf("priming: %v", err)
	}

	srv.Close()              // auth-service is gone
	now = now.Add(time.Hour) // and the cache has aged out

	got, err := v.Subject(context.Background(), sign(t, kp, validClaims(sub), jwt.SigningMethodRS256, "k1"))
	if err != nil {
		t.Fatalf("a known key stopped working when the JWKS was unreachable: %v", err)
	}
	if got != sub {
		t.Fatalf("subject = %s, want %s", got, sub)
	}
}

// An empty JWKS is far more likely to be a misconfiguration than a genuine
// "there are no keys" — adopting it would reject every token in flight.
func TestAnEmptyJWKSDoesNotEvictTheWorkingKey(t *testing.T) {
	kp := newKeypair(t, "k1")
	var serveEmpty atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveEmpty.Load() {
			_, _ = w.Write([]byte(`{"keys":[]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kp.kid,
			"n": base64.RawURLEncoding.EncodeToString(kp.private.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(kp.private.PublicKey.E)).Bytes()),
		}}})
	}))
	defer srv.Close()

	now := time.Now().UTC()
	v := New(srv.URL, WithCacheTTL(time.Minute), WithClock(func() time.Time { return now }))
	sub := uuid.New()
	if _, err := v.Subject(context.Background(), sign(t, kp, validClaims(sub), jwt.SigningMethodRS256, "k1")); err != nil {
		t.Fatalf("priming: %v", err)
	}

	serveEmpty.Store(true)
	now = now.Add(time.Hour)
	if _, err := v.Subject(context.Background(), sign(t, kp, validClaims(sub), jwt.SigningMethodRS256, "k1")); err != nil {
		t.Fatalf("an empty JWKS evicted a working key: %v", err)
	}
}

// Key rotation: auth-service signs with a new kid, and a verifier holding
// only the old one must pick the new key up rather than reject everybody —
// WITHOUT waiting out the cache TTL, since the cache being fresh is exactly
// why the new key is not in it.
//
// The clock advance is the guarantee, stated: an unknown kid may provoke a
// fetch at most once per minRefreshInterval, so a rotation is picked up
// within thirty seconds, not instantly. That bound is what stops a token
// generator from turning this verifier into a load generator aimed at
// auth-service (see the flood test above).
func TestANewSigningKeyIsPickedUpWithoutARestart(t *testing.T) {
	oldKey := newKeypair(t, "k1")
	newKey := newKeypair(t, "k2")
	var rotated atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pairs := []keypair{oldKey}
		if rotated.Load() {
			pairs = []keypair{oldKey, newKey}
		}
		keys := make([]map[string]string, 0, len(pairs))
		for _, p := range pairs {
			keys = append(keys, map[string]string{
				"kty": "RSA", "use": "sig", "alg": "RS256", "kid": p.kid,
				"n": base64.RawURLEncoding.EncodeToString(p.private.PublicKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.private.PublicKey.E)).Bytes()),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	defer srv.Close()

	now := time.Now().UTC()
	v := New(srv.URL, WithCacheTTL(time.Hour), WithClock(func() time.Time { return now }))
	if _, err := v.Subject(context.Background(), sign(t, oldKey, validClaims(uuid.New()), jwt.SigningMethodRS256, "k1")); err != nil {
		t.Fatalf("priming: %v", err)
	}

	rotated.Store(true)
	now = now.Add(minRefreshInterval + time.Second)
	sub := uuid.New()
	got, err := v.Subject(context.Background(), sign(t, newKey, validClaims(sub), jwt.SigningMethodRS256, "k2"))
	if err != nil {
		t.Fatalf("a rotated key was not picked up: %v", err)
	}
	if got != sub {
		t.Fatalf("subject = %s, want %s", got, sub)
	}
}

func TestBearerTokenParsing(t *testing.T) {
	for _, tc := range []struct{ header, want string }{
		{"Bearer abc.def.ghi", "abc.def.ghi"},
		{"bearer abc.def.ghi", "abc.def.ghi"}, // case-insensitive scheme, RFC 7235
		{"Bearer   spaced  ", "spaced"},
		{"", ""},
		{"Basic abc", ""},
		{"Bearer", ""},
		{"Bearer ", ""},
	} {
		if got := BearerToken(tc.header); got != tc.want {
			t.Errorf("BearerToken(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestAnEmptyTokenIsRefusedWithoutTouchingTheNetwork(t *testing.T) {
	srv, hits := jwksServer(t, newKeypair(t, "k1"))
	v := New(srv.URL)
	if _, err := v.Subject(context.Background(), "   "); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("empty token accepted (err = %v)", err)
	}
	if hits.Load() != 0 {
		t.Errorf("an empty token caused %d JWKS fetches; it should cost nothing", hits.Load())
	}
}
