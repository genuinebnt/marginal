package wsapi

import (
	"context"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"marginal/collaboration-service/internal/session"
)

// connSubscriber is one connection's outbound side — the session.Subscriber
// this package registers, plus the single writer goroutine every write to
// the underlying *websocket.Conn must funnel through (concurrent writes to
// one Conn are unsafe, and Session.ApplyClientOp calls Deliver from
// whichever goroutine is handling a *different* client's op while holding
// the session's own lock — that call must never block on this
// connection's socket).
type connSubscriber struct {
	out    chan serverMessage
	cancel context.CancelFunc
}

func newConnSubscriber(cancel context.CancelFunc) *connSubscriber {
	return &connSubscriber{out: make(chan serverMessage, outboxBufferSize), cancel: cancel}
}

// Deliver implements session.Subscriber.
func (c *connSubscriber) Deliver(r session.CommitResult) {
	c.enqueue(serverMessage{Type: "broadcast", Op: &r.Op, Boundaries: r.Boundaries})
}

// DeliverPresence implements session.Subscriber.
func (c *connSubscriber) DeliverPresence(e session.PresenceEvent) {
	joined := e.Joined
	c.enqueue(serverMessage{Type: "presence", ActorID: e.ActorID.String(), Joined: &joined})
}

// DeliverCursor implements session.Subscriber.
func (c *connSubscriber) DeliverCursor(e session.CursorEvent) {
	wire := toCursorWire(e)
	c.enqueue(serverMessage{Type: "cursor", Cursor: &wire})
}

// enqueue never blocks: a full buffer means this connection's reader
// isn't keeping up, and the correct response is to disconnect it (forcing
// a reconnect, which replays cleanly via session.Manager.Get), not to
// stall whichever goroutine is trying to deliver to it — that goroutine
// may be holding the whole session's lock on behalf of every other
// client.
func (c *connSubscriber) enqueue(msg serverMessage) {
	select {
	case c.out <- msg:
	default:
		c.cancel()
	}
}

func (c *connSubscriber) writeLoop(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case msg := <-c.out:
			if err := wsjson.Write(ctx, conn, msg); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
