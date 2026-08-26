-- name: CreatePage :one
-- id is generated application-side (uuid v7, internal/pages), not by a
-- database default: the row's own path needs the id as its final LTREE
-- label, so it has to be known before the INSERT, not after.
INSERT INTO docs.pages (id, created_by, title, parent_id, path, sort_key)
VALUES ($1, $2, $3, sqlc.narg(parent_id), $4::ltree, $5)
RETURNING id, created_by, title, parent_id, path::text AS path, sort_key,
          lifecycle_state, deleted_at, created_at, updated_at;

-- name: GetPage :one
SELECT id, created_by, title, parent_id, path::text AS path, sort_key,
       lifecycle_state, deleted_at, created_at, updated_at
FROM docs.pages
WHERE id = $1 AND deleted_at IS NULL;

-- name: NextSiblingSortKey :one
-- The sibling immediately after afterSortKey under the same parent (NULL
-- parent_id means root pages) — used to bound a new page's fractional
-- sort_key from above when it's inserted after a specific sibling.
SELECT sort_key FROM docs.pages
WHERE deleted_at IS NULL
  AND (parent_id = sqlc.narg(parent_id) OR (parent_id IS NULL AND sqlc.narg(parent_id) IS NULL))
  AND sort_key > @after_sort_key::text
ORDER BY sort_key ASC
LIMIT 1;

-- name: LastSiblingSortKey :one
-- The highest sort_key among a parent's children (or root pages) — used
-- as the lower bound when appending (CreatePageRequest.after omitted).
SELECT sort_key FROM docs.pages
WHERE deleted_at IS NULL
  AND (parent_id = sqlc.narg(parent_id) OR (parent_id IS NULL AND sqlc.narg(parent_id) IS NULL))
ORDER BY sort_key DESC
LIMIT 1;

-- name: ListPages :many
SELECT id, created_by, title, parent_id, path::text AS path, sort_key,
       lifecycle_state, deleted_at, created_at, updated_at
FROM docs.pages
WHERE deleted_at IS NULL
  AND (parent_id = sqlc.narg(parent_id) OR (parent_id IS NULL AND sqlc.narg(parent_id) IS NULL))
  AND (sqlc.narg(after)::text IS NULL OR sort_key > sqlc.narg(after))
ORDER BY sort_key ASC
LIMIT $1;

-- name: RenamePage :one
UPDATE docs.pages SET title = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, created_by, title, parent_id, path::text AS path, sort_key,
          lifecycle_state, deleted_at, created_at, updated_at;

-- name: SoftDeletePage :execrows
-- Simple soft delete only — no cascade to descendants and no saga
-- completion step (ARCHITECTURE.md §5's delete saga is out of scope for
-- this first PageService slice; see docs/porting/PROGRESS.md).
UPDATE docs.pages SET lifecycle_state = 'deleting', deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
