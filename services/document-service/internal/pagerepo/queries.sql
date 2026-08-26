-- name: CreatePage :one
-- id is generated application-side (uuid v7, internal/pages), not by a
-- database default: the row's own path needs the id as its final LTREE
-- label, so it has to be known before the INSERT, not after.
INSERT INTO docs.pages (id, created_by, title, parent_id, path, sort_key)
VALUES ($1, $2, $3, sqlc.narg(parent_id), $4::ltree, $5)
RETURNING id, created_by, title, parent_id, path::text AS path, sort_key,
          lifecycle_state, deleted_at, created_at, updated_at;

-- name: GetPage :one
-- owner_id scopes every read to its caller (docs/api/pages.md § Get: "only
-- I can read or change my pages" — user story A-04). A page that exists
-- but belongs to someone else returns zero rows here, which the caller
-- maps to the identical NOT_FOUND a truly nonexistent page would — the
-- WHERE clause is what makes "doesn't reveal whether the page exists"
-- true, not an application-level check after the fact.
SELECT id, created_by, title, parent_id, path::text AS path, sort_key,
       lifecycle_state, deleted_at, created_at, updated_at
FROM docs.pages
WHERE id = $1 AND created_by = $2 AND deleted_at IS NULL;

-- name: NextSiblingSortKey :one
-- The sibling immediately after afterSortKey under the same parent (NULL
-- parent_id means root pages) — used to bound a new page's fractional
-- sort_key from above when it's inserted after a specific sibling.
-- exclude_id skips the page being moved itself, for ReparentPage's
-- reorder-within-the-same-parent case; Create passes it NULL.
SELECT sort_key FROM docs.pages
WHERE deleted_at IS NULL
  AND (parent_id = sqlc.narg(parent_id) OR (parent_id IS NULL AND sqlc.narg(parent_id) IS NULL))
  AND sort_key > @after_sort_key::text
  AND (sqlc.narg(exclude_id)::uuid IS NULL OR id != sqlc.narg(exclude_id))
ORDER BY sort_key ASC
LIMIT 1;

-- name: LastSiblingSortKey :one
-- The highest sort_key among a parent's children (or root pages) — used
-- as the lower bound when appending (CreatePageRequest.after omitted).
-- exclude_id: see NextSiblingSortKey.
SELECT sort_key FROM docs.pages
WHERE deleted_at IS NULL
  AND (parent_id = sqlc.narg(parent_id) OR (parent_id IS NULL AND sqlc.narg(parent_id) IS NULL))
  AND (sqlc.narg(exclude_id)::uuid IS NULL OR id != sqlc.narg(exclude_id))
ORDER BY sort_key DESC
LIMIT 1;

-- name: ListPages :many
-- owner_id: see GetPage — a list is scoped to the caller's own pages,
-- never a cross-user listing.
SELECT id, created_by, title, parent_id, path::text AS path, sort_key,
       lifecycle_state, deleted_at, created_at, updated_at
FROM docs.pages
WHERE created_by = $2 AND deleted_at IS NULL
  AND (parent_id = sqlc.narg(parent_id) OR (parent_id IS NULL AND sqlc.narg(parent_id) IS NULL))
  AND (sqlc.narg(after)::text IS NULL OR sort_key > sqlc.narg(after))
ORDER BY sort_key ASC
LIMIT $1;

-- name: RenamePage :one
-- owner_id: see GetPage.
UPDATE docs.pages SET title = $2, updated_at = NOW()
WHERE id = $1 AND created_by = $3 AND deleted_at IS NULL
RETURNING id, created_by, title, parent_id, path::text AS path, sort_key,
          lifecycle_state, deleted_at, created_at, updated_at;

-- name: ReparentPageRow :one
-- Moves a single page: new parent (nullable — NULL promotes it to root),
-- new path, new sort_key. Descendants' paths are a separate statement
-- (RewriteDescendantPaths) — both run in the same transaction
-- (internal/pages's Reparent), so a concurrent reader sees all old paths
-- or all new ones, never a mixture (docs/api/pages.md § Reparent).
-- owner_id: see GetPage.
UPDATE docs.pages
SET parent_id = sqlc.narg(parent_id), path = $2::ltree, sort_key = $3, updated_at = NOW()
WHERE id = $1 AND created_by = $4 AND deleted_at IS NULL
RETURNING id, created_by, title, parent_id, path::text AS path, sort_key,
          lifecycle_state, deleted_at, created_at, updated_at;

-- name: RewriteDescendantPaths :execrows
-- Replaces the old_prefix ancestry on every descendant of the page being
-- moved with new_prefix, preserving each descendant's own trailing labels.
-- subpath(path, nlevel(old_prefix)) strips exactly the old ancestor
-- portion off the front of each descendant's path.
UPDATE docs.pages
SET path = @new_prefix::ltree || subpath(path, nlevel(@old_prefix::ltree)),
    updated_at = NOW()
WHERE path <@ @old_prefix::ltree AND id != @page_id;

-- name: SoftDeletePage :execrows
-- Simple soft delete only — no cascade to descendants and no saga
-- completion step (ARCHITECTURE.md §5's delete saga is out of scope for
-- this first PageService slice; see docs/porting/PROGRESS.md).
-- owner_id: see GetPage.
UPDATE docs.pages SET lifecycle_state = 'deleting', deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND created_by = $2 AND deleted_at IS NULL;
