package pagesaga

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	pagerepo "marginal/document-service/internal/pagerepo/gen"
)

// AckTimeout is how long StepSessionsReleased waits for
// collaboration-service's collab.page_released before giving up on the ack
// and moving on. Forward-only compensation (ARCHITECTURE.md §5): a
// collaboration-service that never answers must delay a purge, not block one
// forever, so the timeout advances the saga rather than failing it.
//
// Generous on purpose. The cost of waiting too long is a page that says
// "deleting" for longer than it needed to; the cost of waiting too little is
// purging rows under a live rope, which is the failure the step exists to
// prevent.
const AckTimeout = 2 * time.Minute

// Runner executes saga steps. It owns no state between calls — everything it
// needs to resume is in docs.page_deletions, which is what makes a crash
// mid-saga indistinguishable from a slow one.
type Runner struct {
	pool *pgxpool.Pool
	q    *pagerepo.Queries
	log  *slog.Logger

	// released reports whether collaboration-service has acked page. Supplied
	// by cmd/main.go from the NATS consumer rather than dialled here: the
	// saga's dependency is "has the ack arrived", not "how do we listen".
	released func(ctx context.Context, pageID pgtype.UUID) (bool, error)
}

func NewRunner(pool *pgxpool.Pool, log *slog.Logger, released func(context.Context, pgtype.UUID) (bool, error)) *Runner {
	return &Runner{pool: pool, q: pagerepo.New(pool), log: log, released: released}
}

// ErrAwaitingAck is returned by StepSessionsReleased while the ack is still
// outstanding and the timeout has not elapsed. It is not a failure: the
// sweeper leaves the saga in flight and tries again on its next pass, and it
// deliberately does NOT bump attempts, because waiting is not a retry.
var ErrAwaitingAck = errors.New("pagesaga: awaiting collab.page_released")

// Advance runs steps for one claimed saga until it finishes, blocks on an
// ack, or a step fails. It returns the number of steps completed.
//
// Steps are run one at a time, each recording itself before the next starts.
// Batching them into one transaction would be faster and would defeat the
// point: the recorded progress IS the resume point, so a step's effect and
// its record have to land together or the step is not resumable.
func (r *Runner) Advance(ctx context.Context, s Claimed) (int, error) {
	done := 0
	for _, step := range Remaining(s.StepsDone) {
		err := r.run(ctx, s.PageID, step)
		switch {
		case errors.Is(err, ErrAwaitingAck):
			// Still waiting, and not yet timed out. Leave the saga in flight
			// without recording a failure — an ack that has not arrived is
			// not an error, and counting it as one would make attempts read
			// as instability rather than latency.
			return done, nil
		case err != nil:
			reason := err.Error()
			if rerr := r.q.RecordPageDeletionFailure(ctx, pagerepo.RecordPageDeletionFailureParams{
				PageID:    s.PageID,
				LastError: &reason,
			}); rerr != nil {
				r.log.Error("pagesaga: recording failure", "page_id", s.PageID, "err", rerr)
			}
			return done, fmt.Errorf("pagesaga: step %s: %w", step, err)
		}

		if err := r.q.RecordPageDeletionStep(ctx, pagerepo.RecordPageDeletionStepParams{
			PageID: s.PageID, Step: step,
		}); err != nil {
			// The step's effect landed but its record did not. Safe, and the
			// exact case idempotency is for: the sweeper will run it again.
			return done, fmt.Errorf("pagesaga: recording step %s: %w", step, err)
		}
		done++
	}

	if err := r.q.CompletePageDeletion(ctx, s.PageID); err != nil {
		return done, fmt.Errorf("pagesaga: completing: %w", err)
	}
	r.log.Info("pagesaga: complete", "page_id", s.PageID, "steps", done, "attempts", s.Attempts)
	return done, nil
}

// run dispatches one step. Every branch must be safe to run twice — the
// sweeper resumes at the first unrecorded step, and a step that ran but
// crashed before recording itself runs again.
func (r *Runner) run(ctx context.Context, pageID pgtype.UUID, step string) error {
	switch step {
	case StepTreeDetached:
		// Already done inside DeletePage's own transaction, which is what
		// makes the page disappear from the tree immediately rather than
		// when the sweeper next runs. Recorded here so steps_done describes
		// the whole saga rather than only its asynchronous tail.
		return nil

	case StepLinksRewritten:
		path, err := r.pathOf(ctx, pageID)
		if err != nil {
			return err
		}
		n, err := r.q.DeleteSubtreeLinks(ctx, path)
		if err != nil {
			return err
		}
		r.log.Info("pagesaga: links rewritten", "page_id", pageID, "rows", n)
		return nil

	case StepSearchIndex:
		// Nothing to do, and that is a property of the schema rather than an
		// omission: docs.pages.search_vector and docs.blocks.search_vector are
		// GENERATED ALWAYS ... STORED columns (00004_search_vectors.sql), so
		// they are dropped by the same row delete that removes the page, and
		// excluded from results by the deleted_at filter meanwhile.
		//
		// The step stays in the list because the FTS index is not guaranteed
		// to remain a generated column forever — the moment it becomes a
		// separate index (or Tantivy, as originally planned), this is where
		// that work goes, and every historical saga will already have a slot
		// for it.
		return nil

	case StepSessionsReleased:
		ok, err := r.released(ctx, pageID)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		age, err := r.ageOf(ctx, pageID)
		if err != nil {
			return err
		}
		if age > AckTimeout {
			r.log.Warn("pagesaga: ack timed out, proceeding",
				"page_id", pageID, "waited", age.Round(time.Second))
			return nil
		}
		return ErrAwaitingAck

	case StepEmbeddingsPurged, StepBlobsReleased:
		// No vector store until v4.4.0, no object store until v4.2.0. These
		// complete immediately rather than being skipped, so the step list
		// keeps its shape when those land — see steps.go.
		return nil
	}
	return fmt.Errorf("pagesaga: unknown step %q", step)
}

func (r *Runner) pathOf(ctx context.Context, pageID pgtype.UUID) (string, error) {
	row, err := r.q.GetPage(ctx, pageID)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	return row.Path, nil
}

func (r *Runner) ageOf(ctx context.Context, pageID pgtype.UUID) (time.Duration, error) {
	var started time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT started_at FROM docs.page_deletions WHERE page_id = $1`, pageID).Scan(&started)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("saga row missing for %v", pageID)
	}
	if err != nil {
		return 0, err
	}
	return time.Since(started), nil
}
