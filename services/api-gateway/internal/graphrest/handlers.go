// Package graphrest is api-gateway's REST↔gRPC shim for GraphService —
// the same "minimum to reach portable code" convention pagesrest already
// uses, translating docs/api/graph.md's REST contract onto
// document-service's GraphService.
package graphrest

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"marginal/api-gateway/internal/actorctx"
	"marginal/api-gateway/internal/apierror"
	documentv1 "marginal/document-service/genproto/documentv1"
)

// Handler holds the one thing every route needs: a GraphService client.
type Handler struct {
	client documentv1.GraphServiceClient
}

func NewHandler(client documentv1.GraphServiceClient) *Handler { return &Handler{client: client} }

// Mount registers docs/api/graph.md §2's three routes on r.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/graph", h.getLinkGraph)
	r.Get("/graph/analysis", h.analyze)
	r.Get("/graph/neighborhood/{id}", h.neighborhood)
}

func (h *Handler) getLinkGraph(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.GetLinkGraph(actorctx.FromRequest(r), &documentv1.GetLinkGraphRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toLinkGraphJSON(resp))
}

func (h *Handler) analyze(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.AnalyzeGraph(actorctx.FromRequest(r), &documentv1.AnalyzeGraphRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toGraphAnalysisJSON(resp))
}

func (h *Handler) neighborhood(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resp, err := h.client.GraphNeighborhood(actorctx.FromRequest(r), &documentv1.GraphNeighborhoodRequest{SourcePageId: id})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toNeighborhoodJSON(resp))
}
