-- name: ListPagesForDiscover :many
-- Every live page as the semantic index sees it: identity, classification,
-- and its whole prose body as one string.
--
-- One query, not one per page. The index is rebuilt per request at this
-- repo's scale (see internal/discover's own doc comment for why that is the
-- honest choice today and what would replace it), so the read has to be a
-- single scan or the rebuild cost becomes the feature's cost.
--
-- The body concatenates blocks in position order. Order does not matter to a
-- bag-of-words vector, but it matters to the excerpt the UI shows beside each
-- result, and producing two different concatenations for those two purposes
-- is how the excerpt drifts from the thing that was actually indexed.
--
-- COALESCE, not a filter: a page with no blocks yet is still a page, and it
-- must appear in the corpus as a zero vector rather than vanish from
-- everyone's neighbour list without explanation.
SELECT p.id,
       p.title,
       p.topic_id,
       t.name      AS topic_name,
       t.color_key AS topic_color_key,
       COALESCE(
         (SELECT array_agg(pt.tag ORDER BY pt.tag)
          FROM docs.page_tags pt WHERE pt.page_id = p.id),
         '{}'
       )::text[] AS tags,
       COALESCE(
         (SELECT string_agg(b.content->>'text', ' ' ORDER BY b.position)
          FROM docs.blocks b WHERE b.page_id = p.id),
         ''
       )::text AS body
FROM docs.pages p
LEFT JOIN docs.topics t ON t.id = p.topic_id
WHERE p.deleted_at IS NULL;
