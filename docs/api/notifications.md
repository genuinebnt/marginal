# Notifications

`notification-service` — reached **directly**, not through `api-gateway`,
the same convention `collaboration-service`'s own HTTP routes already use.
Every route requires a bearer token and answers only about the caller.

Mockups: `§ 20` (the inbox) and `§ 24c` (the bell's panel).

---

## 1. The rule this contract is built on

> **A notification is a pointer to an anchor, never a copy of the text.**
> Open it a week later and it still lands on the right words, wherever they
> moved. — `§ 20`

That is not a stylistic preference; it decides the schema. A stored
sentence ("Ada mentioned you in *Sync protocol notes*") freezes three
things that belong to other services and are allowed to change: the
person's display name, the page's title, and the words that were being
discussed. An inbox that renders such a row is quoting the past while
looking like it describes the present.

So `notify.notifications` stores **ids** for every kind whose content can
change, and the client assembles the sentence at read time from the
services that own each part. A mention whose words were since deleted then
says so, because the comments API reports the anchor as orphaned rather
than resolving it to something else.

---

## 2. `GET /notifications`

```json
{
  "notifications": [
    {
      "id": "0192...",
      "kind": "mention",
      "message": "",
      "actor_id": "0192...",
      "pointer": {
        "page_id": "0192...", "block_id": "0192...",
        "thread_id": "0192...", "comment_id": "0192...",
        "actor_id": "0192...", "user_id": "0192..."
      },
      "read_at": null,
      "created_at": "2026-09-02T14:06:00Z"
    },
    {
      "id": "0192...",
      "kind": "welcome",
      "message": "Welcome to Marginal, Ada!",
      "created_at": "2026-09-01T09:00:00Z"
    }
  ]
}
```

`message` and `pointer` are **disjoint by kind**, not merely usually
different:

| Kind | `message` | `pointer` | Why |
|---|---|---|---|
| `welcome` | the sentence | absent | It is about nothing that can change |
| `mention` | empty | `MentionPointer` | Every part of it is owned elsewhere |

A client renders a `mention` by resolving its pointer:
`GET /collab/pages/{page_id}/comments` for the thread (which carries the
live anchor range and the `orphaned` flag), `GET /pages/{page_id}` for the
title as it is now, and the people list for the actor's current name.

Unknown fields inside `pointer` are **passed through verbatim**. The
payload is stored as it arrived, so a field a newer
`collaboration-service` adds reaches a client that understands it even
before `notification-service` does.

## 3. `POST /notifications/{id}/read` · `POST /notifications/read-all` · `GET /notifications/unread-count`

Unchanged from `v2`. Read-marking is scoped by user as well as by id — an
id alone would be a bearer token for someone else's inbox — and is
idempotent, so a second click reports `0` rather than moving the
timestamp.

---

## 4. `collab.comment_mentioned`

Published by `collaboration-service`'s outbox, **in the same transaction as
the comment itself**: a mention cannot be delivered for a comment that
failed to save, and a comment cannot save while its mentions quietly go
nowhere.

**One row per person mentioned**, so redelivery, read state and dedup are
all per recipient. The dedup key is the outbox row id, as for every other
topic here.

### Who can be mentioned

Only members of the **space the page is in**. The candidate list comes from
`SpaceService.ListMembers` called *as the comment's author*, so mentioning
can never reach further than the author can already see. Without that, a
comment body would be a way to send a notification to any account on the
instance.

### The handle grammar

`@` followed by letters, digits, `.`, `_`, `-`. An `@` preceded by a word
character is an **email address, not a mention**. Trailing punctuation is
part of the sentence. Matching is against the display name with spaces
removed, case-folded — `@AdaLovelace` finds "Ada Lovelace".

Three deliberate non-errors, because none of them should cost somebody
their comment:

- **A handle nobody answers to** is dropped. People type names that are
  not accounts.
- **Mentioning yourself** notifies nobody (`§ 20` WHAT NEVER NOTIFIES:
  "Your own ops").
- **A collision** — two display names normalising to one handle — resolves
  to one person, not both. `@ada` meaning "everyone called Ada" cannot be
  corrected by typing more.

A failure to *resolve* (document-service or auth-service unreachable)
drops the mention and logs it loudly; the comment still saves. A failure
to *write* rolls back both.

---

## 5. Known gap: core NATS does not redeliver

`Subscribe`'s existing caveat applies here and bites harder than it did.
A welcome notification lost while `notification-service` was down is a
greeting nobody misses. **A mention lost that way is somebody being told
nothing while believing they were asked a question** — and the sender has
no way to know.

This is the first notification kind whose loss actually matters, which is
the concrete argument for JetStream (a stream, a durable consumer, ack
semantics) that this repo previously did not have. Recorded here rather
than silently accepted. The outbox row is not deleted, so the evidence
survives even when the delivery does not.
