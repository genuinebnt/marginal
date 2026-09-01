# Comments API

`docs/ui-mockups/v2/index.html`'s comment threads and `InspectorRail`'s
"Comments" tab, made real (`v3.2.0`, `RELEASES.md`).

Served by **`collaboration-service`, directly** — not through
`api-gateway`, the same convention its WebSocket and its `/trace`,
`/palimpsest` and `/diff` endpoints already follow. Nothing here is gRPC,
and the gateway only translates gRPC.

---

## 0. Why comments live here, and why they are not ops

**Here**, because a comment's extent is an `AnchorRange` — the same stable
range a mark uses (RFC-001 §9) — and an anchor is only resolvable by
whatever holds the block's live rope and its anchor log. That is
`collaboration-service`. `document-service` owning comments would mean
`document-service` resolving anchors it has no rope for.

**Not ops**, and this is the decision the rest follows from. RFC-002's ISA
is about document *mutation*: every op changes the block tree or a block's
text, every op is invertible, and undo walks that log. A comment changes
neither. Making one an op would put comments inside the document's undo
stack, where one `⌘Z` too many silently retracts somebody's remark.

The consequences are worth stating rather than discovering:

- Comments do **not** appear in `/trace`, the palimpsest, or a revision
  diff. Those are views of the op log, and a comment is not in it.
- Comments are **not** covered by `can_apply`. Their own authorization is
  below (§3), and it is deliberately a different question: a viewer may
  comment, because a comment does not change the document.

---

## 1. Routes

| Method | Path | Does |
|---|---|---|
| `GET` | `/collab/pages/{id}/comments` | every thread on the page, newest first, with its comments |
| `POST` | `/collab/pages/{id}/comments` | opens a thread on an anchored range |
| `POST` | `/collab/threads/{id}/comments` | replies to a thread |
| `POST` | `/collab/threads/{id}/resolve` | marks a thread resolved |
| `POST` | `/collab/threads/{id}/reopen` | undoes that |

Opening a thread:

```json
POST /collab/pages/{id}/comments
{
  "block_id": "…",
  "anchor_start": { "item": { "actor": "server", "counter": 12 }, "bias": "before" },
  "anchor_end":   { "item": { "actor": "server", "counter": 19 }, "bias": "after" },
  "quoted": "anchors survive a split",
  "body": "Is this still true after a merge?"
}
```

The client sends anchors it was given — a block's `boundaries` from a
snapshot or an ack — never offsets it computed itself. An offset pair drifts
to the wrong words the moment somebody types above it, which is the exact
failure "Anchors vs offsets" describes, and the reason the wire takes
anchors at all.

`quoted` is captured **at creation and never updated**. It is not a cache of
what the anchors resolve to: the anchored text changes as people edit, and a
quote that silently followed those edits would make old remarks read as
replies to new words.

## 2. Reading a thread

```json
{
  "threads": [
    {
      "id": "…", "block_id": "…", "quoted": "anchors survive a split",
      "resolved_at": null, "created_by": "…", "created_at": "…",
      // Where the anchors point RIGHT NOW, resolved against the live rope.
      // null when the text they named is gone.
      "range": { "start": 12, "end": 35 },
      "orphaned": false,
      "comments": [
        { "id": "…", "author_id": "…", "body": "Is this still true?", "created_at": "…" }
      ]
    }
  ]
}
```

**`orphaned` is a state, not a deletion.** When the text a thread points at
is gone, the thread is still returned — attached to its block, marked, still
carrying `quoted`. Deleting somebody's remark because somebody else edited a
sentence is a worse failure than an untidy list, and a comment about text
that no longer exists is frequently the most interesting one there.

Resolving is likewise a state. A resolved thread stays readable: the
argument in it is often why the page reads the way it does.

## 3. Who may comment

Authenticated, and a **member of the page's space** — any role, including
`viewer`.

That is deliberate and it is the one place this repo's roles do not line up
with `can_apply`. A viewer may not change the document; a comment does not
change the document. A permission model where reading a page but not being
able to ask a question about it is the default would make `viewer` a role
nobody would give anybody.

Editing or deleting a comment is the **author's** only. An admin of the
space may resolve any thread — resolution is housekeeping, not authorship.

## 4. Not in this version

- **@mentions and notifications.** The parser is easy; deciding what a
  mention *does* (notify how, digest when) is `v3.3.0`'s question and
  answering it here would answer it twice.
- **Reactions.** Same table shape, different noun, and no anchor at all —
  they attach to a comment rather than to a range. Deliberately separate so
  that the anchored half can be finished and used first.
- **Comment-level permissions** beyond authorship. A thread visible to some
  members and not others is a second permission tier, and `ADR-013` ruled
  per-page overrides out for the same reason.
