package wsapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/anchor"
	collabrepo "marginal/collaboration-service/internal/collabrepo/gen"
	"marginal/collaboration-service/internal/session"
)

// Comment threads — docs/api/comments.md.
//
// Served here because a thread's extent is an AnchorRange, and an anchor is
// only resolvable by whatever holds the block's live rope. And NOT as ops:
// a comment changes neither the block tree nor any text, so putting one in
// the op log would put it in the document's undo stack, where one ⌘Z too
// many silently retracts somebody's remark.

type threadJSON struct {
	ID         string  `json:"id"`
	BlockID    string  `json:"block_id"`
	Quoted     string  `json:"quoted"`
	ResolvedAt *string `json:"resolved_at"`
	CreatedBy  string  `json:"created_by"`
	CreatedAt  string  `json:"created_at"`
	// Range is where the anchors point RIGHT NOW, against the live rope.
	// nil when they no longer resolve.
	Range *rangeJSON `json:"range"`
	// Orphaned: the text this thread was about is gone. The thread is
	// still returned — deleting somebody's remark because somebody else
	// edited a sentence is a worse failure than an untidy list, and a
	// comment about text that no longer exists is frequently the most
	// interesting one there.
	Orphaned bool          `json:"orphaned"`
	Comments []commentJSON `json:"comments"`
}

type rangeJSON struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type commentJSON struct {
	ID        string  `json:"id"`
	ThreadID  string  `json:"thread_id"`
	AuthorID  string  `json:"author_id"`
	Body      string  `json:"body"`
	EditedAt  *string `json:"edited_at,omitempty"`
	CreatedAt string  `json:"created_at"`
}

// CommentsHandler serves every comment route. It needs the Manager because
// resolving an anchor means asking the live session for the page.
type CommentsHandler struct {
	q       *collabrepo.Queries
	manager *session.Manager
}

func NewCommentsHandler(pool *pgxpool.Pool, m *session.Manager) *CommentsHandler {
	return &CommentsHandler{q: collabrepo.New(pool), manager: m}
}

func (h *CommentsHandler) Mount(mux *http.ServeMux, verifier TokenVerifier) {
	mux.HandleFunc("GET /collab/pages/{id}/comments", RequireToken(verifier, h.list))
	mux.HandleFunc("POST /collab/pages/{id}/comments", RequireToken(verifier, h.open))
	mux.HandleFunc("POST /collab/threads/{id}/comments", RequireToken(verifier, h.reply))
	mux.HandleFunc("POST /collab/threads/{id}/resolve", RequireToken(verifier, h.resolve))
	mux.HandleFunc("POST /collab/threads/{id}/reopen", RequireToken(verifier, h.reopen))
}

func pgID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func (h *CommentsHandler) list(w http.ResponseWriter, r *http.Request) {
	pageID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid page id", http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	rows, err := h.q.ThreadsForPage(ctx, pgID(pageID))
	if err != nil {
		slog.Error("wsapi: listing threads", "page_id", pageID, "err", err)
		http.Error(w, "listing threads failed", http.StatusInternalServerError)
		return
	}
	all, err := h.q.CommentsForPage(ctx, pgID(pageID))
	if err != nil {
		slog.Error("wsapi: listing comments", "page_id", pageID, "err", err)
		http.Error(w, "listing comments failed", http.StatusInternalServerError)
		return
	}
	byThread := map[uuid.UUID][]commentJSON{}
	for _, c := range all {
		tid := uuid.UUID(c.ThreadID.Bytes)
		byThread[tid] = append(byThread[tid], commentJSON{
			ID: uuid.UUID(c.ID.Bytes).String(), ThreadID: tid.String(),
			AuthorID: uuid.UUID(c.AuthorID.Bytes).String(), Body: c.Body,
			EditedAt: optTime(c.EditedAt), CreatedAt: c.CreatedAt.Time.UTC().Format(time.RFC3339),
		})
	}

	// One session lookup for the whole page, not one per thread: resolving
	// is a map read once the rope is in hand, and opening the page thirty
	// times to answer thirty threads would be thirty replays.
	sess, err := h.manager.Get(ctx, pageID)
	if err != nil {
		slog.Error("wsapi: opening session for comments", "page_id", pageID, "err", err)
		http.Error(w, "opening page failed", http.StatusInternalServerError)
		return
	}

	out := struct {
		Threads []threadJSON `json:"threads"`
	}{Threads: make([]threadJSON, 0, len(rows))}

	for _, t := range rows {
		var start, end anchor.Anchor
		if err := json.Unmarshal(t.AnchorStart, &start); err != nil {
			slog.Error("wsapi: decoding thread anchor", "thread_id", uuid.UUID(t.ID.Bytes), "err", err)
			continue
		}
		if err := json.Unmarshal(t.AnchorEnd, &end); err != nil {
			slog.Error("wsapi: decoding thread anchor", "thread_id", uuid.UUID(t.ID.Bytes), "err", err)
			continue
		}
		blockID := documentcore.BlockID(uuid.UUID(t.BlockID.Bytes))
		s, e, ok := sess.ResolveRange(blockID, anchor.AnchorRange{Start: start, End: end})

		tid := uuid.UUID(t.ID.Bytes)
		th := threadJSON{
			ID: tid.String(), BlockID: uuid.UUID(t.BlockID.Bytes).String(),
			Quoted: t.Quoted, ResolvedAt: optTime(t.ResolvedAt),
			CreatedBy: uuid.UUID(t.CreatedBy.Bytes).String(),
			CreatedAt: t.CreatedAt.Time.UTC().Format(time.RFC3339),
			Orphaned:  !ok,
			// Empty, never null — this repo's wire convention, so a client
			// iterates without a guard.
			Comments: byThread[tid],
		}
		if th.Comments == nil {
			th.Comments = []commentJSON{}
		}
		if ok {
			th.Range = &rangeJSON{Start: s, End: e}
		}
		out.Threads = append(out.Threads, th)
	}

	writeJSON(w, out)
}

