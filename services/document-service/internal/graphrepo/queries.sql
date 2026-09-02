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
-- Scoped to the caller's spaces, like every other read in this service.
-- It was NOT, until v3.3.0: the whole graph — every page title on the
-- instance — was returned to anybody who asked, including titles in
-- spaces they are not a member of. A page title says what somebody is
-- working on, which is the same thing spaces.md says about space names.
WHERE p.deleted_at IS NULL AND p.space_id = ANY(@space_ids::uuid[]);

-- name: ListResolvedLinksForGraph :many
-- Every page_links row that resolved to a real page — the link graph's
-- edge set. A dangling link (target_page IS NULL) has nothing on the
-- other end to draw a line to or walk toward, so it's excluded here the
-- same way graphalgo.Edge's own doc comment says it must be.
-- Both ends must be visible. A link out of a page you cannot see is not
-- an edge you are allowed to know about, and one INTO a page you cannot
-- see would draw a line to a node that is not in your node set.
SELECT l.from_page, l.target_page
FROM docs.page_links l
JOIN docs.pages fp ON fp.id = l.from_page
JOIN docs.pages tp ON tp.id = l.target_page
WHERE l.target_page IS NOT NULL
  AND fp.space_id = ANY(@space_ids::uuid[])
  AND tp.space_id = ANY(@space_ids::uuid[]);

-- name: ListDanglingLinks :many
-- § 20's CHECKS row: a [[link]] to a page that does not exist yet.
-- target_page IS NULL is exactly that, and the column is indexed for it.
--
-- Derived on every read rather than stored as a notification, deliberately:
-- a stored check goes stale the moment somebody creates the page, and an
-- inbox row that has silently become false is worse than no row. Creating
-- the page makes this disappear, which is § 20's "acting on an item clears
-- it" without any clearing machinery.
SELECT l.target_title,
       l.from_page,
       p.title AS from_page_title,
       l.from_block
FROM docs.page_links l
JOIN docs.pages p ON p.id = l.from_page
WHERE l.target_page IS NULL
  AND p.deleted_at IS NULL
  AND p.space_id = ANY(@space_ids::uuid[])
ORDER BY l.target_title;
