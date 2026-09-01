// Package apierror is the one error shape every REST handler in this
// gateway writes — pages.md §2 and auth.md §2's shared status-translation
// table, implemented once instead of duplicated per handler package.
package apierror

import (
	"encoding/json"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Body is the one error response shape — "the client has exactly one
// branch to handle" (pages.md §2).
type Body struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// WriteGRPCStatus translates err (expected to carry a gRPC status, as
// every call through a generated client does) into pages.md/auth.md §2's
// HTTP status + error code, and writes Body as the response.
func WriteGRPCStatus(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		WriteJSON(w, http.StatusInternalServerError, Body{Error: "internal_error", Message: "internal error"})
		return
	}

	httpStatus, code := httpStatusFor(st.Code())
	WriteJSON(w, httpStatus, Body{Error: code, Message: st.Message()})
}

// WriteBadRequest is for errors this gateway detects itself before ever
// calling a backend — a malformed path parameter or request body, which
// has no gRPC status to translate.
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteJSON(w, http.StatusUnprocessableEntity, Body{Error: "validation_failed", Message: message})
}

// httpStatusFor is pages.md §2's table, plus auth.md §2's addition of
// Unauthenticated (pages.md's own services never return it on a path this
// gateway calls, so its table omits it — auth-service returns it
// constantly).
func httpStatusFor(c codes.Code) (int, string) {
	switch c {
	case codes.Unauthenticated:
		return http.StatusUnauthorized, "unauthenticated"
	case codes.InvalidArgument:
		return http.StatusUnprocessableEntity, "validation_failed"
	case codes.NotFound:
		return http.StatusNotFound, "not_found"
	// Added with v3.1.0 (ADR-013). Nothing in this repo could return
	// PERMISSION_DENIED before roles existed, so it fell through to a 500 —
	// which turned "you may not do that" into "the server is broken", the
	// most misleading direction that mistake can go.
	case codes.PermissionDenied:
		return http.StatusForbidden, "forbidden"
	case codes.FailedPrecondition:
		return http.StatusConflict, "conflict"
	case codes.AlreadyExists:
		return http.StatusConflict, "conflict"
	case codes.Unavailable:
		return http.StatusServiceUnavailable, "unavailable"
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout, "timeout"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

// WriteJSON writes body as a JSON response with status — the one
// implementation every REST handler package in this gateway calls,
// rather than each declaring its own byte-for-byte-identical copy
// (pagesrest and authrest both used to; CLAUDE.md's own "duplicate on
// the second use, extract on the third" rule).
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
