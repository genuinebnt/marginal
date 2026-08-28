package pagesaga

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	pagerepo "marginal/document-service/internal/pagerepo/gen"
)

// Claimed is one in-flight saga, claimed for this pass.
type Claimed struct {
	PageID    pgtype.UUID
	StepsDone []string
	Attempts  int32
}

// Sweeper resumes in-flight sagas. It is the whole resume mechanism: nothing
// re-drives a saga after a crash except this loop noticing a row with no
// completed_at and running Remaining(steps_done).
//
// Deliberately a poller rather than an in-process retry after a failed step.
// A retry loop only survives the process that started it, and the case this
// has to handle is precisely the one where that process died — which is why
// ui-mockups § 23c can honestly claim "the process died mid-delete at step 3
// and resumed there, not from the start."
type Sweeper struct {
	pool   *pgxpool.Pool
	q      *pagerepo.Queries
	runner *Runner
	log    *slog.Logger

	Interval  time.Duration
	BatchSize int32
}

func NewSweeper(pool *pgxpool.Pool, runner *Runner, log *slog.Logger) *Sweeper {
	return &Sweeper{
		pool: pool, q: pagerepo.New(pool), runner: runner, log: log,
		// Short enough that a delete finishes while the user is still looking
		// at the trash screen; long enough that an idle instance is not
		// polling an empty table hundreds of times a minute.
		Interval:  2 * time.Second,
		BatchSize: 16,
	}
}

// Run sweeps until ctx is cancelled. One pass never returns an error — a
// failing saga is recorded on its own row and retried next pass, because one
// stuck page must not stop every other delete on the instance.
func (s *Sweeper) Run(ctx context.Context) {
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.pass(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("pagesaga: sweep", "err", err)
			}
		}
	}
}

// pass claims a batch and advances each. The claim and the work share one
// transaction so FOR UPDATE SKIP LOCKED actually holds for the duration —
// claiming in one transaction and working in another would let a second
// instance claim the same saga the moment the first committed.
func (s *Sweeper) pass(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pagesaga: sweep: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := s.q.WithTx(tx).ClaimPageDeletions(ctx, s.BatchSize)
	if err != nil {
		return fmt.Errorf("pagesaga: sweep: claim: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	for _, row := range rows {
		c := Claimed{PageID: row.PageID, StepsDone: row.StepsDone, Attempts: row.Attempts}
		if _, err := s.runner.Advance(ctx, c); err != nil {
			// Recorded on the saga's own row by Advance. Logged and skipped
			// rather than returned, so one poisoned saga cannot hold up the
			// rest of the batch — forward-only means "keep going", including
			// past this one.
			s.log.Warn("pagesaga: advance", "page_id", row.PageID, "err", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pagesaga: sweep: commit: %w", err)
	}
	return nil
}

// Resumed reports sagas that have been through more than one attempt — what
// ui-mockups § 23c shows as "resumed once". Exposed for the trash screen
// rather than computed there, since "resumed" is attempts > 1 and that
// comparison should live next to the code that does the bumping.
func Resumed(attempts int32) bool { return attempts > 1 }

var _ = pgx.ErrNoRows
