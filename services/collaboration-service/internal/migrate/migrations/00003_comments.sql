-- +goose Up
-- v3.2.0's comment threads. See docs/api/comments.md §0 for why they live
-- here and why they are not ops.

CREATE TABLE collab.comment_threads (
    id           UUID PRIMARY KEY,
    -- No FK: pages are document-service's (DATA_MODEL.md §1).
    page_id      UUID NOT NULL,
    block_id     UUID NOT NULL,
    -- The extent, as ANCHORS rather than offsets — the whole reason a
    -- thread survives a concurrent edit. An offset pair drifts to the
    -- wrong words the moment somebody types above it.
    anchor_start JSONB NOT NULL,
    anchor_end   JSONB NOT NULL,
    -- The text the thread was opened ON, captured once and never updated.
    -- Not a cache of what the anchors resolve to: the anchored text
    -- changes as people edit, and a quote that followed those edits would
    -- make old remarks read as replies to new words.
    quoted       TEXT NOT NULL,
    -- A STATE, not a delete. A resolved thread stays readable because the
    -- argument in it is often why the page reads the way it does.
    resolved_at  TIMESTAMPTZ,
    resolved_by  UUID,
    created_by   UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX comment_threads_open ON collab.comment_threads (page_id) WHERE resolved_at IS NULL;
CREATE INDEX comment_threads_by_block ON collab.comment_threads (page_id, block_id);

CREATE TABLE collab.comments (
    id         UUID PRIMARY KEY,
    thread_id  UUID NOT NULL REFERENCES collab.comment_threads(id) ON DELETE CASCADE,
    author_id  UUID NOT NULL,
    body       TEXT NOT NULL,
    -- Edited in place rather than versioned. A comment is a remark, not a
    -- document, and giving it an op log would be building a second editor
    -- inside the first.
    edited_at  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX comments_by_thread ON collab.comments (thread_id, created_at);

-- +goose Down
DROP TABLE collab.comments;
DROP TABLE collab.comment_threads;
