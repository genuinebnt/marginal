package notify

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// SubjectUserRegistered is the NATS subject auth-service's outbox poller
// publishes to (DATA_MODEL.md §10's auth.user_registered topic) —
// mirrored independently in auth-service's own internal/outbox package,
// same reasoning as UserRegisteredEvent's doc comment.
const SubjectUserRegistered = "auth.user_registered"

// wireEvent is the envelope auth-service's poller actually publishes:
// the outbox row's own id (the dedup key) alongside its payload. Kept
// separate from UserRegisteredEvent so the envelope (id) and the payload
// (what the event is about) stay clearly distinct, the same split
// oplog.LoggedOp draws between its own id and its Op.
type wireEvent struct {
	ID      uuid.UUID `json:"id"`
	Payload []byte    `json:"payload"`
}

// Subscribe registers a durable-ish NATS subscription (plain core NATS,
// not JetStream — see the package README note below) that calls
// HandleUserRegistered for each message. Returns an unsubscribe func.
//
// Plain core NATS, not JetStream: core NATS has no redelivery/durability
// of its own — a message published while this service is down is lost,
// not queued. That's a real, deliberate gap at this repo's scope: a
// missed welcome notification is not the kind of event worth JetStream's
// added operational complexity (a stream, a consumer, ack semantics) for
// a Track 1 demo. Revisit if a notification type is ever added whose loss
// actually matters (CLAUDE.md's "ship minimal" — .agents/agents.md §2).
func Subscribe(nc *nats.Conn, repo Repo) (unsubscribe func() error, err error) {
	sub, err := nc.Subscribe(SubjectUserRegistered, func(msg *nats.Msg) {
		var evt wireEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			slog.Error("notify: decoding auth.user_registered envelope", "err", err)
			return
		}
		if err := HandleUserRegistered(context.Background(), repo, evt.ID, evt.Payload); err != nil {
			slog.Error("notify: handling auth.user_registered", "event_id", evt.ID, "err", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return sub.Unsubscribe, nil
}
