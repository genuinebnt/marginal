// Package outboxpoll is the shared FOR UPDATE SKIP LOCKED claim-publish-
// mark polling loop DATA_MODEL.md's Outbox section prescribes —
// auth-service's and collaboration-service's own internal/outbox
// packages used to each independently implement this exact loop
// byte-for-byte (Poller/Run/pollOnce), differing only in which
// sqlc-generated Queries type claims/marks rows and in the JSON shape
// published to NATS. Postgres write + NATS publish is a dual write with
// no distributed transaction, so this two-phase shape — durable row
// first, publish later, at-least-once — is what DATA_MODEL.md's own
// outbox section prescribes, not a shortcut.
//
// Unlike this repo's other deliberate per-service duplicates
// (collaboration-service/outbox's wireEvent, pageop's spliceStringField),
// each of which independently owns its own half of a WIRE CONTRACT that
// could legitimately diverge, the polling mechanism itself has no
// contract to diverge from — every service's outbox table shares the
// same (id, aggregate_id, event_type, payload, published_at, created_at)
// shape (DATA_MODEL.md § Outbox), so one shared Poller is correct here,
// not premature abstraction. Each service still owns its own wireEvent
// shape and its own sqlc row type — Claim/MarkPublished/BuildEnvelope are
// the seams where a service plugs those in.
package outboxpoll

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// Row is one claimed outbox row, reduced to the fields the polling loop
// itself needs — every service's own sqlc-generated row type already has
// these four (DATA_MODEL.md's shared Outbox table shape); Claim converts
// from that service-specific type into this one at the boundary.
type Row struct {
	ID          pgtype.UUID
	AggregateID pgtype.UUID
	EventType   string
	Payload     []byte
}

// ClaimFunc claims up to limit unpublished rows within tx (FOR UPDATE
// SKIP LOCKED — see each service's own ClaimUnpublishedOutboxEvents
// query for the exact SQL) and converts them to Row.
type ClaimFunc func(ctx context.Context, tx pgx.Tx, limit int32) ([]Row, error)

// MarkPublishedFunc marks ids as published within the same tx Claim ran
// in.
type MarkPublishedFunc func(ctx context.Context, tx pgx.Tx, ids []pgtype.UUID) error

// BuildEnvelopeFunc marshals row into the JSON a service's own
// consumer(s) expect on the wire — each service's wireEvent shape
// differs (e.g. whether AggregateID is included), so the caller supplies
// this rather than the Poller assuming one fixed envelope shape.
type BuildEnvelopeFunc func(row Row) ([]byte, error)

// Poller periodically claims unpublished outbox rows and publishes them.
// One instance is safe to run per process; more than one replica running
// it concurrently is also safe (FOR UPDATE SKIP LOCKED).
type Poller struct {
	pool          *pgxpool.Pool
	nc            *nats.Conn
	claim         ClaimFunc
	markPublished MarkPublishedFunc
	buildEnvelope BuildEnvelopeFunc
	batchSize     int32
	interval      time.Duration
}

type Option func(*Poller)

func WithBatchSize(n int32) Option        { return func(p *Poller) { p.batchSize = n } }
func WithInterval(d time.Duration) Option { return func(p *Poller) { p.interval = d } }

const (
	defaultBatchSize = 20
	defaultInterval  = 500 * time.Millisecond
)

// New builds a Poller. claim/markPublished/buildEnvelope are the one
// service-specific seam — everything else (the transaction, the ticking,
// the retry-next-tick-on-publish-failure behavior) is shared.
func New(pool *pgxpool.Pool, nc *nats.Conn, claim ClaimFunc, markPublished MarkPublishedFunc, buildEnvelope BuildEnvelopeFunc, opts ...Option) *Poller {
	p := &Poller{
		pool: pool, nc: nc,
		claim: claim, markPublished: markPublished, buildEnvelope: buildEnvelope,
		batchSize: defaultBatchSize, interval: defaultInterval,
	}
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
		return fmt.Errorf("outboxpoll: poller: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := p.claim(ctx, tx, p.batchSize)
	if err != nil {
		return fmt.Errorf("outboxpoll: poller: claiming: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	published := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		envelope, err := p.buildEnvelope(row)
		if err != nil {
			return fmt.Errorf("outboxpoll: poller: building envelope for %s: %w", row.ID.Bytes, err)
		}
		if err := p.nc.Publish(row.EventType, envelope); err != nil {
			// Not fatal for the whole batch: roll back, so THIS row (and
			// the rest of the batch, since a partial commit would let an
			// un-published row's claim lapse without ever retrying) is
			// retried next tick rather than lost.
			return fmt.Errorf("outboxpoll: poller: publishing %s: %w", row.ID.Bytes, err)
		}
		published = append(published, row.ID)
	}

	if err := p.markPublished(ctx, tx, published); err != nil {
		return fmt.Errorf("outboxpoll: poller: marking published: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("outboxpoll: poller: commit: %w", err)
	}
	return nil
}
