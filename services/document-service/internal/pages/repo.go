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
	// ErrCycle is "Reparent under self or own descendant"
	// (docs/api/pages.md § Status codes) — FAILED_PRECONDITION.
	ErrCycle = errors.New("pages: cannot reparent a page under itself or its own descendant")
)

// ParentChange models ReparentPageRequest's three-way optional parent_id
// (docs/api/pages.md § Reparent): the field can be absent ("leave the
// parent alone"), present and empty ("promote to root"), or present and
// naming a page. A plain *PageID can't distinguish the first two.
type ParentChange struct {
	Change   bool // false: leave the parent alone
	ParentID *PageID
}

// Repo is document-service's only port onto docs.pages — small, declared
// at its point of use, per CLOUD_PORTABILITY.md's ports-and-adapters
// convention. The Postgres implementation lives in this same file.
//
// Every method takes owner explicitly rather than trusting a page's
// created_by implicitly: it's the WHERE-clause filter that makes "only I
// can read or change my pages" true (docs/api/pages.md, user story A-04)
// — a page that exists but belongs to someone else is indistinguishable
// from one that doesn't exist, at the query level, not via a check
// bolted on after fetching it.
type Repo interface {
	Create(ctx context.Context, np NewPage) (Page, error)
	Get(ctx context.Context, owner uuid.UUID, id PageID) (Page, error)
	List(ctx context.Context, owner uuid.UUID, parentID *PageID, after string, limit int32) ([]Page, error)
	Rename(ctx context.Context, owner uuid.UUID, id PageID, title string) (Page, error)
	Reparent(ctx context.Context, owner uuid.UUID, id PageID, parent ParentChange, after *PageID) (Page, error)
	Delete(ctx context.Context, owner uuid.UUID, id PageID) error
}

type PostgresRepo struct {
	q    *pagerepo.Queries
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{q: pagerepo.New(pool), pool: pool}
}

