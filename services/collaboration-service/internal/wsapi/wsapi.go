// Package wsapi is collaboration-service's WebSocket transport — the
// piece ARCHITECTURE.md §4's sequence diagram calls "WS /collab/pages/:id"
// and everything built so far (session, flush, wal, ops) exists to drive.
// It is deliberately thin: decode a client frame, call
// session.Session.ApplyClientOp, encode the result. All the actual
// editing logic already lives in internal/session.
//
// Auth is a temporary stand-in, same convention document-service's
// PageService already uses (docs/api/pages.md): actor identity is read
// directly from request headers rather than verified via a JWT, because
// no api-gateway exists in this repo's scope to have done that
// verification upstream. This is scaffolding to unblock the browser path,
// not a real trust boundary — do not ship this past a demo without a real
// gateway or in-process JWT verification in front of it.
package wsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/doctext"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
	"marginal/collaboration-service/internal/session"
)

// outboxBufferSize bounds how many server frames can queue for one slow
// connection before it's disconnected rather than let it stall
// broadcasting to every other client on the same session (Session.
// ApplyClientOp calls Subscriber.Deliver while holding its own mutex).
const outboxBufferSize = 64

// clientMessage is every shape a client sends: an op to apply ("op"), its
// own current caret/selection ("cursor") — fire-and-forget, no ack — a
// request to undo/redo its own most recent gesture ("undo"/"redo" — no
// further payload, docs/api/collaboration.md §2.1), or a request to
// restore the live document to a past point in its own confirmed op log
// ("restore" — docs/api/collaboration.md §2.2, history.html's "restore
// to a point," v2.4.0). Cursor is set only for Type "cursor"; a nil
// BlockID inside it means "not focused in any block right now" (blurred
// everywhere). UndoGroup is set only for Type "op", optional even then —
// nil is RFC-002 §3's "group of one." ToStep is set only for Type
// "restore".
type clientMessage struct {
	Type      string          `json:"type"` // "op" | "cursor" | "undo" | "redo" | "restore"
	Op        json.RawMessage `json:"op,omitempty"`
	UndoGroup *uuid.UUID      `json:"undo_group,omitempty"`
	Cursor    *cursorPayload  `json:"cursor,omitempty"`
	ToStep    *int            `json:"to_step,omitempty"`
}

// cursorPayload is clientMessage's "cursor" shape — see clientMessage's
// own doc comment. Offsets are rune offsets into the block's own live
// text (docs/api/collaboration.md), the same unit InsertText/DeleteText
// already use — never byte offsets, and (client-side, see RichEditorPane)
// not yet UTF-16-safe for multi-byte text, an accepted simplification
// this repo already carries elsewhere (marks.ts's own doc comment).
type cursorPayload struct {
	BlockID *documentcore.BlockID `json:"block_id"`
	Start   int                   `json:"start"`
	End     int                   `json:"end"`
}

// serverMessage is everything a client can receive. Snapshot is set only
// for Type "snapshot" — the whole page (title, ordered blocks, each
// block's live text). Op is set for "ack"/"broadcast"; Boundaries
// accompanies both, when that op was a pageop.Text (a character edit
// scoped to one block) — that block's current start/end anchors (nil
// once the block is empty), which is what a client with no anchor of its
// own (a plain rune-offset text widget) needs to build its own next
// "replace this block's text" op; see docs/api/collaboration.md and
// doctext.Text.Boundaries's doc comment for why this exists. Boundaries is
// absent for a structural (pageop.Block) op — nothing about it is
// rune-offset-shaped.
// Present, on a "snapshot" frame, is every distinct actor already on the
// page at connect time (session.Session.Subscribe's own return value) —
// so a joining client knows who else is here immediately, not only after
// someone's first future join/leave. ActorID/Joined are set only for
// Type "presence": a later join (Joined true) or leave (Joined false)
// from an actor already connected when this client joined. Cursors, also
// on "snapshot," is present's own last-known caret/selection (only for
// those who have one); Cursor is set for Type "cursor" — one actor's
// caret just moved (or, BlockID nil, they blurred out of every block).
type serverMessage struct {
	Type       string              `json:"type"` // "snapshot" | "ack" | "broadcast" | "presence" | "cursor" | "error"
	Snapshot   *session.Snapshot   `json:"snapshot,omitempty"`
	Present    []string            `json:"present,omitempty"`
	Cursors    []cursorWire        `json:"cursors,omitempty"`
	ActorID    string              `json:"actor_id,omitempty"`
	Joined     *bool               `json:"joined,omitempty"`
	Cursor     *cursorWire         `json:"cursor,omitempty"`
	Op         *oplog.LoggedOp     `json:"op,omitempty"`
	Boundaries *anchor.AnchorRange `json:"boundaries,omitempty"`
	Message    string              `json:"message,omitempty"`
}

