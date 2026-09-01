// Package authverify turns a bearer token into a verified actor id.
//
// It exists as its own module because two services need it and neither can
// import the other's internal/: api-gateway verifies the tokens on REST
// requests, and collaboration-service verifies the ticket on a WebSocket
// handshake — a socket that is reached DIRECTLY, never through the gateway
// (docs/api/collaboration.md). That second entry point is the whole reason
// this cannot live in the gateway: any scheme where only the gateway checks
// is bypassed by connecting to the socket instead, which is the path every
// mutation takes (ADR-013 §1).
//
// Verification is LOCAL. auth-service signs; everyone else verifies against
// its published JWKS. Asking auth-service to validate each token would put a
// second network hop in front of every request and make editing depend on
// auth-service being up (ADR-007's own argument, applied to tokens).
//
// What this package deliberately does NOT do:
//
//   - No revocation check. A token is valid until it expires (15 minutes,
//     sessions.AccessTokenLifetime). auth-service keeps a jti blocklist for
//     early revocation, and consulting it would be the per-request RPC this
//     package exists to avoid. The window is stated in ADR-013 rather than
//     closed.
//   - No role lookup. This answers "who are you", never "what may you do".
//     Those are different questions with different lifetimes, and merging
//     them is how a cache ends up serving a stale permission.
package authverify

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ErrUnauthenticated is every failure this package can have: a missing
// token, a malformed one, a bad signature, an expired one, an unknown key.
//
// One error on purpose. The caller turns it into a 401 and must not tell a
// client WHICH of those it was — "expired" versus "bad signature" versus
// "unknown key" is a description of the server's key state, and answering it
// for free is how an attacker maps that state.
var ErrUnauthenticated = errors.New("authverify: unauthenticated")

// minRefreshInterval bounds how often an unknown kid may provoke a JWKS
// fetch. Rotation has to be picked up promptly — a new signing key should
// not wait out the cache TTL — but "fetch whenever a kid is unknown" lets
// anyone with a token generator point this service at auth-service. Thirty
// seconds is short enough that a rotation is invisible and long enough that
// a flood costs two requests a minute.
const minRefreshInterval = 30 * time.Second

// ClockSkewLeeway matches auth-service's own (sessions.ClockSkewLeeway). It
// has to: a token this verifier rejects as not-yet-valid while the issuer
// considers it live is a login that fails for one machine and works for
// another, which is the least debuggable class of bug there is.
const ClockSkewLeeway = 60 * time.Second

// Verifier holds the cached JWKS and verifies tokens against it. Safe for
// concurrent use; one per process.
type Verifier struct {
	jwksURL string
	client  *http.Client
	ttl     time.Duration
	now     func() time.Time

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	// attemptedAt is when a fetch was last TRIED, successful or not.
	//
	// Separate from fetchedAt because the two answer different questions.
	// fetchedAt bounds how stale the cache may be; attemptedAt bounds how
	// often an unknown kid may provoke a fetch. Using fetchedAt for both
	// meant a fresh cache refused to refresh at all — so a rotated signing
	// key was rejected until the TTL ran out, which is an outage for
	// everyone holding a new token.
	attemptedAt time.Time
	// refetching is held across a refresh so a burst of requests carrying an
	// unknown kid produces ONE fetch, not one per request. Without it an
	// attacker sends tokens with random kids and every one becomes an
	// outbound request to auth-service — a trivially cheap way to point this
	// service at that one.
	refetching sync.Mutex
}

type Option func(*Verifier)

// WithHTTPClient replaces the client used to fetch the JWKS.
func WithHTTPClient(c *http.Client) Option { return func(v *Verifier) { v.client = c } }

// WithCacheTTL sets how long a fetched JWKS is trusted before a refresh.
func WithCacheTTL(d time.Duration) Option { return func(v *Verifier) { v.ttl = d } }

// WithClock replaces time.Now, for tests that need an expired token without
// waiting fifteen minutes for one.
func WithClock(f func() time.Time) Option { return func(v *Verifier) { v.now = f } }

// New returns a Verifier reading auth-service's JWKS from jwksURL
// (e.g. "http://auth-service:8006/.well-known/jwks.json").
//
// It does NOT fetch on construction. A service that cannot start because
// auth-service is briefly unreachable is a worse failure than one that
// starts and refuses tokens until the key arrives — the second recovers by
// itself.
func New(jwksURL string, opts ...Option) *Verifier {
	v := &Verifier{
		jwksURL: jwksURL,
		client:  &http.Client{Timeout: 5 * time.Second},
		ttl:     10 * time.Minute,
		now:     func() time.Time { return time.Now().UTC() },
		keys:    map[string]*rsa.PublicKey{},
	}
	for _, o := range opts {
		o(v)
	}
	return v
}

