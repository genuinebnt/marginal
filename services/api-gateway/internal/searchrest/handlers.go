// Package searchrest is api-gateway's REST↔gRPC shim for SearchService —
// the same "minimum to reach portable code" convention pagesrest/
// graphrest already use, translating docs/api/search.md's REST contract
// onto document-service's SearchService.
package searchrest

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"marginal/api-gateway/internal/actorctx"
	"marginal/api-gateway/internal/apierror"
	documentv1 "marginal/document-service/genproto/documentv1"
)

// Handler holds the one thing every route needs: a SearchService client.
type Handler struct {
	client documentv1.SearchServiceClient
}

func NewHandler(client documentv1.SearchServiceClient) *Handler { return &Handler{client: client} }

// Mount registers docs/api/search.md §2's two routes on r.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/search", h.search)
	r.Get("/search/suggest", h.suggest)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	resp, err := h.client.Search(actorctx.FromRequest(r), &documentv1.SearchRequest{Query: query})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toSearchResponseJSON(resp))
}

func (h *Handler) suggest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	// A malformed or absent max_distance falls back to 0, which
	// SuggestTitles' own server-side default (2) then takes over from —
	// no client-facing error for a query-param the REST contract already
	// documents as optional.
	maxDistance, _ := strconv.Atoi(r.URL.Query().Get("max_distance"))
	resp, err := h.client.SuggestTitles(actorctx.FromRequest(r), &documentv1.SuggestTitlesRequest{Query: query, MaxDistance: int32(maxDistance)})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toSuggestResponseJSON(resp))
}
