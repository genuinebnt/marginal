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

	"marginal/document-service/internal/blockrepo/gen"
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

// PostgresRepo is document-service's only port onto docs.pages. Server
// (api.go) depends on this concrete type directly rather than a Repo
// interface — an earlier version declared one, but it had exactly one
// implementation and no test double ever used it; CLOUD_PORTABILITY.md's
// "small interface at the point of use" convention means declaring one
// when something actually needs to vary, not preemptively (see
// api-gateway's internal/pagesrest.Handler for the same concrete-type,
// unexported-field-plus-constructor shape this now matches).
//
// No method scopes reads/writes by owner (reversed 2026-08-26, at
// explicit user request — docs/porting/PROGRESS.md): every page on this
// instance is visible and editable by every authenticated actor, the same
// "shared workspace, not multi-tenant" model Register's own reversal
// already established. created_by is still recorded on Create (who made
// a page), just no longer an access filter — api.go's actorID(ctx) still
// enforces that the caller is authenticated at all, just not that they
// own the specific page.
type PostgresRepo struct {
	q    *pagerepo.Queries
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{q: pagerepo.New(pool), pool: pool}
}

// Create records np.CreatedBy as the page's author, but — since no page
// is owner-scoped anymore (see Repo's own doc comment) — a parent or
// "after" anchor naming *any* page on the instance is valid, not just one
// the caller made themselves.
func (r *PostgresRepo) Create(ctx context.Context, np NewPage) (Page, error) {
	id := PageID(uuid.Must(uuid.NewV7()))

	var parentPath string
	if np.ParentID != nil {
		parent, err := r.get(ctx, r.q, *np.ParentID)
		if err != nil {
			return Page{}, fmt.Errorf("pages: resolving parent: %w", err)
		}
		parentPath = parent.Path
		// A child inherits its parent's space, and this OVERRIDES anything
		// the caller asked for. Letting a child sit in a different space
		// from its parent would make permissions change partway down a
		// tree, and "who can read this" would stop being one lookup and
		// become a walk — which ADR-013 §2 rules out explicitly.
		np.SpaceID = parent.SpaceID
	}
	path := childPath(parentPath, id)

	sortKey, err := r.nextSortKey(ctx, r.q, np.ParentID, np.After, nil)
	if err != nil {
		return Page{}, err
	}

	row, err := r.q.CreatePage(ctx, pagerepo.CreatePageParams{
		ID:        toPgUUID(uuid.UUID(id)),
		CreatedBy: toPgUUID(np.CreatedBy),
		Title:     np.Title,
		Path:      path,
		SortKey:   sortKey,
		ParentID:  toPgUUIDPtr(np.ParentID),
		SpaceID:   toPgUUID(spaceFor(np)),
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
// moved out of its own neighbor search.
func (r *PostgresRepo) nextSortKey(ctx context.Context, q *pagerepo.Queries, parentID, after, excludeID *PageID) (string, error) {
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

	anchor, err := r.get(ctx, q, *after)
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

func (r *PostgresRepo) Get(ctx context.Context, id PageID) (Page, error) {
	return r.get(ctx, r.q, id)
}

func (r *PostgresRepo) get(ctx context.Context, q *pagerepo.Queries, id PageID) (Page, error) {
	row, err := q.GetPage(ctx, toPgUUID(uuid.UUID(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("pages: get: %w", err)
	}
	return pageFromGetRow(row), nil
}

// List returns pages inside spaceIDs only (ADR-013 §4).
//
// spaceIDs is a required argument rather than something List looks up,
// because the caller is the only one who knows WHO is asking — and a
// signature that could be called without it is one that will be. Passing
// an empty slice returns nothing, which is the right answer for a user in
// no space: a filter that matches everything when it is empty is a
// permission check that disappears exactly when it is needed.
func (r *PostgresRepo) List(ctx context.Context, spaceIDs []uuid.UUID, parentID *PageID, after string, limit int32) ([]Page, error) {
	var afterPtr *string
	if after != "" {
		afterPtr = &after
	}
	rows, err := r.q.ListPages(ctx, pagerepo.ListPagesParams{
		Limit:    limit,
		SpaceIds: toPgUUIDs(spaceIDs),
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
	row, err := r.q.RenamePage(ctx, pagerepo.RenamePageParams{
		ID:    toPgUUID(uuid.UUID(id)),
		Title: title,
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
// or all new ones, never a mixture").
func (r *PostgresRepo) Reparent(ctx context.Context, id PageID, parent ParentChange, after *PageID) (Page, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("pages: reparent: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	current, err := r.get(ctx, q, id)
	if err != nil {
		return Page{}, fmt.Errorf("pages: reparent: resolving page: %w", err)
	}

	targetParentID := current.ParentID
	if parent.Change {
		targetParentID = parent.ParentID
	}

	var targetParentPath string
	if targetParentID != nil {
		targetParent, err := r.get(ctx, q, *targetParentID)
		if err != nil {
			return Page{}, fmt.Errorf("pages: reparent: resolving new parent: %w", err)
		}
		if targetParent.Path == current.Path || strings.HasPrefix(targetParent.Path, current.Path+".") {
			return Page{}, ErrCycle
		}
		targetParentPath = targetParent.Path
	}
	newPath := childPath(targetParentPath, id)

	sortKey, err := r.nextSortKey(ctx, q, targetParentID, after, &id)
	if err != nil {
		return Page{}, err
	}

	row, err := q.ReparentPageRow(ctx, pagerepo.ReparentPageRowParams{
		ID:       toPgUUID(uuid.UUID(id)),
		Path:     newPath,
		SortKey:  sortKey,
		ParentID: toPgUUIDPtr(targetParentID),
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

// Delete soft-deletes id and cascades to every descendant (found via the
// same path <@ pattern Reparent's descendant rewrite uses), all in one
// transaction. A page that doesn't exist, isn't the caller's, or is
// already deleted: zero rows affected, no error — idempotent
// (docs/api/pages.md § Delete). No hard-delete-after-acks step —
// ARCHITECTURE.md §5's full saga needs services outside this repo's
// scope; lifecycle_state stays 'deleting' forever here.
func (r *PostgresRepo) Delete(ctx context.Context, id PageID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pages: delete: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	page, err := r.get(ctx, q, id)
	if errors.Is(err, ErrNotFound) {
		// Nothing to delete or cascade — idempotent no-op, not an error.
		return nil
	}
	if err != nil {
		return fmt.Errorf("pages: delete: resolving page: %w", err)
	}

	if _, err := q.SoftDeleteDescendants(ctx, pagerepo.SoftDeleteDescendantsParams{
		ParentPath: page.Path,
		PageID:     toPgUUID(uuid.UUID(id)),
	}); err != nil {
		return fmt.Errorf("pages: delete: cascading to descendants: %w", err)
	}

	if _, err := q.SoftDeletePage(ctx, toPgUUID(uuid.UUID(id))); err != nil {
		return fmt.Errorf("pages: delete: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pages: delete: commit: %w", err)
	}
	return nil
}

// ListBacklinks reads docs.page_links directly via the pool, not r.q —
// that table belongs to internal/blockproj's projection, a different
// sqlc package (internal/blockrepo) from this file's own pagerepo. See
// the Repo interface's own doc comment on why this method doesn't check
// ownership itself.
func (r *PostgresRepo) ListBacklinks(ctx context.Context, id PageID) ([]Backlink, error) {
	bq := blockrepo.New(r.pool)
	rows, err := bq.ListBacklinksForPage(ctx, toPgUUID(uuid.UUID(id)))
	if err != nil {
		return nil, fmt.Errorf("pages: list backlinks: %w", err)
	}
	out := make([]Backlink, len(rows))
	for i, row := range rows {
		out[i] = Backlink{
			FromPage:        PageID(fromPgUUID(row.FromPage)),
			FromPageTitle:   row.FromPageTitle,
			FromPageDeleted: row.FromPageDeletedAt.Valid,
			TargetTitle:     row.TargetTitle,
		}
	}
	return out, nil
}

// ListBlocks reads docs.blocks directly via the pool, same reasoning as
// ListBacklinks above: that table also belongs to internal/blockproj's
// projection, not this file's own pagerepo.
func (r *PostgresRepo) ListBlocks(ctx context.Context, id PageID) ([]Block, error) {
	bq := blockrepo.New(r.pool)
	rows, err := bq.ListBlocksForPage(ctx, toPgUUID(uuid.UUID(id)))
	if err != nil {
		return nil, fmt.Errorf("pages: list blocks: %w", err)
	}
	out := make([]Block, len(rows))
	for i, row := range rows {
		var parentID *BlockID
		if row.ParentID.Valid {
			id := BlockID(fromPgUUID(row.ParentID))
			parentID = &id
		}
		out[i] = Block{
			ID:          BlockID(fromPgUUID(row.ID)),
			ParentID:    parentID,
			KindJSON:    row.Kind,
			ContentJSON: row.Content,
		}
	}
	return out, nil
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

// DefaultSpaceID is the space every pre-v3.1.0 page was migrated into, and
// where a page goes when nothing says otherwise.
//
// The same constant appears in both services' migrations, which
// DATA_MODEL.md already records as a deliberate coordination point: two
// services must agree which space existing pages belong to and cannot join
// across schemas to find out.
var DefaultSpaceID = uuid.MustParse("00000000-0000-7000-8000-00000000d0c5")

// spaceFor decides where a new page lands, once Create has already applied
// parent inheritance.
//
// A root page with no space named goes to the default. That is v3.1.0's
// migration promise held for new pages too: nothing changes about where
// things land until a space switcher exists to say otherwise (§ 23).
func spaceFor(np NewPage) uuid.UUID {
	if np.SpaceID != uuid.Nil {
		return np.SpaceID
	}
	return DefaultSpaceID
}

// toPgUUIDs converts a caller's space set for the ANY(...) filter.
func toPgUUIDs(ids []uuid.UUID) []pgtype.UUID {
	// Non-nil even when empty: a nil array and an empty one both match
	// nothing here, but returning nil invites a "== nil means unfiltered"
	// shortcut somewhere downstream, and that shortcut is the whole bug
	// this filter exists to prevent.
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		out = append(out, toPgUUID(id))
	}
	return out
}