// BearerToken extracts the token from an Authorization header value.
// Returns "" when the header is absent or is not a Bearer credential.
func BearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// Subject verifies token and returns the actor id from its `sub` claim.
//
// The algorithm is pinned to RS256 and never read from the token's own
// header for key selection — a verifier that trusts the token to say how it
// should be checked is the classic JWT vulnerability, and `alg: none` is the
// classic exploit of it.
func (v *Verifier) Subject(ctx context.Context, token string) (uuid.UUID, error) {
	if strings.TrimSpace(token) == "" {
		return uuid.Nil, ErrUnauthenticated
	}

	var claims jwt.RegisteredClaims
	parsed, err := jwt.ParseWithClaims(token, &claims,
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			return v.keyFor(ctx, kid)
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithLeeway(ClockSkewLeeway),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || !parsed.Valid {
		return uuid.Nil, ErrUnauthenticated
	}

	sub, err := uuid.Parse(claims.Subject)
	if err != nil || sub == uuid.Nil {
		return uuid.Nil, ErrUnauthenticated
	}
	return sub, nil
}

// keyFor returns the public key for kid, refreshing the JWKS at most once if
// the kid is unknown or the cache has aged out.
func (v *Verifier) keyFor(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, ErrUnauthenticated
	}

	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := v.now().Sub(v.fetchedAt) < v.ttl
	v.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}

	// An UNKNOWN kid is the rotation case: fetch even though the cache is
	// within its TTL, because the cache being fresh is exactly why the new
	// key is not in it. Rate-limited rather than unconditional.
	//
	// A known kid with a stale cache is still usable if the refresh fails:
	// auth-service being briefly unreachable should not log everybody out.
	if err := v.refresh(ctx, !ok); err != nil && !ok {
		return nil, ErrUnauthenticated
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, ErrUnauthenticated
	}
	return key, nil
}

// refresh re-fetches the JWKS. unknownKid says this was provoked by a token
// naming a key the cache does not have, which is allowed to bypass the TTL
// (see minRefreshInterval) — a rotation must not wait out the cache.
func (v *Verifier) refresh(ctx context.Context, unknownKid bool) error {
	v.refetching.Lock()
	defer v.refetching.Unlock()

	// Someone else refreshed while this call waited for the lock. Doing it
	// again would be the stampede the lock exists to prevent.
	v.mu.RLock()
	fresh := v.now().Sub(v.fetchedAt) < v.ttl
	recentlyTried := v.now().Sub(v.attemptedAt) < minRefreshInterval
	v.mu.RUnlock()
	if unknownKid {
		if recentlyTried {
			return errors.New("authverify: jwks refresh rate-limited")
		}
	} else if fresh {
		return nil
	}

	v.mu.Lock()
	v.attemptedAt = v.now()
	v.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("authverify: building jwks request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("authverify: fetching jwks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authverify: jwks returned %d", resp.StatusCode)
	}

	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("authverify: decoding jwks: %w", err)
	}

	next := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		// Only RSA signing keys. A JWKS may legitimately carry other kinds,
		// and silently coercing one into an RSA key is how a verifier ends
		// up trusting something it cannot actually check.
		if k.Kty != "RSA" || (k.Alg != "" && k.Alg != "RS256") || k.Kid == "" {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		next[k.Kid] = pub
	}
	if len(next) == 0 {
		// Keep whatever is cached. An empty JWKS is far more likely to be a
		// misconfiguration than a genuine "there are no keys", and adopting
		// it would reject every token in flight.
		return errors.New("authverify: jwks contained no usable RSA keys")
	}

	v.mu.Lock()
	v.keys = next
	v.fetchedAt = v.now()
	v.mu.Unlock()
	return nil
}

func rsaPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	if len(n) == 0 || len(e) == 0 || len(e) > 8 {
		return nil, errors.New("authverify: malformed RSA key material")
	}
	exponent := 0
	for _, b := range e {
		exponent = exponent<<8 | int(b)
	}
	if exponent <= 0 {
		return nil, errors.New("authverify: malformed RSA exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}, nil
}
