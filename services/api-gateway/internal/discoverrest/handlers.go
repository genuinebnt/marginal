// Package discoverrest is api-gateway's REST↔gRPC shim for DiscoverService —
// § 09's "what is near this page by meaning". Same convention as graphrest.
package discoverrest

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"marginal/api-gateway/internal/actorctx"
	"marginal/api-gateway/internal/apierror"
	documentv1 "marginal/document-service/genproto/documentv1"
)

type Handler struct {
	client documentv1.DiscoverServiceClient
}

func NewHandler(client documentv1.DiscoverServiceClient) *Handler {
	return &Handler{client: client}
}

// Mount registers docs/api/discover.md §2's one route on r.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/discover/{id}", h.near)
}

func (h *Handler) near(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	k, _ := strconv.Atoi(q.Get("k"))

	resp, err := h.client.Near(actorctx.FromRequest(r), &documentv1.NearRequest{
		SourcePageId: chi.URLParam(r, "id"),
		K:            int32(k),
		Topics:       csv(q.Get("topics")),
		Tags:         csv(q.Get("tags")),
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toNearJSON(resp))
}

// csv splits a comma-separated query parameter, dropping blanks. An empty
// parameter must produce an EMPTY slice, not a slice holding one empty
// string — the latter reads downstream as "restrict to the topic named ”",
// which matches nothing and silently returns no results.
func csv(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
