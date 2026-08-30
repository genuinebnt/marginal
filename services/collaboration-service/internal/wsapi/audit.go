package wsapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jackc/pgx/v5/pgxpool"
	collabrepo "marginal/collaboration-service/internal/collabrepo/gen"
)

// § 18b AUDIT LOG classifies ops into the classes the screen
// filters by. The database does not know what the product
// considers destructive, and should not — this is the one place
// that judgement lives.
//
// Everything not named here is content. That default is
// deliberate: a new op kind should appear in the log as ordinary
// content rather than vanish from it because nobody remembered
// to classify it.
// Kinds arrive scope-prefixed on the wire ("block:InsertBlock",
// "text:DeleteText") — RFC-002's two op tiers. Matched on the
// part after the colon, which is the op; the tier is not what
// makes something destructive.
//
// This was wrong once, in a way nothing failed on: a bare-name
// map silently classified every delete as ordinary content and
// the DESTRUCTIVE filter returned an empty list that read as
// "nothing was deleted".
var destructiveOps = map[string]bool{
	"DeleteBlock": true,
	"DeleteText":  true,
	// Moving is not losing: MoveBlock carries `from` as well as
	// `to` and inverts exactly (RFC-002 §3).
	"MoveBlock": false,
}

// auditRow is one line of the log.
//
// No payload. An audit row says who did what to which page; the
// text somebody typed is the document's business, and an admin
// surface that quietly includes it is a more invasive feature
// than the one anyone asked for.
type auditRow struct {
	ID        string `json:"id"`
	Seq       int64  `json:"seq"`
	PageID    string `json:"page_id"`
	ActorID   string `json:"actor_id"`
	ActorKind string `json:"actor_kind"`
	Kind      string `json:"kind"`
	// Class is "content" or "destructive" — what § 18b's filter
	// chips select on.
	Class string `json:"class"`
	// UndoGroup ties a row to the one gesture that produced it,
	// so a single keystroke that emitted three ops reads as one
	// action rather than three.
	UndoGroup string `json:"undo_group,omitempty"`
	CreatedAt string `json:"created_at"`
}

type auditResponse struct {
	Rows []auditRow `json:"rows"`
	// Counts per class, over the WHOLE log rather than the page
	// returned — the panel says "by class", and a count of what
	// happens to be on screen would answer a different question.
	Counts map[string]int64 `json:"counts"`
	// Total is every op ever accepted. `rows` is the most recent
	// slice of it, and the screen says how many it is not showing.
	Total int64 `json:"total"`
	// Kinds is every op kind seen, with its count — the honest
	// breakdown behind the class rollup.
	Kinds []kindCount `json:"kinds"`
}

type kindCount struct {
	Kind  string `json:"kind"`
	Class string `json:"class"`
	N     int64  `json:"n"`
}

func classOf(kind string) string {
	if destructiveOps[opName(kind)] {
		return "destructive"
	}
	return "content"
}

// opName strips the tier prefix. An unprefixed kind is returned
// as-is rather than rejected — the log holds every kind ever
// written, including any that predate the convention.
func opName(kind string) string {
	if i := strings.IndexByte(kind, ':'); i >= 0 {
		return kind[i+1:]
	}
	return kind
}

const (
	defaultAuditLimit = 100
	maxAuditLimit     = 500
)

// NewAuditHandler serves § 18b's content rows.
//
// Content only: auth events are auth-service's and arrive
// separately (auth.md `/admin/audit`), because there is no
// cross-service join here and inventing one would be the exact
// thing DATA_MODEL.md forbids. The client merges the two by
// timestamp, which is where a join across service boundaries
// belongs.
//
// Read-only over an append-only table, so it is safe to call
// while people are editing.
func NewAuditHandler(pool *pgxpool.Pool) http.HandlerFunc {
	q := collabrepo.New(pool)
	return func(w http.ResponseWriter, r *http.Request) {
		limit := defaultAuditLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
				return
			}
			limit = min(n, maxAuditLimit)
		}

		// A class filter is applied as a kind filter, because
		// class is this package's idea and the database only
		// knows kinds.
		var kinds []string
		switch r.URL.Query().Get("class") {
		case "destructive":
			for k, v := range destructiveOps {
				if v {
					kinds = append(kinds, "block:"+k, "text:"+k, k)
				}
			}
		case "", "all", "content":
			// content is "everything not destructive", which is
			// not expressible as a kind list without knowing
			// every kind — so it is filtered after the read.
		default:
			http.Error(w, "unknown class", http.StatusBadRequest)
			return
		}

		rows, err := q.AuditOps(r.Context(), collabrepo.AuditOpsParams{
			Kinds: kinds, RowLimit: int32(limit),
		})
		if err != nil {
			slog.Error("wsapi: audit ops failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		counts, err := q.AuditCounts(r.Context())
		if err != nil {
			slog.Error("wsapi: audit counts failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		contentOnly := r.URL.Query().Get("class") == "content"
		resp := auditResponse{
			Rows:   make([]auditRow, 0, len(rows)),
			Counts: map[string]int64{"content": 0, "destructive": 0},
			Kinds:  make([]kindCount, 0, len(counts)),
		}
		for _, c := range counts {
			cls := classOf(c.Kind)
			resp.Counts[cls] += c.N
			resp.Total += c.N
			resp.Kinds = append(resp.Kinds, kindCount{Kind: c.Kind, Class: cls, N: c.N})
		}
		for _, row := range rows {
			cls := classOf(row.Kind)
			if contentOnly && cls != "content" {
				continue
			}
			resp.Rows = append(resp.Rows, auditRow{
				ID: uuidString(row.ID), Seq: row.Seq,
				PageID: uuidString(row.PageID), ActorID: uuidString(row.ActorID),
				ActorKind: row.ActorKind, Kind: row.Kind, Class: cls,
				UndoGroup: uuidString(row.UndoGroup),
				CreatedAt: row.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05.000Z"),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// uuidString renders a pgtype.UUID, and an invalid one as the
// empty string rather than a zero UUID — "absent" and
// "00000000-…" are different facts and the screen shows them
// differently.
func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	s, err := u.Value()
	if err != nil {
		return ""
	}
	str, _ := s.(string)
	return str
}
