package wsapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	collabrepo "marginal/collaboration-service/internal/collabrepo/gen"
	"marginal/collaboration-service/internal/session"
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
	OutboxDepth         int64   `json:"outbox_depth"`
	OutboxOldestSeconds float64 `json:"outbox_oldest_seconds"`
	Ops                 int64   `json:"ops"`
	Pages               int64   `json:"pages"`
	// LagSeconds is time since the newest op — on an idle instance
	// this is large and perfectly healthy, so the screen labels it
	// rather than colouring it red.
	LagSeconds float64 `json:"lag_seconds"`

	// OpenSessions is pages with a live rope in memory — NOT
	// people signed in (auth-service's number) and not editors
	// connected. Three meanings of "sessions"; § 18 labels which.
	OpenSessions int `json:"open_sessions"`
	// DatabaseBytes is this service's own database, because the
	// architecture is database-per-service and an instance-wide
	// "DB size" is a number nobody owns.
	DatabaseBytes int64 `json:"database_bytes"`
	// OpsPerHour is the last 14 hours, oldest first, with quiet
	// hours present as zero — a sparkline that omits them draws
	// a busy day where there was a gap.
	OpsPerHour []int64 `json:"ops_per_hour"`
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
func NewStatsHandler(pool *pgxpool.Pool, manager *session.Manager) http.HandlerFunc {
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

		size, err := q.DatabaseSize(r.Context())
		if err != nil {
			slog.Error("wsapi: database size failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		hours, err := q.OpsPerHour(r.Context())
		if err != nil {
			slog.Error("wsapi: ops per hour failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Never nil: the screen maps over this, and a nil slice
		// reaches it as JSON null. The same lesson § 14 learned
		// the hard way.
		perHour := make([]int64, 0, len(hours))
		for _, h := range hours {
			perHour = append(perHour, h.Ops)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(statsResponse{
			OutboxDepth: outbox.Depth, OutboxOldestSeconds: outbox.OldestSeconds,
			Ops: ops.Ops, Pages: ops.Pages, LagSeconds: ops.LagSeconds,
			OpenSessions: manager.OpenSessions(), DatabaseBytes: size,
			OpsPerHour: perHour,
		})
	}
}
