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

	out := make([]notificationJSON, len(notifications))
	for i, n := range notifications {
		out[i] = toNotificationJSON(n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out})
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
