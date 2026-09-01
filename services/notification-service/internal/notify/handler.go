package notify

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TokenVerifier turns a bearer token into an actor id — an interface at
// its point of use, mirroring wsapi.TokenVerifier and authmw.TokenVerifier.
// The three entry points that are NOT behind api-gateway should describe
// this dependency the same way.
type TokenVerifier interface {
	Subject(ctx context.Context, token string) (uuid.UUID, error)
}

// Handler exposes ListForUser over plain HTTP — reached directly by the
// browser, same as collaboration-service's WebSocket, not proxied through
// api-gateway (that shim only translates document-service/auth-service's
// gRPC; nothing here is gRPC).
//
// Which is exactly why it verifies the token itself (ADR-013 §1). This
// service was missed in that pass and kept reading X-Actor-Id — a header
// the SPA had stopped sending, so every inbox came back empty and the
// unread badge silently vanished from every screen's top bar. An
// authorization gap and a UI regression from the same line.
type Handler struct {
	repo     Repo
	verifier TokenVerifier
}

// NewHandler. verifier is required: a nil one would mean this endpoint
// serves anybody's inbox to anybody, which is the state ADR-013 exists to
// leave behind.
func NewHandler(repo Repo, verifier TokenVerifier) *Handler {
	if verifier == nil {
		panic("notify: NewHandler requires a TokenVerifier")
	}
	return &Handler{repo: repo, verifier: verifier}
}

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
	userID, err := h.actorFrom(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "a valid access token is required")
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

// actorFrom verifies the bearer token and returns its subject.
//
// One error for every failure — missing, malformed, expired, badly signed.
// Which one it was describes the server's key state, and answering that for
// free is how the state gets mapped.
func (h *Handler) actorFrom(r *http.Request) (uuid.UUID, error) {
	const prefix = "Bearer "
	h2 := r.Header.Get("Authorization")
	if len(h2) <= len(prefix) || !strings.EqualFold(h2[:len(prefix)], prefix) {
		return uuid.Nil, errUnauthenticated
	}
	return h.verifier.Subject(r.Context(), strings.TrimSpace(h2[len(prefix):]))
}

var errUnauthenticated = errors.New("notify: unauthenticated")

// actor writes the error itself so each handler is a straight line rather
// than a repeated preamble.
func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, err := h.actorFrom(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "a valid access token is required")
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
