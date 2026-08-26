package pages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"marginal/document-service/internal/pagerepo/gen"
	"marginal/document-service/internal/sortkey"
)

var (
	ErrNotFound = errors.New("pages: not found")
	// ErrAnchorMismatch is docs/api/pages.md's "Anchor is not a child of
	// the named parent" — distinct from ErrNotFound (FAILED_PRECONDITION,
	// not NOT_FOUND, at the gRPC layer): the anchor page exists, just not
	// under the parent the caller named.
	ErrAnchorMismatch = errors.New("pages: anchor is not a child of the named parent")
)

// Repo is document-service's only port onto docs.pages — small, declared
// at its point of use, per CLOUD_PORTABILITY.md's ports-and-adapters
// convention. The Postgres implementation lives in this same file.
type Repo interface {
	Create(ctx context.Context, np NewPage) (Page, error)
	Get(ctx context.Context, id PageID) (Page, error)
	List(ctx context.Context, parentID *PageID, after string, limit int32) ([]Page, error)
	Rename(ctx context.Context, id PageID, title string) (Page, error)
	Delete(ctx context.Context, id PageID) error
}

type PostgresRepo struct {
	q *pagerepo.Queries
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{q: pagerepo.New(pool)}
}

func (r *PostgresRepo) Create(ctx context.Context, np NewPage) (Page, error) {
	id := PageID(uuid.Must(uuid.NewV7()))

	var parentPath string
	if np.ParentID != nil {
		parent, err := r.Get(ctx, *np.ParentID)
		if err != nil {
			return Page{}, fmt.Errorf("pages: resolving parent: %w", err)
		}
		parentPath = parent.Path
	}
	path := pathLabel(id)
	if parentPath != "" {
		path = parentPath + "." + path
	}

	sortKey, err := r.nextSortKey(ctx, np.ParentID, np.After)
	if err != nil {
		return Page{}, err
	}

	row, err := r.q.CreatePage(ctx, pagerepo.CreatePageParams{
		ID:        toPgUUID(uuid.UUID(id)),
		CreatedBy: toPgUUID(np.CreatedBy),
		Title:     np.Title,
		Column4:   path,
		SortKey:   sortKey,
		ParentID:  toPgUUIDPtr(np.ParentID),
	})
	if err != nil {
		return Page{}, fmt.Errorf("pages: create: %w", err)
	}
	return pageFromCreateRow(row), nil
}

// nextSortKey computes where a new page lands among its siblings: right
// after `after` (bounded above by after's current next sibling, if any),
// or at the very end if after is nil (docs/api/pages.md § Create).
func (r *PostgresRepo) nextSortKey(ctx context.Context, parentID, after *PageID) (string, error) {
	pgParent := toPgUUIDPtr(parentID)

	if after == nil {
		lastKey, err := r.q.LastSiblingSortKey(ctx, pgParent)
		if errors.Is(err, pgx.ErrNoRows) {
			lastKey = ""
		} else if err != nil {
			return "", fmt.Errorf("pages: last sibling: %w", err)
		}
		key, err := sortkey.Between(lastKey, "")
		if err != nil {
			return "", fmt.Errorf("pages: no room to append a sort key (needs rebalancing): %w", err)
		}
		return key, nil
	}

	anchor, err := r.Get(ctx, *after)
	if err != nil {
		return "", fmt.Errorf("pages: resolving after: %w", err)
	}
	anchorParentMatches := (anchor.ParentID == nil && parentID == nil) ||
		(anchor.ParentID != nil && parentID != nil && *anchor.ParentID == *parentID)
	if !anchorParentMatches {
		return "", ErrAnchorMismatch
	}

	nextKey, err := r.q.NextSiblingSortKey(ctx, pagerepo.NextSiblingSortKeyParams{
		ParentID:     pgParent,
		AfterSortKey: anchor.SortKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		nextKey = ""
	} else if err != nil {
		return "", fmt.Errorf("pages: next sibling: %w", err)
	}

	key, err := sortkey.Between(anchor.SortKey, nextKey)
	if err != nil {
		return "", fmt.Errorf("pages: no room to insert a sort key (needs rebalancing): %w", err)
	}
	return key, nil
}

func (r *PostgresRepo) Get(ctx context.Context, id PageID) (Page, error) {
	row, err := r.q.GetPage(ctx, toPgUUID(uuid.UUID(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("pages: get: %w", err)
	}
	return pageFromGetRow(row), nil
}

func (r *PostgresRepo) List(ctx context.Context, parentID *PageID, after string, limit int32) ([]Page, error) {
	var afterPtr *string
	if after != "" {
		afterPtr = &after
	}
	rows, err := r.q.ListPages(ctx, pagerepo.ListPagesParams{
		Limit:    limit,
		ParentID: toPgUUIDPtr(parentID),
		After:    afterPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("pages: list: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = pageFromListRow(row)
	}
	return out, nil
}

func (r *PostgresRepo) Rename(ctx context.Context, id PageID, title string) (Page, error) {
	row, err := r.q.RenamePage(ctx, pagerepo.RenamePageParams{ID: toPgUUID(uuid.UUID(id)), Title: title})
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("pages: rename: %w", err)
	}
	return pageFromRenameRow(row), nil
}

func (r *PostgresRepo) Delete(ctx context.Context, id PageID) error {
	n, err := r.q.SoftDeletePage(ctx, toPgUUID(uuid.UUID(id)))
	if err != nil {
		return fmt.Errorf("pages: delete: %w", err)
	}
	if n == 0 {
		// Idempotent per docs/api/pages.md § Delete: deleting an
		// already-deleted (or nonexistent) page still succeeds.
		return nil
	}
	return nil
}

// pathLabel is the LTREE label for a page: LTREE labels may only contain
// letters, digits, and underscores, so a UUID's hyphens don't survive —
// "p" + the hex digits, prefixed to guarantee a non-digit start.
func pathLabel(id PageID) string {
	return "p" + strings.ReplaceAll(uuid.UUID(id).String(), "-", "")
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgUUIDPtr(id *PageID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{Valid: false}
	}
	return toPgUUID(uuid.UUID(*id))
}

func fromPgUUID(id pgtype.UUID) uuid.UUID { return uuid.UUID(id.Bytes) }

func fromPgUUIDPtr(id pgtype.UUID) *PageID {
	if !id.Valid {
		return nil
	}
	p := PageID(fromPgUUID(id))
	return &p
}
