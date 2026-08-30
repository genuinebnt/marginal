package wsapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	collabrepo "marginal/collaboration-service/internal/collabrepo/gen"
)

// statsResponse is what GET /collab/stats returns — § 16 PERF's
// QUEUE DEPTH panel, measured.
//
// Two numbers per queue rather than one, because a depth alone
// cannot tell a healthy burst from a stopped poller: 400 events
// draining in 200 ms and 3 events whose oldest has waited four
// minutes are opposite conditions with the smaller number on the
// wrong side.
type statsResponse struct {
	OutboxDepth        int64   `json:"outbox_depth"`
	OutboxOldestSeconds float64 `json:"outbox_oldest_seconds"`
	Ops                int64   `json:"ops"`
	Pages              int64   `json:"pages"`
	// LagSeconds is time since the newest op — on an idle instance
	// this is large and perfectly healthy, so the screen labels it
	// rather than colouring it red.
	LagSeconds float64 `json:"lag_seconds"`
}

// NewStatsHandler serves this instance's own queue depths.
//
// Reached directly rather than through api-gateway, the same
// convention every other collaboration-service debug endpoint
// follows (docs/api/collaboration.md) — the gateway maps the REST
// resource contracts, and this is an instance fact, not a resource.
//
// Read-only, two aggregate queries, no session state touched. It is
// safe to poll while people are editing, which is the only way it is
// useful.
func NewStatsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	q := collabrepo.New(pool)
	return func(w http.ResponseWriter, r *http.Request) {
		outbox, err := q.OutboxDepth(r.Context())
		if err != nil {
			slog.Error("wsapi: outbox depth failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		ops, err := q.OpLogStats(r.Context())
		if err != nil {
			slog.Error("wsapi: op log stats failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(statsResponse{
			OutboxDepth: outbox.Depth, OutboxOldestSeconds: outbox.OldestSeconds,
			Ops: ops.Ops, Pages: ops.Pages, LagSeconds: ops.LagSeconds,
		})
	}
}
