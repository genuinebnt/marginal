-- +goose Up

-- v2.5.0's full-text search: search.html's own "search is FULL TEXT plus
-- the link graph, not vectors" — Postgres FTS (websearch_to_tsquery +
-- ts_rank/ts_headline), stood in for Tantivy at this repo's scope (an
-- in-process, embeddable-index choice, docs/planning/RELEASES.md's own
-- wording — no second store to run or reconcile). GENERATED ALWAYS ...
-- STORED keeps the vector transactionally consistent with the row it
-- indexes, unlike the BK-tree title index (internal/bktree), which
-- deliberately has its OWN rebuild cadence (search.html's own admitted
-- gap) since it lives in process memory, not in Postgres.
ALTER TABLE docs.pages
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', title)) STORED;
CREATE INDEX docs_pages_search_vector_idx ON docs.pages USING GIN (search_vector);

ALTER TABLE docs.blocks
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(content->>'text', ''))) STORED;
CREATE INDEX docs_blocks_search_vector_idx ON docs.blocks USING GIN (search_vector);

-- +goose Down
DROP INDEX docs.docs_blocks_search_vector_idx;
ALTER TABLE docs.blocks DROP COLUMN search_vector;
DROP INDEX docs.docs_pages_search_vector_idx;
ALTER TABLE docs.pages DROP COLUMN search_vector;
