package pages

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"marginal/document-service/internal/pagerepo/gen"
)

// pageFromRow builds a Page from the fields every pagerepo row type has —
// sqlc generates a distinct Go struct per query even when the column list
// is identical, so each query's Row type gets a one-line adapter below
// rather than duplicating this logic four times.
func pageFromRow(
	id, createdBy, parentID pgtype.UUID,
	title, path, sortKey, lifecycleState string,
	deletedAt, createdAt, updatedAt pgtype.Timestamptz,
) Page {
	return Page{
		ID:             PageID(fromPgUUID(id)),
		CreatedBy:      fromPgUUID(createdBy),
		Title:          title,
		ParentID:       fromPgUUIDPtr(parentID),
		Path:           path,
		SortKey:        sortKey,
		LifecycleState: LifecycleState(lifecycleState),
		DeletedAt:      fromPgTimestamptzPtr(deletedAt),
		CreatedAt:      fromPgTimestamptz(createdAt),
		UpdatedAt:      fromPgTimestamptz(updatedAt),
	}
}

func fromPgTimestamptz(ts pgtype.Timestamptz) time.Time { return ts.Time }

func fromPgTimestamptzPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func pageFromCreateRow(r pagerepo.CreatePageRow) Page {
	return pageFromRow(r.ID, r.CreatedBy, r.ParentID, r.Title, r.Path, r.SortKey, r.LifecycleState, r.DeletedAt, r.CreatedAt, r.UpdatedAt)
}

func pageFromGetRow(r pagerepo.GetPageRow) Page {
	return pageFromRow(r.ID, r.CreatedBy, r.ParentID, r.Title, r.Path, r.SortKey, r.LifecycleState, r.DeletedAt, r.CreatedAt, r.UpdatedAt)
}

func pageFromListRow(r pagerepo.ListPagesRow) Page {
	return pageFromRow(r.ID, r.CreatedBy, r.ParentID, r.Title, r.Path, r.SortKey, r.LifecycleState, r.DeletedAt, r.CreatedAt, r.UpdatedAt)
}

func pageFromRenameRow(r pagerepo.RenamePageRow) Page {
	return pageFromRow(r.ID, r.CreatedBy, r.ParentID, r.Title, r.Path, r.SortKey, r.LifecycleState, r.DeletedAt, r.CreatedAt, r.UpdatedAt)
}
