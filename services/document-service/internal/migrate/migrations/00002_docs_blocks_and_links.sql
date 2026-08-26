-- +goose Up

-- docs.blocks is document-service's materialised read model of a page's
-- block content — the source of truth is collab.ops
-- (collaboration-service's own database); this table is rebuilt from the
-- collab.ops_flushed event stream (internal/blockproj), never written to
-- directly by any request handler (DATA_MODEL.md § collab.ops → docs.blocks).
--
-- No parent_id/path (LTREE) yet, unlike DATA_MODEL.md's fuller sketch —
-- every block kind documentcore implements today (paragraph, heading,
-- quote, code_block, divider) is flat; a tree column earns its keep only
-- once a nesting kind (list, toggle) actually exists (agents.md §3:
-- feature depth, not surface area, and PROJECT_STRUCTURE.md's "don't
-- design for hypothetical future requirements").
--
-- position is a plain integer, not a fractional sort_key like docs.pages
-- — a projection is fully rewritten from its source event stream on every
-- change (internal/blockproj), so there's no concurrent-independent-writer
-- reordering problem a fractional key exists to solve here.
CREATE TABLE docs.blocks (
    id         UUID PRIMARY KEY, -- same id collaboration-service's ops name — the projection's join key to its source
    page_id    UUID NOT NULL REFERENCES docs.pages(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    kind       JSONB NOT NULL, -- documentcore.BlockKind's own tagged-object JSON shape
    content    JSONB NOT NULL DEFAULT '{}', -- documentcore.Content{text, marks}
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON docs.blocks (page_id, position);

-- docs.page_links — backlinks, extracted from each block's plain text by
-- internal/blockproj scanning for `[[Page Title]]`. target_page is NULL
-- for a dangling link (RFC-003 §6's DanglingPageLink diagnostic, not
-- built in this repo's scope yet, but the data this projection produces
-- is exactly what that diagnostic and a future graph/embedding feature
-- both need — DATA_MODEL.md's own schema, brought forward here because
-- it's now buildable, not because diagnostics-service exists).
CREATE TABLE docs.page_links (
    id           UUID PRIMARY KEY,
    from_page    UUID NOT NULL REFERENCES docs.pages(id) ON DELETE CASCADE,
    from_block   UUID NOT NULL REFERENCES docs.blocks(id) ON DELETE CASCADE,
    target_title TEXT NOT NULL,
    target_page  UUID REFERENCES docs.pages(id), -- NULL = dangling
    UNIQUE (from_block, target_title)
);
CREATE INDEX ON docs.page_links (lower(target_title));
CREATE INDEX ON docs.page_links (target_page);

-- +goose Down
DROP TABLE IF EXISTS docs.page_links;
DROP TABLE IF EXISTS docs.blocks;
