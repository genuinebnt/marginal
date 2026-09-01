package wsapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	collabrepo "marginal/collaboration-service/internal/collabrepo/gen"
)

// profileResponse is § 23b PROFILE — "a person as their op log".
//
// Every number is a GROUP BY over collab.ops. None of it is a counter kept
// beside the log: a counter can drift from what happened, a projection of
// the log cannot, and the screen's whole claim is that it cannot.
//
// What is deliberately NOT here: page titles, topics and tags. Those live
// in document-service's schema and this service does not reach across
// (DATA_MODEL.md §1) — the client joins them from the graph it already
// fetches, the same way § 18b's audit rows get their titles.
type profileResponse struct {
	ActorID string `json:"actor_id"`
	Ops     int64  `json:"ops"`
	Pages   int64  `json:"pages"`
	// One entry per day the actor wrote anything, for the contribution
	// grid. Silent days are ABSENT rather than zero — the client draws a
	// fixed 52×7 and looks each date up, so a year of empty rows would be
	// payload spent saying nothing happened.
	Daily  []dayJSON     `json:"daily"`
	Top    []pageOpsJSON `json:"top_pages"`
	Recent []recentJSON  `json:"recent"`
	// Who else has ops on the pages this person touched. Pages in common
	// rather than ops in common: one edit to forty of your pages is more
	// working-alongside than forty edits to one, and counting ops would
	// say the opposite.
	With []collaboratorJSON `json:"most_edited_with"`
	// The window every figure above covers, stated rather than implied: a
	// grid that silently means "52 weeks" and a total that silently means
	// "all time" would be two numbers that look comparable and are not.
	Weeks int `json:"weeks"`
}

type dayJSON struct {
	Day string `json:"day"` // YYYY-MM-DD
	Ops int64  `json:"ops"`
}

type pageOpsJSON struct {
	PageID      string `json:"page_id"`
	Ops         int64  `json:"ops"`
	LastTouched string `json:"last_touched"`
}

type collaboratorJSON struct {
	ActorID string `json:"actor_id"`
	Pages   int64  `json:"pages"`
}

type recentJSON struct {
	ID        string `json:"id"`
	PageID    string `json:"page_id"`
	Kind      string `json:"kind"`
	Seq       int64  `json:"seq"`
	CreatedAt string `json:"created_at"`
}

const (
	profileTopPages = 8
	profileRecent   = 20
	profileWeeks    = 52
)

// NewProfileHandler serves GET /collab/people/{id}/profile.
//
// Behind RequireToken like every other read here: an op log says who edited
// what and when, which is a page's content by another name.
func NewProfileHandler(pool *pgxpool.Pool) http.HandlerFunc {
	q := collabrepo.New(pool)
	return func(w http.ResponseWriter, r *http.Request) {
		actorID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid actor id", http.StatusBadRequest)
			return
		}
		id := pgtype.UUID{Bytes: actorID, Valid: true}
		ctx := r.Context()

		totals, err := q.ProfileTotals(ctx, id)
		if err != nil {
			slog.Error("wsapi: profile totals", "actor_id", actorID, "err", err)
			http.Error(w, "reading profile failed", http.StatusInternalServerError)
			return
		}

		out := profileResponse{
			ActorID: actorID.String(),
			Ops:     totals.Ops,
			Pages:   totals.Pages,
			Weeks:   profileWeeks,
			// Empty, never null — this repo's wire convention, so a client
			// can iterate without a guard (docs/api/diagnostics.md).
			Daily:  []dayJSON{},
			Top:    []pageOpsJSON{},
			Recent: []recentJSON{},
			With:   []collaboratorJSON{},
		}

		if days, err := q.ProfileDaily(ctx, id); err == nil {
			for _, d := range days {
				out.Daily = append(out.Daily, dayJSON{Day: d.Day.Time.Format(time.DateOnly), Ops: d.Ops})
			}
		} else {
			slog.Error("wsapi: profile daily", "actor_id", actorID, "err", err)
		}

		if pages, err := q.ProfilePages(ctx, collabrepo.ProfilePagesParams{
			ActorID: id, RowLimit: profileTopPages,
		}); err == nil {
			for _, p := range pages {
				out.Top = append(out.Top, pageOpsJSON{
					PageID: uuid.UUID(p.PageID.Bytes).String(), Ops: p.Ops,
					LastTouched: p.LastTouched.Time.UTC().Format(time.RFC3339),
				})
			}
		} else {
			slog.Error("wsapi: profile pages", "actor_id", actorID, "err", err)
		}

		if recent, err := q.ProfileRecent(ctx, collabrepo.ProfileRecentParams{
			ActorID: id, RowLimit: profileRecent,
		}); err == nil {
			for _, o := range recent {
				out.Recent = append(out.Recent, recentJSON{
					ID: uuid.UUID(o.ID.Bytes).String(), PageID: uuid.UUID(o.PageID.Bytes).String(),
					Kind: o.Kind, Seq: o.Seq, CreatedAt: o.CreatedAt.Time.UTC().Format(time.RFC3339),
				})
			}
		} else {
			slog.Error("wsapi: profile recent", "actor_id", actorID, "err", err)
		}

		if with, err := q.ProfileCollaborators(ctx, collabrepo.ProfileCollaboratorsParams{
			ActorID: id, RowLimit: profileTopPages,
		}); err == nil {
			for _, c := range with {
				out.With = append(out.With, collaboratorJSON{
					ActorID: uuid.UUID(c.ActorID.Bytes).String(), Pages: c.Pages,
				})
			}
		} else {
			slog.Error("wsapi: profile collaborators", "actor_id", actorID, "err", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			slog.Error("wsapi: profile encode", "err", err)
		}
	}
}
