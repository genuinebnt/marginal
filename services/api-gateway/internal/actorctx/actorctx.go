// Package actorctx forwards the VERIFIED actor identity from an incoming
// HTTP request onto the outgoing gRPC call's actor-id metadata, which is
// what document-service and auth-service both actually read.
//
// The identity comes from authmw, which derived it from a token's `sub`
// claim after checking the signature (ADR-013 §1). It used to come from the
// request's own X-Actor-Id header — a value the caller wrote, verified by
// nobody. That header is no longer read anywhere.
//
// Nothing downstream changed when this did, which was the point of routing
// it through one metadata key in the first place: services look at
// "actor-id", never at how it got set.
package actorctx

import (
	"context"
	"net/http"

	"google.golang.org/grpc/metadata"

	"marginal/api-gateway/internal/authmw"
)

// FromRequest returns a context carrying r's verified actor id as gRPC
// actor-id metadata, for a backend call made on r's behalf. A request with
// no verified subject still returns a valid context — the public routes
// (register, login, refresh) genuinely have no actor, and a route that
// needs one gets the backend's own UNAUTHENTICATED rejection, translated by
// apierror the same as any other backend error.
func FromRequest(r *http.Request) context.Context {
	sub, ok := authmw.Subject(r.Context())
	if !ok {
		return r.Context()
	}
	return metadata.AppendToOutgoingContext(r.Context(), "actor-id", sub.String())
}
