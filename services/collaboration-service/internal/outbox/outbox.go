// Package outbox is collab.outbox's publish side — opstore.go already
// writes a row per flushed op in the same transaction as the op itself
// (DATA_MODEL.md § Outbox), publishing it to NATS so document-service's
// blockproj consumer can materialise docs.blocks (docs/porting/PROGRESS.md).
// The actual claim-publish-mark polling loop lives in the shared
// marginal/outboxpoll module (this package used to duplicate
// auth-service's own internal/outbox byte-for-byte); this package
// supplies outboxpoll the two things genuinely specific to collab.outbox:
// how to claim/mark rows via collabrepo, and this service's own wireEvent
// shape (which, unlike auth-service's, carries AggregateID).
package outbox

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"marginal/outboxpoll"

	"marginal/collaboration-service/internal/collabrepo/gen"
)

// wireEvent is the envelope published to NATS. Unlike auth-service's own
// wireEvent, this carries AggregateID (the page id) explicitly — collab.
// outbox.payload is only ever the op itself (opstore.buildOpRow), which
// has no page_id inside it (an op is scoped to a page by the outbox row,
// not by its own JSON), so a consumer needs it carried alongside.
//
// Payload is json.RawMessage, not []byte: encoding/json base64-encodes a
// plain []byte field, so the wire message's "payload" would be an opaque
// base64 string rather than the readable nested JSON it actually holds —
// only "working" because every consumer today happens to be Go using the
// same package. json.RawMessage passes the bytes through verbatim, which
// document-service's blockproj.wireEvent mirrors for the same reason.
type wireEvent struct {
	ID          uuid.UUID       `json:"id"`
	AggregateID uuid.UUID       `json:"aggregate_id"`
	Payload     json.RawMessage `json:"payload"`
}

// NewPoller wires up the shared outboxpoll.Poller with collab.outbox's
// own claim/mark queries and wireEvent shape — the polling loop itself
// (transaction, ticking, retry-next-tick-on-publish-failure) lives in
// marginal/outboxpoll, identical to auth-service's own NewPoller.
func NewPoller(pool *pgxpool.Pool, nc *nats.Conn, opts ...outboxpoll.Option) *outboxpoll.Poller {
	claim := func(ctx context.Context, tx pgx.Tx, limit int32) ([]outboxpoll.Row, error) {
		rows, err := collabrepo.New(tx).ClaimUnpublishedOutboxEvents(ctx, limit)
		if err != nil {
			return nil, err
		}
		out := make([]outboxpoll.Row, len(rows))
		for i, r := range rows {
			out[i] = outboxpoll.Row{ID: r.ID, AggregateID: r.AggregateID, EventType: r.EventType, Payload: r.Payload}
		}
		return out, nil
	}
	markPublished := func(ctx context.Context, tx pgx.Tx, ids []pgtype.UUID) error {
		return collabrepo.New(tx).MarkOutboxEventsPublished(ctx, ids)
	}
	buildEnvelope := func(row outboxpoll.Row) ([]byte, error) {
		return json.Marshal(wireEvent{ID: row.ID.Bytes, AggregateID: row.AggregateID.Bytes, Payload: row.Payload})
	}
	return outboxpoll.New(pool, nc, claim, markPublished, buildEnvelope, opts...)
}
