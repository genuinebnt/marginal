// Package outbox is auth-service's own outbox (DATA_MODEL.md § Outbox) —
// the Postgres write happens in the same transaction as whatever domain
// change produced it (Register, for auth.user_registered). The actual
// claim-publish-mark polling loop lives in the shared marginal/outboxpoll
// module (collaboration-service's own internal/outbox used to duplicate
// it byte-for-byte); this package supplies outboxpoll the three things
// that are genuinely specific to auth.outbox: how to claim/mark rows via
// authrepo, and this service's own wireEvent shape.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"marginal/outboxpoll"

	"marginal/auth-service/internal/authrepo/gen"
)

// EventUserRegistered is DATA_MODEL.md §10's auth.user_registered topic —
// the one event this repo's Track 1 scope can actually produce (every
// other auth.* topic needs sharing/RBAC/deactivation, none in scope).
const EventUserRegistered = "auth.user_registered"

// UserRegisteredPayload mirrors notification-service's own
// notify.UserRegisteredEvent field-for-field — kept as an independent
// definition rather than a shared module; see that type's own doc
// comment for why.
type UserRegisteredPayload struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
}

// WriteUserRegistered inserts the auth.user_registered outbox row in the
// SAME transaction as the caller's own user insert (authservice.Register)
// — q must be a *authrepo.Queries scoped to that transaction via WithTx,
// never the pool-level Queries, or the event could be durable without the
// user actually existing (or vice versa) on a partial failure.
func WriteUserRegistered(ctx context.Context, q *authrepo.Queries, userID uuid.UUID, email, displayName string) error {
	payload, err := json.Marshal(UserRegisteredPayload{UserID: userID, Email: email, DisplayName: displayName})
	if err != nil {
		return fmt.Errorf("outbox: marshaling user_registered payload: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("outbox: generating event id: %w", err)
	}
	_, err = q.InsertOutboxEvent(ctx, authrepo.InsertOutboxEventParams{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		AggregateID: pgtype.UUID{Bytes: userID, Valid: true},
		EventType:   EventUserRegistered,
		Payload:     payload,
	})
	if err != nil {
		return fmt.Errorf("outbox: inserting user_registered event: %w", err)
	}
	return nil
}

// wireEvent is the envelope published to NATS — the outbox row's own id
// (the dedup key a consumer keys idempotency on) alongside its payload.
// Mirrors notification-service's own internal/notify wireEvent shape;
// see that type's doc comment for why this isn't a shared module.
//
// Payload is json.RawMessage, not []byte: encoding/json base64-encodes a
// plain []byte field, so the wire message's "payload" would be an opaque
// base64 string rather than the readable nested JSON it actually holds —
// only "working" because every consumer today happens to be Go using the
// same package. json.RawMessage passes the bytes through verbatim.
type wireEvent struct {
	ID      uuid.UUID       `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// NewPoller wires up the shared outboxpoll.Poller with auth.outbox's own
// claim/mark queries and wireEvent shape — the polling loop itself
// (transaction, ticking, retry-next-tick-on-publish-failure) lives in
// marginal/outboxpoll, identical to collaboration-service's own NewPoller.
func NewPoller(pool *pgxpool.Pool, nc *nats.Conn, opts ...outboxpoll.Option) *outboxpoll.Poller {
	claim := func(ctx context.Context, tx pgx.Tx, limit int32) ([]outboxpoll.Row, error) {
		rows, err := authrepo.New(tx).ClaimUnpublishedOutboxEvents(ctx, limit)
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
		return authrepo.New(tx).MarkOutboxEventsPublished(ctx, ids)
	}
	buildEnvelope := func(row outboxpoll.Row) ([]byte, error) {
		return json.Marshal(wireEvent{ID: row.ID.Bytes, Payload: row.Payload})
	}
	return outboxpoll.New(pool, nc, claim, markPublished, buildEnvelope, opts...)
}
