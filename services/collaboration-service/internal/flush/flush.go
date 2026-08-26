// Package flush is the batched WAL→Postgres drain loop RFC-002 §7 calls
// out as the thing that makes the write volume survivable: individual ops
// are already durable the instant internal/wal fsyncs them (the client is
// acknowledged then, not after this package does anything), so this loop
// is free to accumulate several of them and commit as one round trip
// instead of one-per-op.
//
// Go's idiomatic fit for RFC-002 §7's "crossbeam::ArrayQueue + hand-written
// Stream" is a bounded buffered channel plus one goroutine reading it —
// the channel bound is the backpressure (Enqueue blocks, doesn't drop, once
// full), and the goroutine is the flush loop itself. No custom Stream/Waker
// plumbing is needed; that machinery exists in Rust because Rust has no
// runtime scheduling goroutines for you.
package flush

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/opstore"
)

const (
	// DefaultQueueSize bounds how many un-flushed ops Enqueue will accept
	// before it starts blocking callers — real backpressure, not a
	// silently-dropping best-effort buffer (RFC-002 §7).
	DefaultQueueSize = 256
	// DefaultBatchSize mirrors RFC-002 §7's own worked example: "batched
	// ~20:1 [...] one primary handles comfortably."
	DefaultBatchSize = 20
	// DefaultInterval bounds how long an op can sit un-flushed when
	// traffic is too light to fill a batch on its own — a staleness
	// bound against Postgres, not a client-visible latency (the client
	// was already acknowledged at the WAL fsync).
	DefaultInterval = 200 * time.Millisecond
	// DefaultMaxAttempts caps how many times one batch is retried against
	// a failing Postgres before this loop gives up on it and moves on.
	// The op is not lost — the WAL already holds it durably — but nothing
	// in this repo's scope yet re-drives a WAL segment into a fresh Loop
	// after a give-up (docs/porting/PROGRESS.md names startup WAL replay
	// as still-open wiring); this constant exists so an outage doesn't
	// wedge the loop retrying one batch forever while later ops queue up
	// behind it.
	DefaultMaxAttempts = 5
)

var ErrStopped = errors.New("flush: loop is stopped")

// Loop drains LoggedOps into opstore.Repo.AppendBatch in batches, bounded
// by either count (BatchSize) or time (Interval), whichever comes first.
type Loop struct {
	repo     opstore.Repo
	pending  chan oplog.LoggedOp
	stopped  chan struct{}
	cancel   context.CancelFunc
	stopOnce sync.Once
	wg       sync.WaitGroup

	batchSize   int
	interval    time.Duration
	maxAttempts int
	onError     func(error)
}

type Option func(*Loop)

func WithQueueSize(n int) Option          { return func(l *Loop) { l.pending = make(chan oplog.LoggedOp, n) } }
func WithBatchSize(n int) Option          { return func(l *Loop) { l.batchSize = n } }
func WithInterval(d time.Duration) Option { return func(l *Loop) { l.interval = d } }
func WithMaxAttempts(n int) Option        { return func(l *Loop) { l.maxAttempts = n } }

// WithOnError is how a caller observes a batch that exhausted
// MaxAttempts — Loop itself never logs, per this codebase's convention of
// small interfaces at the point of use rather than an assumed logger.
func WithOnError(f func(error)) Option { return func(l *Loop) { l.onError = f } }

func New(repo opstore.Repo, opts ...Option) *Loop {
	l := &Loop{
		repo:        repo,
		pending:     make(chan oplog.LoggedOp, DefaultQueueSize),
		stopped:     make(chan struct{}),
		cancel:      func() {}, // safe no-op if Stop is ever called before Start
		batchSize:   DefaultBatchSize,
		interval:    DefaultInterval,
		maxAttempts: DefaultMaxAttempts,
		onError:     func(error) {},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Start launches the drain goroutine. ctx bounds the loop's own lifetime
// beyond an explicit Stop — cancelling ctx has the same drain-then-exit
// effect Stop does.
func (l *Loop) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	l.wg.Add(1)
	go l.run(runCtx)
}

// Enqueue blocks until op is accepted, the loop is stopped, or ctx is
// done — never drops op silently. A caller on the request path should
// pass a context with the same deadline it would apply to any other
// downstream call it's willing to wait on.
func (l *Loop) Enqueue(ctx context.Context, op oplog.LoggedOp) error {
	// Checked up front, not just as a select case below: once l.stopped
	// is closed, l.pending has no reader left (run's ctx.Done branch
	// already drained and returned), but a buffered channel with room
	// still looks "ready to send" to select — which would let this race
	// a closed-stopped case and occasionally report success for an op
	// that then sits forever unflushed. A stopped Loop must reject every
	// Enqueue after it, deterministically.
	select {
	case <-l.stopped:
		return ErrStopped
	default:
	}

	select {
	case l.pending <- op:
		return nil
	case <-l.stopped:
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop rejects further Enqueue calls, flushes every op already buffered
// (using a background context — a shutdown flush must not be aborted by
// the same ctx that's shutting everything down), and blocks until the
// drain goroutine has exited. Idempotent — safe to call more than once,
// or on a Loop that was never Start-ed (New sets a no-op l.cancel for
// exactly that case) — a second call, or one racing the first from a
// different goroutine, no longer panics on an already-closed l.stopped.
func (l *Loop) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopped)
		l.cancel()
		l.wg.Wait()
	})
}

func (l *Loop) run(ctx context.Context) {
	defer l.wg.Done()
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	var batch []oplog.LoggedOp
	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.flushWithRetry(context.Background(), batch); err != nil {
			l.onError(err)
		}
		batch = nil
	}

	for {
		select {
		case op := <-l.pending:
			batch = append(batch, op)
			if len(batch) >= l.batchSize {
				flushBatch()
			}
		case <-ticker.C:
			flushBatch()
		case <-ctx.Done():
			batch = append(batch, l.drainRemaining()...)
			flushBatch()
			return
		}
	}
}

// drainRemaining collects whatever is already sitting in the channel
// buffer without waiting for more to arrive — Stop must not hang waiting
// for a producer that has already been told (via l.stopped) to stop
// sending. Returns a fresh slice rather than mutating run's own `batch`
// through a pointer parameter: run's batch is already reached through
// flushBatch's closure capture, and reaching it a second way here too
// meant one mutable value aliased through both a closure and a pointer —
// exactly the shape a Rust port's borrow checker would refuse outright,
// worth not writing in the Go this is ported from either. The shutdown
// drain no longer respects batchSize mid-sweep (everything collected here
// flushes as one final batch) — a deliberate simplification: batchSize
// exists to bound steady-state latency, not shutdown correctness.
func (l *Loop) drainRemaining() []oplog.LoggedOp {
	var extra []oplog.LoggedOp
	for {
		select {
		case op := <-l.pending:
			extra = append(extra, op)
		default:
			return extra
		}
	}
}

func (l *Loop) flushWithRetry(ctx context.Context, batch []oplog.LoggedOp) error {
	var lastErr error
	backoff := 10 * time.Millisecond
	for attempt := 1; attempt <= l.maxAttempts; attempt++ {
		_, err := l.repo.AppendBatch(ctx, batch)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < l.maxAttempts {
			select {
			case <-ctx.Done():
				return fmt.Errorf("flush: batch of %d abandoned after %d attempts: %w", len(batch), attempt, ctx.Err())
			case <-time.After(backoff):
			}
			backoff *= 2
		}
	}
	return fmt.Errorf("flush: giving up on a batch of %d after %d attempts: %w", len(batch), l.maxAttempts, lastErr)
}
