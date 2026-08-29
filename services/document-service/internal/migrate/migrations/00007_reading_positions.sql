-- +goose Up

-- v2.8.0's resume (DATA_MODEL.md § Reading positions).
--
-- The dashboard's "resume" is not a recent-files list. Recent files are
-- derivable from updated_at and say only THAT a page changed; resume says
-- where you were in it, which nothing in the tree records.
--
-- That position is view state, and RFC-001 §1's rule applies with a sharper
-- edge than it does to toggle collapse: if the caret were model state, moving
-- your cursor would be a collaborative edit that moved everyone else's. So it
-- lives beside the document rather than in it.
CREATE TABLE docs.reading_positions (
    user_id    UUID NOT NULL,          -- auth.users(id), no FK: cross-schema
    -- ON DELETE CASCADE is the whole argument for document-service owning
    -- this: a position in a page that no longer exists is not a position.
    -- In auth-service that cascade would be a cross-service saga to delete a
    -- row nobody would miss.
    page_id    UUID NOT NULL REFERENCES docs.pages(id) ON DELETE CASCADE,

    -- Where the caret was. A block id rather than an offset into the page:
    -- a page-level offset is invalidated by any concurrent edit before it,
    -- which is RFC-002 §2's anchors-over-offsets argument one level up.
    block_id   UUID,

    -- Offsets WITHIN that block, which is tolerable where a page-level offset
    -- would not be: a block is small, a stale offset lands a few characters
    -- off, and resume is advisory — it is not an op and nothing replays it.
    caret_start INT NOT NULL DEFAULT 0,
    caret_end   INT NOT NULL DEFAULT 0,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One row per (user, page), upserted. A history of where you have been is
    -- a different feature with a different retention story; this answers one
    -- question and cannot grow without bound.
    PRIMARY KEY (user_id, page_id)
);

-- "Where was I?" — one user's most recent positions, newest first.
CREATE INDEX docs_reading_positions_recent_idx
    ON docs.reading_positions (user_id, updated_at DESC);

-- +goose Down
DROP INDEX docs.docs_reading_positions_recent_idx;
DROP TABLE docs.reading_positions;
