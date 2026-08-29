-- name: ListPagesForGraph :many
-- Every live page — the link graph's node set (graphalgo.Graph
-- needs every page to exist as a node even with zero links, so orphan
-- detection can see it sitting alone). parent_id IS NULL identifies a
-- root for graphalgo.Orphans' own root set — a page nobody has nested
-- under anything else, so it's reachable without already knowing another
-- page's title.
-- topic_id joins the DECLARED partition onto the graph, so modularity
-- can be scored against it (graphalgo.Modularity).
-- The topic NAME and colour key travel with the node, not just the id: the
-- id is what modularity scores a partition by, and the name is what a legend
-- prints. A client joining the name in for itself is what caused the bug this
-- join fixes — ListPages returns one parent's children, so the join silently
-- covered only the root pages.
SELECT p.id, p.title, p.parent_id, p.topic_id,
       t.name      AS topic_name,
       t.color_key AS topic_color_key,
       COALESCE(
         (SELECT array_agg(pt.tag ORDER BY pt.tag)
          FROM docs.page_tags pt WHERE pt.page_id = p.id),
         '{}'
       )::text[] AS tags
FROM docs.pages p
LEFT JOIN docs.topics t ON t.id = p.topic_id
WHERE p.deleted_at IS NULL;

-- name: ListResolvedLinksForGraph :many
-- Every page_links row that resolved to a real page — the link graph's
-- edge set. A dangling link (target_page IS NULL) has nothing on the
-- other end to draw a line to or walk toward, so it's excluded here the
-- same way graphalgo.Edge's own doc comment says it must be.
SELECT from_page, target_page
FROM docs.page_links
WHERE target_page IS NOT NULL;
