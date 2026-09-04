-- +goose Up
-- § 20's rule, made structural: "A notification is a pointer to an anchor,
-- never a copy of the text."
--
-- The `message` column was enough while `welcome` was the only kind, because
-- a welcome message is about nothing that can change. A mention is about a
-- specific range of characters in a specific block, and every part of the
-- sentence a reader wants to see — who mentioned them, which page, what the
-- words are now — belongs to a service that is allowed to change it. Copying
-- any of it here would produce an inbox that quotes a page as it was, with no
-- way to tell that it has moved on.
--
-- So: no `page_title`, no `actor_name`, no quoted body. Ids only, and the
-- reader resolves them. A mention whose words were deleted then says so
-- rather than quoting a ghost.
ALTER TABLE notify.notifications ADD COLUMN pointer JSONB;

-- Who the notification is ABOUT the actions of, as opposed to user_id, who
-- it is FOR. Nullable: 'welcome' has no actor — nobody registers you.
ALTER TABLE notify.notifications ADD COLUMN actor_id UUID;

-- +goose Down
ALTER TABLE notify.notifications DROP COLUMN pointer;
ALTER TABLE notify.notifications DROP COLUMN actor_id;
