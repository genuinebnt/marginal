// Package authmw turns a bearer token into a verified actor id on the way
// in, and refuses the request when it cannot (ADR-013 §1).
//
// This is the gateway's ONLY job in the authorization story: it
// authenticates — establishes who is calling — and passes that identity
// down. It does not decide what the caller may do. Enforcement belongs to
// the service that owns the data, because collaboration-service's WebSocket
// is reached directly and never passes through here at all; a gateway that
// were the only checkpoint would be one connection away from irrelevant.
package authmw

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"marginal/api-gateway/internal/apierror"
	"marginal/authverify"
)

type ctxKey struct{}

// TokenVerifier is what turns a credential into an actor id — an interface
// declared at its point of use, this repo's rule for every external
// dependency. Concretely it is *authverify.Verifier; as an interface, a
// test can exercise the real middleware without standing up a JWKS server,
// which is what makes "the handler forwards the VERIFIED actor" testable at
// all. Mirrors wsapi.TokenVerifier, deliberately: the two entry points
// should not describe the same dependency two different ways.
type TokenVerifier interface {
	Subject(ctx context.Context, token string) (uuid.UUID, error)
}

// public is the exact set of routes that may be reached with no token.
//
// An ALLOWLIST, keyed by method and path, and deliberately not a prefix
// match or a "starts with /auth" rule: /auth also carries revoke and
// revoke-all, which act on a session and must prove they own it. A rule
// that lets an unauthenticated caller revoke sessions is one careless
// prefix away, and this shape makes adding a public route a decision
// somebody has to write down.
var public = map[string]bool{
	"POST /auth/register": true,
	"POST /auth/login":    true,
	// Refresh presents a refresh token in its body, which auth-service
	// verifies itself. It cannot require an access token: the entire reason
	// to call it is that yours has expired.
	"POST /auth/refresh": true,
	"GET /health":        true,
}

// Middleware verifies the Authorization header on every non-public route
// and stores the subject for actorctx to forward.
//
// X-Actor-Id is not consulted. Not "preferred less than the token" —
// ignored: a header read even when the token is absent is the same hole
// with an extra step, and it is the hole this whole slice exists to close.
func Middleware(v TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if public[r.Method+" "+r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			// CORS preflight carries no credentials by definition; rejecting
			// it would break every cross-origin call before the real request
			// is ever made.
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			sub, err := v.Subject(r.Context(), authverify.BearerToken(r.Header.Get("Authorization")))
			if err != nil {
				apierror.WriteJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "unauthenticated",
					// One message for every failure. Which of them it was —
					// expired, bad signature, unknown key — describes the
					// server's key state, and answering it for free is how
					// that state gets mapped.
					"message": "a valid access token is required",
				})
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sub)))
		})
	}
}

// Subject returns the verified actor id Middleware stored, and whether
// there was one. Absent only on a public route.
func Subject(ctx context.Context) (uuid.UUID, bool) {
	sub, ok := ctx.Value(ctxKey{}).(uuid.UUID)
	return sub, ok
}
