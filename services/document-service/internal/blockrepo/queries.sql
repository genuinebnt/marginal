-- name: ReplaceBlocksForPage :exec
-- internal/blockproj rewrites a page's whole projection on every event
-- (its own doc comment explains why this is the right amount of
-- simplicity for a rebuildable read model) — delete then bulk-insert via
-- pgx.Batch (InsertBlock, :batchexec below), one round trip for the
-- whole page's blocks, both in the same transaction as the page_links
-- rewrite below.
DELETE FROM docs.blocks WHERE page_id = $1;

-- name: InsertBlock :exec
INSERT INTO docs.blocks (id, page_id, position, kind, content)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertBlockBatch :batchexec
-- Same statement as InsertBlock, queued via pgx.Batch — one pipelined
-- round trip for however many blocks a page has, same reasoning as
-- collaboration-service's own InsertOpBatch.
INSERT INTO docs.blocks (id, page_id, position, kind, content)
VALUES ($1, $2, $3, $4, $5);

-- name: ReplacePageLinksForPage :exec
DELETE FROM docs.page_links WHERE from_page = $1;

-- name: InsertPageLink :exec
-- target_page resolves by case-insensitive title match against this
-- service's own docs.pages — a link is dangling (NULL) until a page with
-- that title exists (RFC-003 §6). Duplicate titles are NOT diagnosed
-- here (docs.pages' own migration: uniqueness isn't enforced, matching
-- RFC-003's DuplicateTitle being a separate check) — an arbitrary but
-- deterministic match (lowest id) is picked rather than left ambiguous.
-- target_title is bound twice (sqlc can't alias one placeholder to two
-- struct fields) — the caller passes the identical string both times.
INSERT INTO docs.page_links (id, from_page, from_block, target_title, target_page)
VALUES (
    $1, $2, $3, sqlc.arg(target_title),
    (SELECT p.id FROM docs.pages p
     WHERE lower(p.title) = lower(sqlc.arg(target_title_for_lookup)) AND p.deleted_at IS NULL
     ORDER BY p.id LIMIT 1)
);

-- name: InsertPageLinkBatch :batchexec
-- Same statement as InsertPageLink, batched for the same reason as InsertBlockBatch.
INSERT INTO docs.page_links (id, from_page, from_block, target_title, target_page)
VALUES (
    $1, $2, $3, sqlc.arg(target_title),
    (SELECT p.id FROM docs.pages p
     WHERE lower(p.title) = lower(sqlc.arg(target_title_for_lookup)) AND p.deleted_at IS NULL
     ORDER BY p.id LIMIT 1)
);

-- name: ListBlocksForPage :many
SELECT id, page_id, position, kind, content, updated_at
FROM docs.blocks
WHERE page_id = $1
ORDER BY position;

-- name: ListBacklinksForPage :many
-- Pages linking INTO $1 — the "Backlinks" inspector tab's query. Joined
-- to docs.pages for the linking page's own title/lifecycle, since a
-- backlink from a since-deleted page is still worth showing as such
-- (docs/ui-mockups/editor.html's own "Q2 planning · deleted" row).
SELECT pl.from_page, pl.from_block, pl.target_title, p.title AS from_page_title, p.deleted_at AS from_page_deleted_at
FROM docs.page_links pl
JOIN docs.pages p ON p.id = pl.from_page
WHERE pl.target_page = $1
ORDER BY p.title;
