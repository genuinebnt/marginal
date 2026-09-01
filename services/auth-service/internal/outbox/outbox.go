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
	"time"

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

// The two membership events (DATA_MODEL.md §10, reserved there long before
// v3.1.0 built them). document-service consumes both to maintain
// docs.space_members, the projection it filters reads against.
const (
	EventRoleGranted = "auth.role_granted"
	EventRoleRevoked = "auth.role_revoked"
)

// RolePayload carries enough for a consumer to apply the change without
// asking anything back.
//
// GrantedAt is the decisive field. Core NATS gives no ordering guarantee
// across publishes, so two events for one user can arrive out of order —
// a consumer that took last-write-by-arrival would let a revoke be undone
// by a grant that happened before it. Role is empty on a revoke.
type RolePayload struct {
	UserID    uuid.UUID `json:"user_id"`
	SpaceID   uuid.UUID `json:"space_id"`
	Role      string    `json:"role,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
}

// WriteRoleEvent inserts a membership event in the SAME transaction as the
// membership row itself — the write and its announcement cannot disagree.
// q must be scoped to that transaction via WithTx, never the pool-level
// Queries (see WriteUserRegistered for the failure that prevents).
func WriteRoleEvent(ctx context.Context, q *authrepo.Queries, eventType string, p RolePayload) error {
	payload, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("outbox: marshaling %s payload: %w", eventType, err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("outbox: generating event id: %w", err)
	}
	// AggregateID is the USER, matching DATA_MODEL.md §10's partition key
	// for both topics — events about one person stay in order relative to
	// each other, which is the ordering that actually matters here.
	if _, err := q.InsertOutboxEvent(ctx, authrepo.InsertOutboxEventParams{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		AggregateID: pgtype.UUID{Bytes: p.UserID, Valid: true},
		EventType:   eventType,
		Payload:     payload,
	}); err != nil {
		return fmt.Errorf("outbox: inserting %s event: %w", eventType, err)
	}
	return nil
}

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