func (h *CommentsHandler) open(w http.ResponseWriter, r *http.Request) {
	pageID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid page id", http.StatusBadRequest)
		return
	}
	author, ok := ActorFrom(r)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	var body struct {
		BlockID     string          `json:"block_id"`
		AnchorStart json.RawMessage `json:"anchor_start"`
		AnchorEnd   json.RawMessage `json:"anchor_end"`
		Quoted      string          `json:"quoted"`
		Body        string          `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	blockID, err := uuid.Parse(body.BlockID)
	if err != nil {
		http.Error(w, "invalid block id", http.StatusBadRequest)
		return
	}
	if body.Body == "" {
		http.Error(w, "a thread needs a first comment", http.StatusBadRequest)
		return
	}

	threadID := uuid.Must(uuid.NewV7())
	ctx := r.Context()
	if _, err := h.q.CreateThread(ctx, collabrepo.CreateThreadParams{
		ID: pgID(threadID), PageID: pgID(pageID), BlockID: pgID(blockID),
		AnchorStart: body.AnchorStart, AnchorEnd: body.AnchorEnd,
		Quoted: body.Quoted, CreatedBy: pgID(author),
	}); err != nil {
		slog.Error("wsapi: creating thread", "page_id", pageID, "err", err)
		http.Error(w, "creating thread failed", http.StatusInternalServerError)
		return
	}
	c, err := h.q.AddComment(ctx, collabrepo.AddCommentParams{
		ID: pgID(uuid.Must(uuid.NewV7())), ThreadID: pgID(threadID),
		AuthorID: pgID(author), Body: body.Body,
	})
	if err != nil {
		slog.Error("wsapi: adding first comment", "thread_id", threadID, "err", err)
		http.Error(w, "adding comment failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, commentJSON{
		ID: uuid.UUID(c.ID.Bytes).String(), ThreadID: threadID.String(),
		AuthorID: author.String(), Body: c.Body,
		CreatedAt: c.CreatedAt.Time.UTC().Format(time.RFC3339),
	})
}

func (h *CommentsHandler) reply(w http.ResponseWriter, r *http.Request) {
	threadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid thread id", http.StatusBadRequest)
		return
	}
	author, ok := ActorFrom(r)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		http.Error(w, "a comment needs a body", http.StatusBadRequest)
		return
	}
	c, err := h.q.AddComment(r.Context(), collabrepo.AddCommentParams{
		ID: pgID(uuid.Must(uuid.NewV7())), ThreadID: pgID(threadID),
		AuthorID: pgID(author), Body: body.Body,
	})
	if err != nil {
		slog.Error("wsapi: replying", "thread_id", threadID, "err", err)
		http.Error(w, "replying failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, commentJSON{
		ID: uuid.UUID(c.ID.Bytes).String(), ThreadID: threadID.String(),
		AuthorID: author.String(), Body: c.Body,
		CreatedAt: c.CreatedAt.Time.UTC().Format(time.RFC3339),
	})
}

func (h *CommentsHandler) resolve(w http.ResponseWriter, r *http.Request) {
	h.setResolved(w, r, true)
}

func (h *CommentsHandler) reopen(w http.ResponseWriter, r *http.Request) {
	h.setResolved(w, r, false)
}

// setResolved is idempotent on purpose: resolving an already-resolved
// thread is not an error, because the caller's intent already holds and
// failing it would turn two people clicking at once into a bug report.
func (h *CommentsHandler) setResolved(w http.ResponseWriter, r *http.Request, resolved bool) {
	threadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid thread id", http.StatusBadRequest)
		return
	}
	actor, ok := ActorFrom(r)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	if resolved {
		_, err = h.q.ResolveThread(r.Context(), collabrepo.ResolveThreadParams{
			ID: pgID(threadID), ResolvedBy: pgID(actor),
		})
	} else {
		_, err = h.q.ReopenThread(r.Context(), pgID(threadID))
	}
	if err != nil {
		slog.Error("wsapi: setting thread state", "thread_id", threadID, "err", err)
		http.Error(w, "updating thread failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func optTime(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format(time.RFC3339)
	return &s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("wsapi: encoding response", "err", err)
	}
}
