-- name: CreatePage :one
-- id is generated application-side (uuid v7, internal/pages), not by a
-- database default: the row's own path needs the id as its final LTREE
-- label, so it has to be known before the INSERT, not after.
INSERT INTO docs.pages (id, created_by, title, parent_id, path, sort_key)
VALUES ($1, $2, $3, sqlc.narg(parent_id), $4::ltree, $5)
RETURNING id, created_by, title, parent_id, path::text AS path, sort_key,
          lifecycle_state, deleted_at, created_at, updated_at;

-- name: GetPage :one
-- No owner scoping — every page on this instance is visible to every
-- authenticated actor (docs/porting/PROGRESS.md's "shared workspace, not
-- multi-tenant" reversal, the same one Register's own registration model
-- already made: real multi-user collaboration needs a second person to
-- actually see the first person's pages, not just edit their live
-- content). created_by is still recorded — who made a page — just no
-- longer an access filter.
SELECT id, created_by, title, parent_id, path::text AS path, sort_key,
       lifecycle_state, deleted_at, created_at, updated_at
FROM docs.pages
WHERE id = $1 AND deleted_at IS NULL;

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
-- No owner scoping — see GetPage. Lists every page on the instance
-- matching parent_id, not just the caller's own.
SELECT id, created_by, title, parent_id, path::text AS path, sort_key,
       lifecycle_state, deleted_at, created_at, updated_at
FROM docs.pages
WHERE deleted_at IS NULL
  AND (parent_id = sqlc.narg(parent_id) OR (parent_id IS NULL AND sqlc.narg(parent_id) IS NULL))
  AND (sqlc.narg(after)::text IS NULL OR sort_key > sqlc.narg(after))
ORDER BY sort_key ASC
LIMIT $1;

-- name: RenamePage :one
-- No owner scoping — see GetPage.
UPDATE docs.pages SET title = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, created_by, title, parent_id, path::text AS path, sort_key,
          lifecycle_state, deleted_at, created_at, updated_at;

-- name: ReparentPageRow :one
-- Moves a single page: new parent (nullable — NULL promotes it to root),
-- new path, new sort_key. Descendants' paths are a separate statement
-- (RewriteDescendantPaths) — both run in the same transaction
-- (internal/pages's Reparent), so a concurrent reader sees all old paths
-- or all new ones, never a mixture (docs/api/pages.md § Reparent).
-- No owner scoping — see GetPage.
UPDATE docs.pages
SET parent_id = sqlc.narg(parent_id), path = $2::ltree, sort_key = $3, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
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
-- No owner scoping — see GetPage. No saga completion step (ARCHITECTURE.md
-- §5's hard-delete-after-acks needs services outside this repo's scope;
-- docs/api/pages.md § Delete) — lifecycle_state stays 'deleting' forever.
UPDATE docs.pages SET lifecycle_state = 'deleting', deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteDescendants :execrows
-- The cascade half of Delete: every descendant of the deleted page (found
-- via the same path <@ pattern RewriteDescendantPaths uses) is
-- soft-deleted too. Excludes already-deleted rows so a repeated call
-- (idempotent Delete) touches nothing on the second pass.
UPDATE docs.pages SET lifecycle_state = 'deleting', deleted_at = NOW(), updated_at = NOW()
WHERE path <@ @parent_path::ltree AND id != @page_id AND deleted_at IS NULL;