// Create's owner is np.CreatedBy — a new page's parent and its "after"
// anchor must both already belong to that same actor: you can only nest
// under, or position relative to, your own pages. Nothing in this repo
// yet models sharing a page tree between actors (RBAC is ADR-009,
// deferred), so cross-owner nesting has no meaning to accept.
func (r *PostgresRepo) Create(ctx context.Context, np NewPage) (Page, error) {
	id := PageID(uuid.Must(uuid.NewV7()))

	var parentPath string
	if np.ParentID != nil {
		parent, err := r.get(ctx, r.q, np.CreatedBy, *np.ParentID)
		if err != nil {
			return Page{}, fmt.Errorf("pages: resolving parent: %w", err)
		}
		parentPath = parent.Path
	}
	path := childPath(parentPath, id)

	sortKey, err := r.nextSortKey(ctx, r.q, np.CreatedBy, np.ParentID, np.After, nil)
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

// nextSortKey computes where a page lands among its siblings under
// parentID: right after `after` (bounded above by after's current next
// sibling, if any), or at the very end if after is nil (docs/api/pages.md
// § Create). excludeID (only non-nil from Reparent) leaves the page being
// moved out of its own neighbor search. owner scopes the after-anchor
// lookup the same as every other read (see Repo's doc comment).
func (r *PostgresRepo) nextSortKey(ctx context.Context, q *pagerepo.Queries, owner uuid.UUID, parentID, after, excludeID *PageID) (string, error) {
	pgParent := toPgUUIDPtr(parentID)
	pgExclude := toPgUUIDPtr(excludeID)

	if after == nil {
		lastKey, err := q.LastSiblingSortKey(ctx, pagerepo.LastSiblingSortKeyParams{ParentID: pgParent, ExcludeID: pgExclude})
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

	anchor, err := r.get(ctx, q, owner, *after)
	if err != nil {
		return "", fmt.Errorf("pages: resolving after: %w", err)
	}
	anchorParentMatches := (anchor.ParentID == nil && parentID == nil) ||
		(anchor.ParentID != nil && parentID != nil && *anchor.ParentID == *parentID)
	if !anchorParentMatches {
		return "", ErrAnchorMismatch
	}

	nextKey, err := q.NextSiblingSortKey(ctx, pagerepo.NextSiblingSortKeyParams{
		ParentID:     pgParent,
		AfterSortKey: anchor.SortKey,
		ExcludeID:    pgExclude,
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

func (r *PostgresRepo) Get(ctx context.Context, owner uuid.UUID, id PageID) (Page, error) {
	return r.get(ctx, r.q, owner, id)
}

func (r *PostgresRepo) get(ctx context.Context, q *pagerepo.Queries, owner uuid.UUID, id PageID) (Page, error) {
	row, err := q.GetPage(ctx, pagerepo.GetPageParams{ID: toPgUUID(uuid.UUID(id)), CreatedBy: toPgUUID(owner)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("pages: get: %w", err)
	}
	return pageFromGetRow(row), nil
}

func (r *PostgresRepo) List(ctx context.Context, owner uuid.UUID, parentID *PageID, after string, limit int32) ([]Page, error) {
	var afterPtr *string
	if after != "" {
		afterPtr = &after
	}
	rows, err := r.q.ListPages(ctx, pagerepo.ListPagesParams{
		Limit:     limit,
		CreatedBy: toPgUUID(owner),
		ParentID:  toPgUUIDPtr(parentID),
		After:     afterPtr,
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

func (r *PostgresRepo) Rename(ctx context.Context, owner uuid.UUID, id PageID, title string) (Page, error) {
	row, err := r.q.RenamePage(ctx, pagerepo.RenamePageParams{
		ID:        toPgUUID(uuid.UUID(id)),
		Title:     title,
		CreatedBy: toPgUUID(owner),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("pages: rename: %w", err)
	}
	return pageFromRenameRow(row), nil
}

// Reparent moves id to a new parent and/or position. Every descendant's
// path is rewritten in the same transaction as the page's own row
// (docs/api/pages.md § Reparent: "a concurrent reader sees all old paths
// or all new ones, never a mixture"). owner must own id, and (if the
// parent is changing) the new parent too — same reasoning as Create.
func (r *PostgresRepo) Reparent(ctx context.Context, owner uuid.UUID, id PageID, parent ParentChange, after *PageID) (Page, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("pages: reparent: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	current, err := r.get(ctx, q, owner, id)
	if err != nil {
		return Page{}, fmt.Errorf("pages: reparent: resolving page: %w", err)
	}

	targetParentID := current.ParentID
	if parent.Change {
		targetParentID = parent.ParentID
	}

	var targetParentPath string
	if targetParentID != nil {
		targetParent, err := r.get(ctx, q, owner, *targetParentID)
		if err != nil {
			return Page{}, fmt.Errorf("pages: reparent: resolving new parent: %w", err)
		}
		if targetParent.Path == current.Path || strings.HasPrefix(targetParent.Path, current.Path+".") {
			return Page{}, ErrCycle
		}
		targetParentPath = targetParent.Path
	}
	newPath := childPath(targetParentPath, id)

	sortKey, err := r.nextSortKey(ctx, q, owner, targetParentID, after, &id)
	if err != nil {
		return Page{}, err
	}

	row, err := q.ReparentPageRow(ctx, pagerepo.ReparentPageRowParams{
		ID:        toPgUUID(uuid.UUID(id)),
		Column2:   newPath,
		SortKey:   sortKey,
		CreatedBy: toPgUUID(owner),
		ParentID:  toPgUUIDPtr(targetParentID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("pages: reparent: update: %w", err)
	}

	if newPath != current.Path {
		if _, err := q.RewriteDescendantPaths(ctx, pagerepo.RewriteDescendantPathsParams{
			NewPrefix: newPath,
			OldPrefix: current.Path,
			PageID:    toPgUUID(uuid.UUID(id)),
		}); err != nil {
			return Page{}, fmt.Errorf("pages: reparent: rewrite descendants: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Page{}, fmt.Errorf("pages: reparent: commit: %w", err)
	}
	return pageFromReparentRow(row), nil
}

func (r *PostgresRepo) Delete(ctx context.Context, owner uuid.UUID, id PageID) error {
	_, err := r.q.SoftDeletePage(ctx, pagerepo.SoftDeletePageParams{
		ID:        toPgUUID(uuid.UUID(id)),
		CreatedBy: toPgUUID(owner),
	})
	if err != nil {
		return fmt.Errorf("pages: delete: %w", err)
	}
	// A rows-affected count of 0 is still success: deleting an
	// already-deleted (or nonexistent, or not-yours) page is idempotent
	// (docs/api/pages.md § Delete) — it doesn't distinguish those cases
	// either, same reasoning as Get's NOT_FOUND.
	return nil
}

// pathLabel is the LTREE label for a page: LTREE labels may only contain
// letters, digits, and underscores, so a UUID's hyphens don't survive —
// "p" + the hex digits, prefixed to guarantee a non-digit start.
func pathLabel(id PageID) string {
	return "p" + strings.ReplaceAll(uuid.UUID(id).String(), "-", "")
}

// childPath is parentPath's LTREE ancestry with id's own label appended
// (or just the label, for a root page when parentPath is "").
func childPath(parentPath string, id PageID) string {
	if parentPath == "" {
		return pathLabel(id)
	}
	return parentPath + "." + pathLabel(id)
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
