package notify

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Handler exposes ListForUser over plain HTTP — reached directly by the
// browser, same as collaboration-service's WebSocket, not proxied through
// api-gateway (that shim only translates document-service/auth-service's
// gRPC; nothing here is gRPC). Actor identity is the same temporary
// X-Actor-Id header stand-in pages.md/auth.md already document.
type Handler struct {
	repo Repo
}

func NewHandler(repo Repo) *Handler { return &Handler{repo: repo} }

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /notifications", h.list)
	// An inbox is cleared by acting on it, so the two acts are routes.
	// POST rather than PATCH: this is not a partial update of a resource the
	// caller composed, it is a named transition with no body.
	mux.HandleFunc("POST /notifications/{id}/read", h.markRead)
	mux.HandleFunc("POST /notifications/read-all", h.markAllRead)
}

type notificationJSON struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Message   string  `json:"message"`
	ReadAt    *string `json:"read_at,omitempty"`
	CreatedAt string  `json:"created_at"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	actorID := r.Header.Get("X-Actor-Id")
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, "missing X-Actor-Id")
		return
	}
	userID, err := uuid.Parse(actorID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid X-Actor-Id")
		return
	}

	const defaultLimit = 50
	notifications, err := h.repo.ListForUser(r.Context(), userID, defaultLimit)
	if err != nil {
		slog.Error("notify: listing notifications failed", "user_id", userID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	unread, err := h.repo.CountUnread(r.Context(), userID)
	if err != nil {
		slog.Error("notify: counting unread failed", "user_id", userID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]notificationJSON, len(notifications))
	for i, n := range notifications {
		out[i] = toNotificationJSON(n)
	}
	// unread travels WITH the list rather than on its own route: the bell and
	// the panel are drawn from one response, so they cannot disagree about
	// how many rows are unread.
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out, "unread": unread})
}

// actor resolves the temporary X-Actor-Id stand-in, writing the error itself
// so each handler is a straight line rather than a repeated eight-line
// preamble.
func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	actorID := r.Header.Get("X-Actor-Id")
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, "missing X-Actor-Id")
		return uuid.Nil, false
	}
	userID, err := uuid.Parse(actorID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid X-Actor-Id")
		return uuid.Nil, false
	}
	return userID, true
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.actor(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid notification id")
		return
	}
	n, err := h.repo.MarkRead(r.Context(), userID, id)
	if err != nil {
		slog.Error("notify: mark read failed", "user_id", userID, "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// 0 rows is "already read, or not yours" — deliberately not a 404. The
	// two are indistinguishable to a caller by design: reporting "not yours"
	// separately would confirm the row exists.
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.actor(w, r)
	if !ok {
		return
	}
	n, err := h.repo.MarkAllRead(r.Context(), userID)
	if err != nil {
		slog.Error("notify: mark all read failed", "user_id", userID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

func toNotificationJSON(n Notification) notificationJSON {
	out := notificationJSON{
		ID:        n.ID.String(),
		Kind:      n.Kind,
		Message:   n.Message,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if n.ReadAt != nil {
		s := n.ReadAt.UTC().Format(time.RFC3339Nano)
		out.ReadAt = &s
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
