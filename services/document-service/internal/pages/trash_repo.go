package pages

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"marginal/document-service/internal/pagerepo/gen"
)

// DeletePreview is what a delete would take with it, before it takes it.
//
// The screen this feeds (§ 23c) exists because deleting is a STATE, not an
// event — and the first half of that argument is that you can see what the
// state will cost before entering it.
type DeletePreview struct {
	// Descendants is the LTREE subtree, excluding the page itself.
	Descendants []Page
	// Referrers are pages OUTSIDE the subtree whose [[links]] point into it —
	// the links that would dangle. Distinct by source: a page linking three
	// times is one page to warn about, not three.
	Referrers []Page
	// Blocks is how much content goes, over the projection — the number a
	// person would see, not the op count behind it.
	Blocks int32
}

func (r *PostgresRepo) PreviewDelete(ctx context.Context, id PageID) (DeletePreview, error) {
	pg := toPgUUID(uuid.UUID(id))

	descRows, err := r.q.ListDescendants(ctx, pg)
	if err != nil {
		return DeletePreview{}, fmt.Errorf("pages: preview descendants: %w", err)
	}
	refRows, err := r.q.ListReferrers(ctx, pg)
	if err != nil {
		return DeletePreview{}, fmt.Errorf("pages: preview referrers: %w", err)
	}
	blocks, err := r.q.CountBlocksInSubtree(ctx, pg)
	if err != nil {
		return DeletePreview{}, fmt.Errorf("pages: preview blocks: %w", err)
	}

	out := DeletePreview{Blocks: blocks}
	for _, d := range descRows {
		out.Descendants = append(out.Descendants, pageFromRow(
			d.ID, d.CreatedBy, d.ParentID, d.Title, d.Path, d.SortKey,
			d.LifecycleState, d.DeletedAt, d.CreatedAt, d.UpdatedAt))
	}
	for _, f := range refRows {
		out.Referrers = append(out.Referrers, pageFromRow(
			f.ID, f.CreatedBy, f.ParentID, f.Title, f.Path, f.SortKey,
			f.LifecycleState, f.DeletedAt, f.CreatedAt, f.UpdatedAt))
	}
	return out, nil
}

// TrashEntry is one deleted page, with the saga's progress when there is any.
//
// A finished saga has no progress to report, so a nil Progress is how a
// caller tells "deleted, restorable" from "deleting, mid-saga" without
// re-reading lifecycle_state.
type TrashEntry struct {
	Page     Page
	PurgeAt  time.Time
	Progress *SagaProgress
}

type SagaProgress struct {
	StepsDone []string
	Attempts  int32
	LastError string
}

func (r *PostgresRepo) ListTrash(ctx context.Context, window time.Duration, limit, offset int32) ([]TrashEntry, int32, error) {
	// The purge window is passed in rather than stored per row: changing it
	// must move every pending purge, and a stored purge_at would need a
	// backfill to do that (DATA_MODEL.md § Page deletions).
	rows, err := r.q.ListTrash(ctx, pagerepo.ListTrashParams{
		PurgeWindow: pgtype.Interval{Microseconds: window.Microseconds(), Valid: true},
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("pages: list trash: %w", err)
	}
	total, err := r.q.CountTrash(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("pages: count trash: %w", err)
	}

	out := make([]TrashEntry, 0, len(rows))
	for _, row := range rows {
		e := TrashEntry{
			Page: pageFromRow(row.ID, row.CreatedBy, row.ParentID, row.Title, row.Path,
				row.SortKey, row.LifecycleState, row.DeletedAt, row.CreatedAt, row.UpdatedAt),
		}
		if row.PurgeAt.Valid {
			e.PurgeAt = row.PurgeAt.Time
		}
		// CompletedAt set means the saga is done and has nothing left to
		// report; steps_done nil means there is no saga row at all.
		if row.StepsDone != nil && !row.CompletedAt.Valid {
			p := SagaProgress{StepsDone: row.StepsDone}
			if row.Attempts != nil {
				p.Attempts = *row.Attempts
			}
			if row.LastError != nil {
				p.LastError = *row.LastError
			}
			e.Progress = &p
		}
		out = append(out, e)
	}
	return out, int32(total), nil
}

// Restore is the inverse of the FIRST saga step and nothing else. The later
// steps are not undone, they are re-derived: page_links and the FTS index are
// projections that rebuild from the op log, and the op log was sealed, never
// deleted.
func (r *PostgresRepo) Restore(ctx context.Context, id PageID) (int64, error) {
	pg := toPgUUID(uuid.UUID(id))
	n, err := r.q.RestorePageAndSubtree(ctx, pg)
	if err != nil {
		return 0, fmt.Errorf("pages: restore: %w", err)
	}
	if n == 0 {
		return 0, nil
	}
	// The saga row goes rather than being marked: a restored page deleted
	// again is a NEW saga, and keeping the old row would make `attempts` and
	// `steps_done` describe two different operations.
	if err := r.q.ClearPageDeletion(ctx, pg); err != nil {
		return 0, fmt.Errorf("pages: clear deletion: %w", err)
	}
	return n, nil
}
