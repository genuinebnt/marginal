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
	// collaboration-service's envelope carries the page id alongside the
	// payload (its outbox rows are scoped to a page by the row, not by
	// their own JSON). Decoded but unused here: a mention's payload names
	// its own page, so this service does not need the envelope's copy —
	// and a field that is present on the wire is better declared and
	// ignored than silently dropped by a struct that cannot see it.
	AggregateID uuid.UUID `json:"aggregate_id"`
}

// handleEventTimeout bounds one HandleUserRegistered call — see
// document-service/internal/blockproj's identical constant and doc
// comment for why a NATS delivery goroutine must never block forever on
// a stuck DB write.
const handleEventTimeout = 10 * time.Second

// Subscribe registers a durable-ish NATS subscription (plain core NATS,
// not JetStream — see the package README note below) that calls
// HandleUserRegistered for each message. Returns the *nats.Subscription
// itself, not a hand-rolled unsubscribe closure wrapping exactly one of
// its methods — callers get the real object (Unsubscribe, but also
// IsValid, Pending, etc.) for free instead of a narrower one-off shape.
//
// Plain core NATS, not JetStream: core NATS has no redelivery/durability
// of its own — a message published while this service is down is lost,
// not queued. That's a real, deliberate gap at this repo's scope: a
// missed welcome notification is not the kind of event worth JetStream's
// added operational complexity (a stream, a consumer, ack semantics) for
// a Track 1 demo. Revisit if a notification type is ever added whose loss
// actually matters (CLAUDE.md's "ship minimal" — .agents/agents.md §2).
func Subscribe(nc *nats.Conn, repo Repo) (*nats.Subscription, error) {
	return nc.Subscribe(SubjectUserRegistered, func(msg *nats.Msg) {
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
}

// SubscribeMentions registers the collab.comment_mentioned subscription
// (v3.3.0). Separate from Subscribe rather than a subject parameter: the
// two topics decode to different payloads and are handled by different
// functions, so the only thing a merged version would share is the word
// "subscribe".
//
// The core-NATS caveat on Subscribe applies here and bites harder. A
// welcome notification lost while this service was down is a greeting
// nobody misses; a MENTION lost that way is somebody being told nothing
// while believing they were asked a question. That is the notification
// kind whose loss "actually matters" in Subscribe's own words, and it is
// the concrete argument for JetStream this repo did not previously have —
// recorded in docs/api/notifications.md rather than silently accepted.
func SubscribeMentions(nc *nats.Conn, repo Repo) (*nats.Subscription, error) {
	return nc.Subscribe(MentionEvent, func(msg *nats.Msg) {
		var evt wireEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			slog.Error("notify: decoding "+MentionEvent+" envelope", "err", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), handleEventTimeout)
		defer cancel()
		if err := HandleMention(ctx, repo, evt.ID, evt.Payload); err != nil {
			slog.Error("notify: handling "+MentionEvent, "event_id", evt.ID, "err", err)
		}
	})
}
