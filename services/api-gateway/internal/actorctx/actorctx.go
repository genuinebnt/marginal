// Package actorctx forwards the temporary actor-identity stand-in
// (pages.md/auth.md's "Actor identity (temporary, until a gateway
// exists)") from an incoming HTTP request's X-Actor-Id header onto the
// outgoing gRPC call's actor-id metadata, which is what document-service
// and auth-service both actually read.
//
// This gateway is that stand-in's replacement transport, not its fix —
// it still isn't verifying anything, only relaying a header a browser
// client sends unauthenticated. Real verification (a session cookie, a
// verified JWT) belongs here once it exists; nothing downstream should
// need to change when it does, since they already only look at the
// actor-id metadata key, not how it got set.
package actorctx

import (
	"context"
	"net/http"

	"google.golang.org/grpc/metadata"
)

// FromRequest returns a context carrying r's X-Actor-Id header as gRPC
// actor-id metadata, for a backend call made on r's behalf. A missing
// header still returns a valid context — some routes (e.g. Authenticate,
// Refresh) don't require one; a route that does will get the backend's
// own UNAUTHENTICATED rejection, translated by apierror same as any other
// backend error.
func FromRequest(r *http.Request) context.Context {
	actorID := r.Header.Get("X-Actor-Id")
	if actorID == "" {
		return r.Context()
	}
	return metadata.AppendToOutgoingContext(r.Context(), "actor-id", actorID)
}
