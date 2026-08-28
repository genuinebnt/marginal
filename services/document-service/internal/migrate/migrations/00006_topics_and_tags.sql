-- +goose Up

-- v2.7.0's classification, and the reason it is TWO tables rather than one
-- (ui-mockups § 10b TOPICS & TAGS, docs/api/pages.md):
--
--   A TOPIC is singular, owned, and indexed — one per page, a real column.
--   It clusters the link graph, colours a node, and scopes similarity search.
--   A TAG is free-form and many — it facets search and never boosts rank,
--   never picks a hue.
--
-- Collapsing them into one field gives you folders: a page that is genuinely
-- about two things then has to lie about one of them, and every consumer has
-- to guess which of a page's labels was the load-bearing one. Splitting them
-- means the graph has exactly one colour to read per node and search has an
-- unbounded number of facets, which is what each actually needs.

CREATE TABLE docs.topics (
    id         UUID PRIMARY KEY,
    -- Renaming is free precisely because nothing references the name — the
    -- graph, search, and every page row join on id (ui-mockups § 10b's own
    -- status line: "topic by id · rename is free").
    name       TEXT NOT NULL UNIQUE,
    -- Which of the five categorical hues this topic draws as. A key, not a
    -- hex value: the palette belongs to the design system (v2's
    -- DESIGN_GUIDELINES.md §3.4), and storing #7AA8E8 here would fork it the
    -- first time the ramp is retuned.
    --
    -- Deliberately disjoint from the semantic four (amber/teal/violet/slate).
    -- A hue that means both "diagnostic" and "topic: operations" means
    -- neither.
    color_key  TEXT NOT NULL
        CHECK (color_key IN ('protocol','storage','interface','operations','research')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One owned topic per page. Nullable on purpose: "untopiced" is a real,
-- visible state the UI reports and offers to fix, not a gap to backfill with
-- a guess. ON DELETE SET NULL because deleting a topic must not delete pages.
ALTER TABLE docs.pages
    ADD COLUMN topic_id UUID REFERENCES docs.topics(id) ON DELETE SET NULL;
CREATE INDEX docs_pages_topic_idx ON docs.pages (topic_id) WHERE deleted_at IS NULL;

-- Free-form, many per page. The tag IS the key — no id, no tags table.
-- A tag has no properties beyond its own text, so a lookup table would add a
-- join to every read in exchange for renaming a string, and tags are cheap
-- enough to re-type that renaming is not the operation anyone needs.
CREATE TABLE docs.page_tags (
    page_id UUID NOT NULL REFERENCES docs.pages(id) ON DELETE CASCADE,
    tag     TEXT NOT NULL CHECK (tag = lower(tag) AND length(tag) BETWEEN 1 AND 40),
    PRIMARY KEY (page_id, tag)
);
-- The facet query: every page carrying a given tag.
CREATE INDEX docs_page_tags_tag_idx ON docs.page_tags (tag);

-- The five topics the mockup names. Seeded here rather than left to the
-- application because color_key's CHECK already fixes the set at five — a
-- topic table that starts empty would let the first write choose names the
-- design system has no hue for.
INSERT INTO docs.topics (id, name, color_key) VALUES
    ('018f2b1c-0000-7000-8000-0000000000a1', 'Protocol',   'protocol'),
    ('018f2b1c-0000-7000-8000-0000000000a2', 'Storage',    'storage'),
    ('018f2b1c-0000-7000-8000-0000000000a3', 'Interface',  'interface'),
    ('018f2b1c-0000-7000-8000-0000000000a4', 'Operations', 'operations'),
    ('018f2b1c-0000-7000-8000-0000000000a5', 'Research',   'research');

-- +goose Down
DROP TABLE docs.page_tags;
DROP INDEX docs.docs_pages_topic_idx;
ALTER TABLE docs.pages DROP COLUMN topic_id;
DROP TABLE docs.topics;
