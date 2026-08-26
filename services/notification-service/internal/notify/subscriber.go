package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

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
//
// Payload is json.RawMessage, not []byte: encoding/json base64-encodes a
// plain []byte field, so the wire message's "payload" would be an opaque
// base64 string rather than the readable nested JSON it actually holds —
// only "working" because every consumer today happens to be Go using the
// same package. json.RawMessage passes the bytes through verbatim,
// matching auth-service's own wireEvent.
type wireEvent struct {
	ID      uuid.UUID       `json:"id"`
	Payload json.RawMessage `json:"payload"`
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
// handleEventTimeout bounds one HandleUserRegistered call — see
// document-service/internal/blockproj's identical constant and doc
// comment for why a NATS delivery goroutine must never block forever on
// a stuck DB write.
const handleEventTimeout = 10 * time.Second

func Subscribe(nc *nats.Conn, repo Repo) (unsubscribe func() error, err error) {
	sub, err := nc.Subscribe(SubjectUserRegistered, func(msg *nats.Msg) {
		var evt wireEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			slog.Error("notify: decoding auth.user_registered envelope", "err", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), handleEventTimeout)
		defer cancel()
		if err := HandleUserRegistered(ctx, repo, evt.ID, evt.Payload); err != nil {
			slog.Error("notify: handling auth.user_registered", "event_id", evt.ID, "err", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return sub.Unsubscribe, nil
}
