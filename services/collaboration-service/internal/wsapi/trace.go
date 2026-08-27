package wsapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"marginal/collaboration-service/internal/opstore"
	"marginal/collaboration-service/internal/session"
)

// traceResponse is what GET /collab/pages/{id}/trace returns —
// docs/api/collaboration.md §5's own shape, one entry per confirmed op.
type traceResponse struct {
	Steps []session.TraceStep `json:"steps"`
}

// NewTraceHandler serves a page's whole confirmed op log, replayed for
// real and law-checked (session.Trace) — a plain request/response GET,
// not a WebSocket, since it's "give me the whole replay once," not a
// live session. Read-only: never touches a live Session, so it's safe to
// call for a page someone else has open right now.
func NewTraceHandler(repo opstore.Repo, serverActor string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pageID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid page id", http.StatusBadRequest)
			return
		}

		steps, err := session.Trace(r.Context(), pageID, repo, serverActor)
		if err != nil {
			slog.Error("wsapi: trace failed", "page_id", pageID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(traceResponse{Steps: steps}); err != nil {
			slog.Error("wsapi: encoding trace response failed", "page_id", pageID, "err", err)
		}
	}
}
