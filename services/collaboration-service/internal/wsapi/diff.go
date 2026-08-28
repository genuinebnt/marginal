package wsapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/opstore"
	"marginal/collaboration-service/internal/pageop"
	"marginal/collaboration-service/internal/session"
)

// moveWire is one MoveBlock op found strictly after "from" and at or
// before "to" — docs/ui-mockups/diff.html's own "block-level MOVE
// detection, which a flat text diff cannot express." No algorithm here:
// MoveBlock already carries From/To (RFC-002 §3, documentcore's own
// six-variant block ISA), so detecting a move between two revisions is a
// filter over the confirmed log Trace already replayed, not a second
// computation.
type moveWire struct {
	BlockID    documentcore.BlockID  `json:"block_id"`
	FromParent *documentcore.BlockID `json:"from_parent"`
	From       *documentcore.BlockID `json:"from"`
	ToParent   *documentcore.BlockID `json:"to_parent"`
	To         *documentcore.BlockID `json:"to"`
	Step       int                   `json:"step"`
}

// diffResponse is what GET /collab/pages/{id}/diff?from=N&to=M returns —
// docs/api/collaboration.md §7's own shape. Before/After are exactly
// Trace's own steps[from].After/steps[to].After — the same "the client
// draws what Go already computed, never re-runs apply() itself"
// principle §5 already states, applied to picking two existing entries
// instead of one.
type diffResponse struct {
	Before session.Snapshot `json:"before"`
	After  session.Snapshot `json:"after"`
	Moves  []moveWire       `json:"moves"`
}

// NewDiffHandler serves two revisions' snapshots plus every block move
// between them — diff.html's data source. The LCS text diff itself is
// NOT computed here: it runs client-side, compiled to wasm
// (services/textdiff via document-service/cmd/diffwasm), because
// diff.html's own "token granularity switching (word ↔ character),
// recomputed live" needs interactive response to a toggle — this
// endpoint only ever hands over the two already-computed document
// states a client picks one block's text out of.
func NewDiffHandler(repo opstore.Repo, serverActor string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pageID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid page id", http.StatusBadRequest)
			return
		}
		from, err := strconv.Atoi(r.URL.Query().Get("from"))
		if err != nil {
			http.Error(w, "invalid or missing from", http.StatusBadRequest)
			return
		}
		to, err := strconv.Atoi(r.URL.Query().Get("to"))
		if err != nil {
			http.Error(w, "invalid or missing to", http.StatusBadRequest)
			return
		}

		steps, err := session.Trace(r.Context(), pageID, repo, serverActor)
		if err != nil {
			slog.Error("wsapi: diff: trace failed", "page_id", pageID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if from < 0 || from >= len(steps) || to < 0 || to >= len(steps) || from > to {
			http.Error(w, "from/to out of range or from > to", http.StatusBadRequest)
			return
		}

		moves := []moveWire{}
		for i := from + 1; i <= to; i++ {
			blockOp, ok := steps[i].Op.Op.(pageop.Block)
			if !ok {
				continue
			}
			mb, ok := blockOp.Op.(documentcore.MoveBlock)
			if !ok {
				continue
			}
			moves = append(moves, moveWire{
				BlockID: mb.ID, FromParent: mb.FromParent, From: mb.From,
				ToParent: mb.ToParent, To: mb.To, Step: i,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		resp := diffResponse{Before: steps[from].After, After: steps[to].After, Moves: moves}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("wsapi: encoding diff response failed", "page_id", pageID, "err", err)
		}
	}
}