// cursorWire is session.CursorEvent's wire shape — ActorID spelled out
// (a CursorEvent en route to one specific client already knows whose it
// is from context in every other message type, but a batch of them in a
// snapshot's Cursors doesn't).
type cursorWire struct {
	ActorID string                `json:"actor_id"`
	BlockID *documentcore.BlockID `json:"block_id"`
	Start   int                   `json:"start"`
	End     int                   `json:"end"`
}

func toCursorWire(e session.CursorEvent) cursorWire {
	return cursorWire{ActorID: e.ActorID.String(), BlockID: e.BlockID, Start: e.Start, End: e.End}
}

// Handler upgrades a connection for one page, wires it into that page's
// session.Session, and runs its read/write loops until the connection
// closes. Register it at a path carrying the page id — e.g.
// mux.HandleFunc("/collab/pages/{id}", handler.ServeHTTP) — Go 1.22+'s
// ServeMux pattern syntax, read via r.PathValue("id").
type Handler struct {
	manager    *session.Manager
	acceptOpts *websocket.AcceptOptions
}

// NewHandler. acceptOpts is passed straight through to websocket.Accept —
// nil means coder/websocket's own default, which REJECTS a cross-origin
// upgrade (the browser's Origin header not matching the request's Host).
// web/ is served from a different origin (a different port, in local
// dev), so cmd/main.go must pass real options here — see that file's own
// comment on why InsecureSkipVerify is the right default at this repo's
// scope (found by actually trying this in a browser, not by any test:
// every test in this package dials from the same process, which
// coder/websocket's default already allows since the request host and
// origin genuinely match there).
func NewHandler(m *session.Manager, acceptOpts *websocket.AcceptOptions) *Handler {
	return &Handler{manager: m, acceptOpts: acceptOpts}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pageID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid page id", http.StatusBadRequest)
		return
	}
	actorID, actorKind, err := actorFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, h.acceptOpts)
	if err != nil {
		slog.Error("wsapi: websocket upgrade rejected", "page_id", pageID, "err", err)
		return // Accept already wrote the error response
	}
	defer func() { _ = conn.CloseNow() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	sess, err := h.manager.Get(ctx, pageID)
	if err != nil {
		slog.Error("wsapi: opening session failed", "page_id", pageID, "err", err)
		_ = conn.Close(websocket.StatusInternalError, "opening session failed")
		return
	}

	sub := newConnSubscriber(cancel)
	subscription := sess.Subscribe(actorID, sub)
	defer subscription.Close()

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		defer cancel() // a dead writer means a dead connection
		sub.writeLoop(ctx, conn)
	}()

	presentIDs := make([]string, len(subscription.Present))
	for i, a := range subscription.Present {
		presentIDs[i] = a.String()
	}
	cursorWires := make([]cursorWire, len(subscription.Cursors))
	for i, c := range subscription.Cursors {
		cursorWires[i] = toCursorWire(c)
	}
	sub.enqueue(serverMessage{Type: "snapshot", Snapshot: &subscription.Snapshot, Present: presentIDs, Cursors: cursorWires})

	readLoop(ctx, conn, sess, sub, actorID, actorKind, subscription.ID)

	cancel()
	<-writeDone
}

// actorFromRequest is the temporary auth stand-in described in the
// package doc comment. Accepts the actor id as either the X-Actor-Id
// header (pages.md/auth.md's convention, and what a non-browser client —
// grpcurl, a test, another service — can set directly) or an
// `actor_id` query parameter (what a real browser actually has to use:
// the WebSocket browser API has no mechanism to set custom headers on
// the upgrade request at all — not a missing feature to add, a
// characteristic of the API itself — so this is the one place in the
// repo where the header stand-in needs a second on-the-wire spelling).
// The header wins if both are somehow present. X-Actor-Kind/actor_kind
// defaults to "user" when absent — every real client is a person until
// agents/plugins are actors (ADR-009, not yet in this repo's scope).
func actorFromRequest(r *http.Request) (uuid.UUID, oplog.ActorKind, error) {
	raw := r.Header.Get("X-Actor-Id")
	if raw == "" {
		raw = r.URL.Query().Get("actor_id")
	}
	if raw == "" {
		return uuid.UUID{}, "", fmt.Errorf("wsapi: missing actor id (X-Actor-Id header or actor_id query param)")
	}
	actorID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, "", fmt.Errorf("wsapi: invalid actor id: %w", err)
	}
	kindRaw := r.Header.Get("X-Actor-Kind")
	if kindRaw == "" {
		kindRaw = r.URL.Query().Get("actor_kind")
	}
	kind := oplog.ActorKind(kindRaw)
	if kind == "" {
		kind = oplog.ActorUser
	}
	if !kind.Valid() {
		return uuid.UUID{}, "", fmt.Errorf("wsapi: invalid actor kind %q", kind)
	}
	return actorID, kind, nil
}

