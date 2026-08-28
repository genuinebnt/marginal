-- name: CreatePage :one
-- id is generated application-side (uuid v7, internal/pages), not by a
-- database default: the row's own path needs the id as its final LTREE
-- label, so it has to be known before the INSERT, not after.
INSERT INTO docs.pages (id, created_by, title, parent_id, path, sort_key)
VALUES (@id, @created_by, @title, sqlc.narg(parent_id), @path::ltree, @sort_key)
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
SET parent_id = sqlc.narg(parent_id), path = @path::ltree, sort_key = @sort_key, updated_at = NOW()
WHERE id = @id AND deleted_at IS NULL
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

-- name: StartPageDeletion :exec
-- Opens the saga row alongside the soft-delete, in the same transaction
-- (internal/pagesaga). ON CONFLICT DO NOTHING makes a repeated DeletePage
-- idempotent: the second call must not reset steps_done or attempts on a
-- saga already in flight.
INSERT INTO docs.page_deletions (page_id, requested_by)
VALUES (@page_id, @requested_by)
ON CONFLICT (page_id) DO NOTHING;

-- name: ClaimPageDeletions :many
-- The sweeper's claim. FOR UPDATE SKIP LOCKED so two document-service
-- instances sweep the same table without either blocking or double-running
-- a step — the same pattern marginal/outboxpoll uses for outbox rows.
-- Oldest first, so a saga that keeps failing cannot starve behind newer ones.
SELECT page_id, requested_by, steps_done, attempts, started_at
FROM docs.page_deletions
WHERE completed_at IS NULL
ORDER BY started_at
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: RecordPageDeletionStep :exec
-- Appends one completed step. array_append is guarded by NOT (... = ANY ...)
-- so re-running a step that already recorded itself is a no-op rather than a
-- duplicate entry — steps_done is a set that happens to keep its order.
UPDATE docs.page_deletions
SET steps_done = CASE WHEN @step::text = ANY(steps_done)
                      THEN steps_done
                      ELSE array_append(steps_done, @step::text) END,
    last_error = NULL,
    updated_at = NOW()
WHERE page_id = @page_id;

-- name: RecordPageDeletionFailure :exec
-- Forward-only compensation: a failing step is recorded and retried, never
-- rolled back. attempts is bumped here rather than at claim time so it
-- counts real resumptions, not sweeps that found nothing to do.
UPDATE docs.page_deletions
SET last_error = @last_error, attempts = attempts + 1, updated_at = NOW()
WHERE page_id = @page_id;

-- name: CompletePageDeletion :exec
-- The saga's terminal write: the page leaves 'deleting' for 'deleted' and
-- becomes restorable. Both rows move together so a reader can never see a
-- finished saga over a still-'deleting' page.
WITH done AS (
    UPDATE docs.page_deletions SET completed_at = NOW(), last_error = NULL, updated_at = NOW()
    WHERE page_id = @page_id AND completed_at IS NULL
    RETURNING page_id
)
UPDATE docs.pages SET lifecycle_state = 'deleted', updated_at = NOW()
WHERE id = (SELECT page_id FROM done);

-- name: DeleteSubtreeLinks :execrows
-- StepLinksRewritten. Drops only rows where a page in the deleted subtree
-- was the SOURCE — claims made by a page that no longer exists. Rows
-- pointing AT the subtree are left alone on purpose: a [[link]] to a
-- deleted page is a real dangling reference RFC-003's DanglingLink
-- analyzer is meant to report, not a row to tidy away.
DELETE FROM docs.page_links
WHERE source_page_id IN (
    SELECT id FROM docs.pages WHERE path <@ @parent_path::ltree
);

-- name: ListTrash :many
-- Both terminal-ish states, newest first: 'deleting' (saga in flight) and
-- 'deleted' (restorable until purge). purge_at is derived from deleted_at
-- at read time rather than stored, so changing the window moves every
-- pending purge without a backfill (DATA_MODEL.md § Page deletions).
SELECT p.id, p.created_by, p.title, p.parent_id, p.path::text AS path, p.sort_key,
       p.lifecycle_state, p.deleted_at, p.created_at, p.updated_at,
       p.deleted_at + @purge_window::interval AS purge_at,
       d.steps_done, d.attempts, d.last_error, d.completed_at
FROM docs.pages p
LEFT JOIN docs.page_deletions d ON d.page_id = p.id
WHERE p.deleted_at IS NOT NULL
ORDER BY p.deleted_at DESC
LIMIT $1 OFFSET $2;

