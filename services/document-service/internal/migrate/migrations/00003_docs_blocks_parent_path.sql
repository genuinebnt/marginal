-- +goose Up

-- Adds RFC-001 §1's containment (Quote/Toggle/List/ListItem nesting,
-- documentcore's Block.Parent) to docs.blocks — deferred at 00002's own
-- migration time ("a tree column earns its keep only once a nesting
-- kind actually exists"); that kind now exists (documentcore's
-- List/ListItem/Toggle/Image). Mirrors docs.pages' own parent_id/path
-- shape one level deeper (DATA_MODEL.md § Blocks).
--
-- path is nullable at the column level, not NOT NULL like docs.pages' —
-- internal/blockproj always computes and supplies a real value on every
-- insert (persist()), so this is a pragmatic allowance for whatever rows
-- already exist from before this migration, not a gap in what blockproj
-- itself ever writes. docs.blocks is a fully-rebuilt-on-every-event
-- projection (blockproj's own doc comment) — any pre-existing row is
-- replaced wholesale, path included, the next time its page receives a
-- collab.ops_flushed event, never patched in place. There is no real
-- production data this repo has ever needed to migrate (CLOUD_ROADMAP.md's
-- cloud track is still pre-apply).
ALTER TABLE docs.blocks ADD COLUMN parent_id UUID REFERENCES docs.blocks(id) ON DELETE CASCADE;
ALTER TABLE docs.blocks ADD COLUMN path LTREE;
CREATE INDEX ON docs.blocks USING GIST (path);

-- +goose Down
ALTER TABLE docs.blocks DROP COLUMN path;
ALTER TABLE docs.blocks DROP COLUMN parent_id;
