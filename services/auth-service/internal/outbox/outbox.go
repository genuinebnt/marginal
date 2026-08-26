// Package outbox is auth-service's own outbox (DATA_MODEL.md § Outbox) —
// the Postgres write happens in the same transaction as whatever domain
// change produced it (Register, for auth.user_registered), and a
// separate Poller publishes it to NATS afterward. Postgres write + NATS
// publish is a dual write with no distributed transaction, so this
// two-phase shape — durable row first, publish later, at-least-once — is
// what DATA_MODEL.md's own outbox section prescribes, not a shortcut.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

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
type wireEvent struct {
	ID      uuid.UUID `json:"id"`
	Payload []byte    `json:"payload"`
}

// Poller periodically claims unpublished outbox rows and publishes them.
// One poller instance is safe to run per process; more than one replica
// running it concurrently is also safe (FOR UPDATE SKIP LOCKED — see
// ClaimUnpublishedOutboxEvents's own doc comment).
type Poller struct {
	pool      *pgxpool.Pool
	nc        *nats.Conn
	batchSize int32
	interval  time.Duration
}

type Option func(*Poller)

func WithBatchSize(n int32) Option        { return func(p *Poller) { p.batchSize = n } }
func WithInterval(d time.Duration) Option { return func(p *Poller) { p.interval = d } }

const (
	defaultBatchSize = 20
	defaultInterval  = 500 * time.Millisecond
)

func NewPoller(pool *pgxpool.Pool, nc *nats.Conn, opts ...Option) *Poller {
	p := &Poller{pool: pool, nc: nc, batchSize: defaultBatchSize, interval: defaultInterval}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run blocks, polling until ctx is done. Call it in its own goroutine.
func (p *Poller) Run(ctx context.Context, onError func(error)) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.pollOnce(ctx); err != nil {
				onError(err)
			}
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("outbox: poller: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := authrepo.New(tx)

	rows, err := q.ClaimUnpublishedOutboxEvents(ctx, p.batchSize)
	if err != nil {
		return fmt.Errorf("outbox: poller: claiming: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	published := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		envelope, err := json.Marshal(wireEvent{ID: row.ID.Bytes, Payload: row.Payload})
		if err != nil {
			return fmt.Errorf("outbox: poller: marshaling envelope for %s: %w", row.ID.Bytes, err)
		}
		if err := p.nc.Publish(row.EventType, envelope); err != nil {
			// Not fatal for the whole batch: roll back, so THIS row (and
			// the rest of the batch, since a partial commit would let an
			// un-published row's claim lapse without ever retrying) is
			// retried next tick rather than lost.
			return fmt.Errorf("outbox: poller: publishing %s: %w", row.ID.Bytes, err)
		}
		published = append(published, row.ID)
	}

	if err := q.MarkOutboxEventsPublished(ctx, published); err != nil {
		return fmt.Errorf("outbox: poller: marking published: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("outbox: poller: commit: %w", err)
	}
	return nil
}
