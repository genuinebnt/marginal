package wsapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/opstore"
	"marginal/collaboration-service/internal/palimpsest"
)

// palimpsestResponse is what GET /collab/pages/{id}/blocks/{blockId}/palimpsest
// returns — docs/api/collaboration.md §6's own shape. CurrentStep is the
// confirmed log's own last step index (len(confirmed)-1, or -1 for an
// empty log) — the same axis GET .../trace's "steps" array is indexed
// by, so a client can drive one scrubber against both endpoints.
type palimpsestResponse struct {
	Chars       []palimpsest.Char `json:"chars"`
	CurrentStep int               `json:"current_step"`
}

// NewPalimpsestHandler serves one block's whole tombstoned character
// history (internal/palimpsest.Build) — history.html's "the palimpsest
// paragraph is a real persistent sequence," made real. Plain HTTP, not a
// WebSocket, and read-only for the same reason NewTraceHandler is: give
// me the whole replay once, safe to call for a page someone else has
// open right now, never touches a live Session. serverActor must match
// NewTraceHandler/session.NewManager's own — see Build's own doc
// comment for why a mismatch breaks anchor resolution.
func NewPalimpsestHandler(repo opstore.Repo, serverActor string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pageID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid page id", http.StatusBadRequest)
			return
		}
		rawBlockID, err := uuid.Parse(r.PathValue("blockId"))
		if err != nil {
			http.Error(w, "invalid block id", http.StatusBadRequest)
			return
		}
		blockID := documentcore.BlockID(rawBlockID)

		confirmed, err := repo.ListForPage(r.Context(), pageID)
		if err != nil {
			slog.Error("wsapi: palimpsest: listing confirmed ops failed", "page_id", pageID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		chars, err := palimpsest.Build(confirmed, blockID, serverActor)
		if err != nil {
			slog.Error("wsapi: palimpsest: build failed", "page_id", pageID, "block_id", blockID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if chars == nil {
			chars = []palimpsest.Char{} // empty, never null — this repo's own wire convention (docs/api/diagnostics.md)
		}

		w.Header().Set("Content-Type", "application/json")
		resp := palimpsestResponse{Chars: chars, CurrentStep: len(confirmed) - 1}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("wsapi: encoding palimpsest response failed", "page_id", pageID, "block_id", blockID, "err", err)
		}
	}
}