-- name: CountTrash :one
SELECT COUNT(*) FROM docs.pages WHERE deleted_at IS NOT NULL;

-- name: RestorePageAndSubtree :execrows
-- Restore is the inverse of StepTreeDetached and nothing else — the later
-- steps are not undone, they are re-derived: page_links and the FTS index
-- are projections that rebuild from the op log, and the op log was sealed,
-- never deleted. Scoped to rows deleted in the SAME saga (deleted_at equal
-- to the target's) so restoring a page does not resurrect a child that was
-- already in the trash on its own before this delete ran.
UPDATE docs.pages p
SET lifecycle_state = 'active', deleted_at = NULL, updated_at = NOW()
FROM docs.pages target
WHERE target.id = @page_id
  AND target.lifecycle_state = 'deleted'
  AND (p.id = target.id OR (p.path <@ target.path AND p.deleted_at = target.deleted_at));

-- name: ClearPageDeletion :exec
-- Restore drops the saga row rather than marking it. A restored page that
-- is deleted again is a NEW saga, not a resumed one — keeping the old row
-- would make attempts and steps_done describe two different operations.
DELETE FROM docs.page_deletions WHERE page_id = @page_id;

-- name: ListTopics :many
-- With the page count each carries, since every screen that lists topics
-- also shows how much is in them (ui-mockups § 10b). Excludes deleted pages
-- from the count — a topic is not "still busy" because its pages are in the
-- trash.
SELECT t.id, t.name, t.color_key, t.created_at,
       COUNT(p.id) FILTER (WHERE p.deleted_at IS NULL) AS page_count
FROM docs.topics t
LEFT JOIN docs.pages p ON p.topic_id = t.id
GROUP BY t.id, t.name, t.color_key, t.created_at
ORDER BY t.name;

-- name: SetPageTopic :execrows
-- NULL clears it back to untopiced, which is a real state rather than a
-- failure — assignment is the user's call (ui-mockups § 10b: "suggestion
-- available, assignment is yours").
UPDATE docs.pages SET topic_id = @topic_id, updated_at = NOW()
WHERE id = @page_id AND deleted_at IS NULL;

-- name: CountUntopicedPages :one
SELECT COUNT(*) FROM docs.pages WHERE topic_id IS NULL AND deleted_at IS NULL;

-- name: ListPageTags :many
SELECT tag FROM docs.page_tags WHERE page_id = $1 ORDER BY tag;

-- name: AddPageTag :exec
-- Idempotent: re-tagging is a no-op, not a constraint violation. The CHECK
-- on the column already enforces lowercase, so this lowers here rather than
-- letting a mixed-case write fail at the database — the caller typed a tag,
-- they did not make a mistake.
INSERT INTO docs.page_tags (page_id, tag) VALUES (@page_id, lower(@tag::text))
ON CONFLICT DO NOTHING;

-- name: RemovePageTag :execrows
DELETE FROM docs.page_tags WHERE page_id = @page_id AND tag = lower(@tag::text);

-- name: ListTagFacets :many
-- Search's facet rail: every tag with how many live pages carry it, most
-- used first. The tag-with-a-topic-count join is what makes ui-mockups
-- § 10b's "a tag that lives in three topics is doing real work" real —
-- topics_spanned is the distinct topic count across the tag's pages.
SELECT pt.tag,
       COUNT(*) AS page_count,
       COUNT(DISTINCT p.topic_id) AS topics_spanned
FROM docs.page_tags pt
JOIN docs.pages p ON p.id = pt.page_id AND p.deleted_at IS NULL
GROUP BY pt.tag
ORDER BY COUNT(*) DESC, pt.tag
LIMIT $1;

-- name: ListPagesByTopic :many
SELECT id, created_by, title, parent_id, path::text AS path, sort_key,
       lifecycle_state, deleted_at, created_at, updated_at, topic_id
FROM docs.pages
WHERE topic_id = $1 AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- name: TopicsForPages :many
-- Batch lookup for ListPages. One query for the whole page, not one per row
-- — a tree render asks for 50 pages at a time, and N+1 there is 50 round
-- trips to decorate a list that already cost one.
SELECT p.id AS page_id, t.id, t.name, t.color_key
FROM docs.pages p JOIN docs.topics t ON t.id = p.topic_id
WHERE p.id = ANY(@page_ids::uuid[]);

-- name: TagsForPages :many
SELECT page_id, tag FROM docs.page_tags
WHERE page_id = ANY(@page_ids::uuid[])
ORDER BY page_id, tag;