func readLoop(ctx context.Context, conn *websocket.Conn, sess *session.Session, sub *connSubscriber, actorID uuid.UUID, actorKind oplog.ActorKind, subID uint64) {
	for {
		var msg clientMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return // connection closed, context cancelled, or a protocol error — either way, done
		}

		switch msg.Type {
		case "op":
			op, err := pageop.Unmarshal(msg.Op)
			if err != nil {
				sub.enqueue(serverMessage{Type: "error", Message: "invalid op: " + err.Error()})
				continue
			}

			result, err := sess.ApplyClientOp(ctx, actorID, actorKind, op, msg.UndoGroup, subID)
			if err != nil {
				sub.enqueue(serverMessage{Type: "error", Message: clientSafeMessage(err)})
				continue
			}
			sub.enqueue(serverMessage{Type: "ack", Op: &result.Op, Boundaries: result.Boundaries})

		case "undo":
			results, err := sess.Undo(ctx, actorID, actorKind, subID)
			enqueueUndoRedoAcks(sub, results, err)

		case "redo":
			results, err := sess.Redo(ctx, actorID, actorKind, subID)
			enqueueUndoRedoAcks(sub, results, err)

		case "restore":
			if msg.ToStep == nil {
				sub.enqueue(serverMessage{Type: "error", Message: "restore message missing to_step"})
				continue
			}
			results, err := sess.RestoreTo(ctx, actorID, actorKind, *msg.ToStep, subID)
			enqueueUndoRedoAcks(sub, results, err)

		case "cursor":
			if msg.Cursor == nil {
				sub.enqueue(serverMessage{Type: "error", Message: "cursor message missing cursor payload"})
				continue
			}
			sess.SetCursor(actorID, session.CursorEvent{BlockID: msg.Cursor.BlockID, Start: msg.Cursor.Start, End: msg.Cursor.End}, subID)

		default:
			sub.enqueue(serverMessage{Type: "error", Message: fmt.Sprintf("unknown message type %q", msg.Type)})
		}
	}
}

// enqueueUndoRedoAcks sends one "ack" frame per op an Undo/Redo call
// actually committed, in the order they applied — docs/api/collaboration.md
// §2.1: "from every other client's point of view, an undo is
// indistinguishable from the sender submitting N ordinary ops, because it
// is one." Each already-committed op still gets acked even when err is
// non-nil (a multi-op group that failed partway per that same section) —
// only the failure itself, not the partial success, is reported as an
// "error" frame.
func enqueueUndoRedoAcks(sub *connSubscriber, results []session.CommitResult, err error) {
	for _, r := range results {
		sub.enqueue(serverMessage{Type: "ack", Op: &r.Op, Boundaries: r.Boundaries})
	}
	if err != nil {
		sub.enqueue(serverMessage{Type: "error", Message: clientSafeMessage(err)})
	}
}

// clientSafeMessage decides what ApplyClientOp's error becomes on the
// wire. An earlier version sent err.Error() verbatim for every failure —
// fine for the recognized domain errors below (their messages are
// already client-facing by design, naming a conflicting edit or a
// missing block, nothing internal), but the same bare err.Error() also
// covered *unrecognized* errors: a WAL Append failure, for instance,
// wraps Go's own os package errors, which routinely embed the full
// filesystem path being written to. Anything not on this allowlist gets
// a generic message instead, with the real error only reaching the
// server's own log — the same "known errors get a specific status,
// everything else is INTERNAL with no detail" convention document-service's
// api.go (toStatus) and auth-service's api.go already use for their own
// gRPC/REST error boundaries; this is the WebSocket transport's
// equivalent chokepoint.
func clientSafeMessage(err error) string {
	switch {
	case errors.Is(err, session.ErrDenied),
		errors.Is(err, session.ErrUnknownBlock),
		errors.Is(err, session.ErrOutOfRange),
		errors.Is(err, ops.ErrUnknownAnchor),
		errors.Is(err, anchor.ErrOutOfBounds),
		errors.Is(err, doctext.ErrOutOfBounds),
		errors.Is(err, doctext.ErrInvertedRange):
		return err.Error()
	}
	var blockNotFound *documentcore.BlockNotFoundError
	var duplicateBlockID *documentcore.DuplicateBlockIDError
	var positionMismatch *documentcore.PositionMismatchError
	var precondition *documentcore.PreconditionError
	if errors.As(err, &blockNotFound) || errors.As(err, &duplicateBlockID) ||
		errors.As(err, &positionMismatch) || errors.As(err, &precondition) {
		return err.Error()
	}
	slog.Error("wsapi: op rejected by an unrecognized error", "err", err)
	return "internal error"
}
