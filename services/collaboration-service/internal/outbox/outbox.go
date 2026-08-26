// Package outbox is collab.outbox's publish side — opstore.go already
// writes a row per flushed op in the same transaction as the op itself
// (DATA_MODEL.md § Outbox); Poller is the piece that was missing until
// now, periodically claiming unpublished rows and publishing them to NATS
// so document-service's blockproj consumer can materialise docs.blocks
// (docs/porting/PROGRESS.md). Mirrors auth-service's own internal/outbox
// package field-for-field — same at-least-once, FOR UPDATE SKIP LOCKED
// pattern, independently duplicated rather than shared for the same
// reason notification-service's wireEvent isn't shared with auth-service's
// (see either's own doc comment): each side of an event contract owns its
// half, and the contract is the JSON shape, not a Go type.
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

	"marginal/collaboration-service/internal/collabrepo/gen"
)

// wireEvent is the envelope published to NATS. Unlike auth-service's own
// wireEvent, this carries AggregateID (the page id) explicitly — collab.
// outbox.payload is only ever the op itself (opstore.buildOpRow), which
// has no page_id inside it (an op is scoped to a page by the outbox row,
// not by its own JSON), so a consumer needs it carried alongside.
type wireEvent struct {
	ID          uuid.UUID `json:"id"`
	AggregateID uuid.UUID `json:"aggregate_id"`
	Payload     []byte    `json:"payload"`
}

// Poller periodically claims unpublished collab.outbox rows and publishes
// them under their own event_type subject (opstore.OutboxEventOpAppended
// today — the only event this repo produces). One instance is safe to
// run per process; more than one replica running it concurrently is also
// safe (FOR UPDATE SKIP LOCKED).
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
	q := collabrepo.New(tx)

	rows, err := q.ClaimUnpublishedOutboxEvents(ctx, p.batchSize)
	if err != nil {
		return fmt.Errorf("outbox: poller: claiming: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	published := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		envelope, err := json.Marshal(wireEvent{ID: row.ID.Bytes, AggregateID: row.AggregateID.Bytes, Payload: row.Payload})
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
