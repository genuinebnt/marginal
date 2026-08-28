// Package diagnosticsrest is api-gateway's REST↔gRPC shim for
// DiagnosticsService — the same "minimum to reach portable code"
// convention pagesrest/graphrest already use, translating
// docs/api/diagnostics.md's REST contract onto diagnostics-service.
package diagnosticsrest

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"marginal/api-gateway/internal/actorctx"
	"marginal/api-gateway/internal/apierror"
	diagnosticsv1 "marginal/diagnostics-service/genproto/diagnosticsv1"
)

// Handler holds the one thing every route needs: a DiagnosticsService
// client.
type Handler struct {
	client diagnosticsv1.DiagnosticsServiceClient
}

func NewHandler(client diagnosticsv1.DiagnosticsServiceClient) *Handler {
	return &Handler{client: client}
}

// Mount registers docs/api/diagnostics.md §2's three routes on r.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/pages/{id}/diagnostics", h.analyzePage)
	r.Get("/facts", h.analyzeFacts)
	r.Get("/facts/{name}/stale", h.staleReferences)
}

func (h *Handler) analyzePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resp, err := h.client.AnalyzePage(actorctx.FromRequest(r), &diagnosticsv1.AnalyzePageRequest{PageId: id})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toAnalyzePageJSON(resp))
}

func (h *Handler) analyzeFacts(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.AnalyzeFacts(actorctx.FromRequest(r), &diagnosticsv1.AnalyzeFactsRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toAnalyzeFactsJSON(resp))
}

func (h *Handler) staleReferences(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	resp, err := h.client.StaleReferences(actorctx.FromRequest(r), &diagnosticsv1.StaleReferencesRequest{FactName: name})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toReferencesJSON(resp.GetReferences()))
}
