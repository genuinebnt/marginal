-- name: SearchPageTitles :many
-- A title-only hit — websearch_to_tsquery understands quoted phrases
-- and bare-word OR/AND the way a user actually types a query box, unlike
-- plainto_tsquery (AND-only) or the raw to_tsquery operator syntax
-- (search.html is a plain <input>, not a query language). Ranked by
-- ts_rank against docs.pages' own GENERATED ALWAYS tsvector column
-- (migration 00004) — transactionally consistent with title, never
-- stale the way the BK-tree title index deliberately can be.
SELECT id, title, ts_rank(search_vector, websearch_to_tsquery('english', sqlc.arg(query)::text)) AS rank
FROM docs.pages
-- Scoped to the caller's spaces (v3.3.0). It was not: a full-text search
-- returned hits from every space on the instance, which is a worse leak
-- than the graph's titles because a snippet is the CONTENT.
WHERE deleted_at IS NULL
  AND space_id = ANY(@space_ids::uuid[])
  AND search_vector @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
ORDER BY rank DESC
LIMIT 20;

-- name: SearchBlockText :many
-- A block-text hit, joined back to its page for the title a result card
-- needs. ts_headline builds the <b>...</b>-wrapped snippet directly —
-- search.html's own "the snippet shows why it matched," computed once
-- here rather than re-deriving match spans client-side.
SELECT
    b.page_id AS page_id,
    p.title AS page_title,
    b.id AS block_id,
    ts_headline('english', coalesce(b.content->>'text', ''), websearch_to_tsquery('english', sqlc.arg(query)::text)) AS snippet,
    ts_rank(b.search_vector, websearch_to_tsquery('english', sqlc.arg(query)::text)) AS rank
FROM docs.blocks b
JOIN docs.pages p ON p.id = b.page_id
WHERE p.deleted_at IS NULL
  AND p.space_id = ANY(@space_ids::uuid[])
  AND b.search_vector @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
ORDER BY rank DESC
LIMIT 20;

-- name: ListPageTitlesForIndex :many
-- Every live page's id/title — internal/search's own BK-tree rebuild
-- reads this on its refresh cadence (NOT on every request: the whole
-- point of a rebuilt-periodically index, search.html's own admitted
-- "may lag the write path").
-- NOT space-scoped, and that is correct: this feeds the BK-tree index,
-- which is rebuilt once per cadence rather than per caller. The scoping
-- happens where the index is QUERIED, against the asker's own spaces —
-- see internal/search. An index built per space would be one index per
-- member of the instance.
SELECT id, title, space_id
FROM docs.pages
WHERE deleted_at IS NULL;
