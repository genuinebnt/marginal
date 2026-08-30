# API — Collaboration

**Status:** Implemented in Go (`services/collaboration-service/internal/wsapi`,
built on `internal/session`) — connect, snapshot, submit an op, get acked,
broadcast to every other connected client, real join/leave presence
(§3's `"presence"` frame — per-actor, not per-connection), real live
cursor tracking (§2/§3's `"cursor"` frame — added 2026-08-26, at explicit
request, superseding the earlier "presence answers who's here, not
where" line), error frames for a bad message. A `Session` now reconciles
RFC-002 §2's two ISA tiers into one system (`internal/pageop`): structural
block ops (insert/delete/reorder/kind — a `documentcore.Page`) and
character ops scoped to one block's own live rope (`internal/doctext`,
via `internal/ops`) share the same WAL, flush pipeline, and broadcast.
**The frontend (`web/`'s `RichEditorPane`/`useCollabPage`) speaks this
protocol** — see `CLAUDE.md`'s "Block/live-text reconciliation" note.
**Owners:** `collaboration-service` (WebSocket) · `api-gateway` (out of this
repo's scope — see "Auth" below for what that means today)
**Related:** `RFC-002` (op ISA, invertibility, WAL) · `ARCHITECTURE.md` §4
(Request Flow — Live Editing) · `docs/porting/PROGRESS.md`'s session/wsapi
entries

Unlike `pages.md`/`auth.md`, there is **one contract, not two**: no REST
projection exists or is planned for this endpoint — a live editing session
is inherently a persistent connection, not a request/response resource, so
`api-gateway`'s REST-translation role (§2 in the other two docs) doesn't
apply here even conceptually.

```
   browser  ──WebSocket/JSON──▶  collaboration-service
```

---

## 1. Connecting

```
GET /collab/pages/{id}?actor_id=<uuid>&actor_kind=user
Upgrade: websocket
```

or, for a non-browser caller that can set headers on the upgrade request
(`grpcurl`-style tools, another service, a test):

```
GET /collab/pages/{id}
Upgrade: websocket
X-Actor-Id: <uuid>
X-Actor-Kind: user            # optional, defaults to "user"
```

**A real browser must use the query parameters, not the headers.** The
WebSocket browser API has no mechanism to set custom headers on the
upgrade request at all — not a gap in this API, a characteristic of the
browser API itself. The header form exists because it matches
`pages.md`/`auth.md`'s convention for every other service; the query-param
form exists because this is the one endpoint in the repo an actual browser
connects to directly without going through `fetch` (which can set
headers) first. If both are present, the header wins.

`{id}` is the page's id (a UUID, matching `document-service`'s `Page.id`).
There is no check today that the page actually exists in `document-service`
— `collaboration-service` owns `collab.ops` independently (`DATA_MODEL.md`
§1: no cross-schema joins) and will happily open a session for any UUID.
Opening a session for a page that document-service has no record of is a
client-side bug, not something this service can detect on its own.

On success, the connection upgrades and the server immediately sends one
`snapshot` frame (§3) with the whole page — title and every block's current
live text — replayed from `collab.ops` plus any locally-recovered,
not-yet-flushed WAL records (`internal/session`'s `open`; see
`PROGRESS.md`'s crash-recovery entry).

### Auth is a temporary stand-in

There is no JWT verification here. The actor id/kind are read directly
off the request (header or query param, per above), unauthenticated —
the same convention `document-service`'s `PageService` already uses
(`pages.md`'s "temporary scaffolding, not the real trust boundary")
because no `api-gateway` exists in this repo's scope to have verified a
token upstream. **Do not treat this as a real security boundary.** A
missing or unparseable actor id, or an invalid actor kind (must be one of
`user`/`agent`/`plugin`/`system`), gets `401`/`400` before the WebSocket
upgrade even happens.

---

## 2. Client → Server: submitting an op, or reporting a cursor

A client sends one of two message types. `"op"` is the durable one —
everything in this section below the example. `"cursor"` is the other:

```json
{ "type": "cursor", "cursor": { "block_id": "<block-uuid>", "start": 3, "end": 7 } }
{ "type": "cursor", "cursor": { "block_id": null, "start": 0, "end": 0 } }
```

Fire-and-forget — no `ack`, never touches the WAL/op log/`can_apply`
(`internal/session.CursorEvent`'s own doc comment: this is where someone
*is right now*, not a fact worth reconstructing on replay). `start`/`end`
are rune offsets into `block_id`'s live text, the same unit `InsertText`/
`DeleteText` already use (`start === end` is a plain caret, not a
selection). `block_id: null` clears the sender's cursor — they blurred
out of every block — rather than leaving a stale position parked
somewhere they've since left; `start`/`end` are ignored in that case.

The only message that commits anything:

```json
{
  "type": "op",
  "op": { "scope": "text", "block": "<block-uuid>", "op": { "type": "InsertText", "at": null, "text": "hello" } },
  "undo_group": "3fae2f9e-...-uuid"
}
```

`undo_group` is optional — omit it (or send `null`) for a single-op edit,
RFC-002 §3's "a group of one." Set it to the same UUID on every op that
belongs to one user gesture (a paste, the `## ` input rule's `SetBlockKind`
+ `DeleteText` pair, one accepted assistant proposal) so a later `"undo"`
(§2.1 below) reverts the whole gesture in one step instead of one twentieth
of it. **The client generates this id and owns the grouping decision** —
RFC-002 §3: "assigned by whoever originates the gesture... never the
server, which cannot know where a gesture began."

### 2.1. Undo and redo

```json
{ "type": "undo" }
{ "type": "redo" }
```

No payload — undo/redo apply to **the sender's own actor id**, scoped to
this page. The server pops that actor's most recent `undo_group` (or the
most recent single op, if it wasn't grouped) off a durable, per-actor stack
reconstructed from `collab.ops` itself on session open, not a client-side
history — RFC-002 §3: "putting it in the log rather than a client-side
stack." Each op in the popped group is inverted and re-applied against
**current** state (never a stale snapshot), oldest-original-op undone
last — the same reason `documentcore.History` already applies an inverse
to the live page rather than restoring a snapshot: this session's ops
address content by stable id/anchor (`BlockID`, `ItemID`), so an inverse
computed against today's state is correct even if other actors' ops landed
in between, without a separate OT transform step.

The server acknowledges an `"undo"`/`"redo"` the same way it acknowledges
an `"op"`: one `"ack"` frame per op the action actually committed (a
grouped gesture of N ops produces N `"ack"` frames, in the order they were
applied), each also broadcast (`"broadcast"`) to every other connection —
from every other client's point of view, an undo is indistinguishable from
the sender submitting N ordinary ops, because it is one. An empty stack is
a **no-op**, not an `error` frame — `documentcore.History`'s own contract,
carried through unchanged.

**Not atomic across a multi-op group.** Each op in the popped group commits
through the normal durable pipeline (WAL, broadcast, flush-enqueue) as it's
applied, one at a time — if op *k* of a group fails to apply (its target
block was deleted by someone else since), ops `1..k-1` are already
committed and cannot be un-committed for free. The remaining, not-yet-applied
tail of the group is left as a new pending group on the actor's stack (so a
second `"undo"` continues where the first one stopped) and the failure
surfaces as an `"error"` frame. This only matters for genuinely multi-op
gestures; a single-op undo (the common case) is atomic the same way
`documentcore.History.Undo` already is.

Redo is **not** reconstructed from `collab.ops` on reconnect — it's
in-memory only per session, cleared (like every editor's redo stack) the
moment a new op commits for that actor, matching `documentcore.History`'s
existing "a new op invalidates redo" rule.

### 2.2. Restore to a point

```json
{ "type": "restore", "to_step": 3 }
```

`docs/ui-mockups/v2/index.html § 17 HISTORY`'s "restore to a point," made real
(`v2.4.0`) — brings the live document back to its state as of right
after step `to_step` of this page's own confirmed op log, 0-indexed, the
same indexing `GET /collab/pages/{id}/trace`'s own `steps` array uses
(§5). A client builds its scrubber against that endpoint, then sends
`to_step` from whichever step the scrubber is parked on.

This is **repeated undo, not a restore-from-backup** — the same
`internal/session.Trace` replay §5 already exposes read-only computes,
each step's own already-known inverse, applied backward from the current
tip through the exact same `commitOpLocked` pipeline `"undo"`/`"redo"`
use (WAL, broadcast, flush-enqueue), most-recent-step first. From every
other connection's point of view this is indistinguishable from the
requesting actor submitting that many ordinary `"op"` messages, because
it is that: each reverted step gets its own `"ack"`/`"broadcast"` frame,
same as `"undo"`'s own multi-op contract above.

The whole restore becomes **one new undo group** for the requesting
actor — a single `"undo"` afterward reapplies every step it just
reverted, in their *original* order (not the restore's own reverse
order: each step's own precondition, e.g. `SetBlockContent`'s `Prev`,
was only ever valid against the state that existed right before it, so
reapplying them out of order fails).

`to_step` must satisfy `0 <= to_step < (the current confirmed step
count)` — one past the end (i.e. "restore to now") is a **no-op**, not
an `error` frame, matching `"undo"`/`"redo"`'s own empty-stack contract;
anything else out of range is an `"error"` frame naming the problem, not
an `"internal error"` (`session.ErrOutOfRange`). Same not-atomic-across-
a-multi-step contract as a grouped `"undo"`: if step *k* of the range
fails to revert (its target was touched by someone else since), the
steps already reverted stay reverted and the failure surfaces as an
`"error"` frame — a second `"restore"` (or plain `"undo"`) can be
retried from there.

Shares `Trace`'s own documented visibility boundary (§5): it replays
*confirmed* rows, so an op still sitting in this session's own
not-yet-flushed WAL tail at the moment `"restore"` runs is invisible to
it — the same accepted gap `Trace` already states plainly, unlikely to
matter in practice for a manual, deliberate click (flush is sub-second).

`op` is `internal/pageop.Op`, JSON-encoded exactly as `pageop.Marshal`
produces it — one of two scopes, each nesting its own tier's op:

```jsonc
// scope: "block" — structural, whole-page. The nested op is one of
// documentcore's six block-tier variants (RFC-002 §2), tagged the same
// way (documentcore.MarshalOp) and merged into this envelope directly —
// there is no extra nesting level for this scope, unlike "text" below.
{ "scope": "block", "type": "InsertBlock", "id": "<block-uuid>", "after": null, "kind": { "tag": "paragraph" }, "content": { "text": "" } }
{ "scope": "block", "type": "DeleteBlock", "tombstone": { "id": "...", "kind": {...}, "content": {...} }, "after": null }
{ "scope": "block", "type": "SetBlockKind", "id": "...", "from": { "tag": "paragraph" }, "to": { "tag": "heading", "level": 2 } }
{ "scope": "block", "type": "SetBlockContent", "block": "...", "prev": { "text": "old" }, "content": { "text": "new" } }
{ "scope": "block", "type": "SetTitle", "page": "...", "from": "", "to": "Untitled" }
{ "scope": "block", "type": "MoveBlock", "id": "...", "from": null, "to": "<other-block-uuid>" }

// scope: "text" — character-granular, scoped to exactly one block's own
// live rope (RFC-002 §2.1). "block" names which block; "op" is one of
// internal/ops' two variants, tagged the same way ops.MarshalOp always
// has (unchanged from before this envelope existed):
{ "scope": "text", "block": "...", "op": { "type": "InsertText", "at": null, "text": "hello" } }
{ "scope": "text", "block": "...", "op": { "type": "DeleteText", "range": { "start": {...}, "end": {...} }, "text": "" } }
```

`"at": null` on `InsertText` means "the start of that block's text" — the
one position nothing can anchor to yet. `text` on `DeleteText` is ignored
on the way in (the server fills it from what it actually deletes) —
sending it is harmless, not required. `SetMark` doesn't exist yet
(`internal/doctext` has no mark storage — `PROGRESS.md`).

A block must exist (a prior `InsertBlock` committed) before any `"text"`
op naming it can apply — an unknown block id is an `error` frame, not a
silent no-op. `SetBlockContent`'s `prev` must equal that block's current
content exactly (`documentcore.Page.Apply`'s precondition), which is kept
in sync with the block's live rope after every `"text"` op that touches
it — so a `SetBlockContent` right after live typing sees that typing's
result, not a stale snapshot.

Every submitted op is authorized through `can_apply` (`RFC-002` §5) before
touching anything — it always allows today (single-tenant scope), same as
the spec itself says: `fn can_apply(op, actor) -> bool { true } // today`.

---

## 3. Server → Client frames

Every frame the server sends has this shape:

```jsonc
{ "type": "snapshot", "snapshot": { "page_id": "...", "title": "", "blocks": [ /* BlockSnapshot[] — see below */ ] }, "present": ["actor-id", "..."], "cursors": [ /* CursorWire[] — see below; omitted/empty if nobody has one set */ ] }
{ "type": "ack",       "op": { /* the committed LoggedOp — see below */ }, "boundaries": { /* set only for a "text" op; omitted for "block" or an emptied block */ } }
{ "type": "broadcast", "op": { /* the same shape, for everyone else */ }, "boundaries": { /* same as the ack's */ } }
{ "type": "presence",  "actor_id": "...", "joined": true }
{ "type": "cursor",    "cursor": { "actor_id": "...", "block_id": "<block-uuid>", "start": 3, "end": 7 } }
{ "type": "error",     "message": "human-readable reason" }
```

- **`snapshot`** — sent once, immediately after connecting (§1). Each
  entry in `blocks` (`session.BlockSnapshot`) is:
  ```json
  { "id": "...", "kind": { "tag": "paragraph" }, "text": "current live text", "boundaries": { /* or omitted if this block is empty */ } }
  ```
  `text` is that block's *live* rope content, which can differ from
  whatever a `SetBlockContent`/`InsertBlock` op last recorded if
  character-level edits happened since — the rope, not the last-recorded
  `Content`, is authoritative during a live session (`DATA_MODEL.md`).
  Each block's own `boundaries` lets a reconnecting client build its
  first edit into that block without waiting on an ack of its own.
  `present` (top-level, sibling to `snapshot`) is every distinct actor
  already connected to this page at join time — omitted/empty if nobody
  else is here. Use it to seed a presence list immediately; don't wait
  for a future `"presence"` event to learn who's already here. `cursors`
  is `present`'s own last-known caret/selection — only for those who
  currently have one set (see the `"cursor"` frame below) — so a joining
  client can render every already-connected peer's live caret
  immediately, not just their avatar.
- **`presence`** — sent to every *other* connection when an actor's
  *first* connection joins (`joined: true`) or their *last* connection
  closes (`joined: false`). A second tab/device from an actor already
  present does **not** re-send `joined: true`, and closing just one of
  several open connections for the same actor does **not** send
  `joined: false` — presence is per-actor, not per-connection
  (`internal/session.Session.Subscribe`'s own bookkeeping). A departing
  actor's cursor is cleared (a synthetic `"cursor"` frame with
  `block_id: null`) in the same moment their `joined: false` goes out —
  a gone actor's stale caret must not linger for whoever's still here.
- **`cursor`** — one actor's caret/selection just changed: `block_id`
  names which block (`null` means they blurred out of every block —
  `start`/`end` are meaningless then), `start`/`end` are rune offsets
  into that block's live text (`start === end` is a plain caret). Sent to
  every *other* connection on the page, never echoed back to whoever
  reported it. Purely ephemeral — never appears in an `ack`/`broadcast`,
  the op log, or a replay; a fresh connection only ever learns current
  cursors from the snapshot's own `cursors` list above.
- **`ack`** — sent only to the connection that submitted the op, in
  response to its own `op` message. This is the acknowledgement point:
  by the time this frame is sent, the op is durable in the local WAL
  (`RFC-002` §6 — "the client is acknowledged after the local WAL sync,
  not after Postgres") and has already been broadcast to every other
  connected client.
- **`broadcast`** — sent to every *other* connection subscribed to the
  same page. The submitter never receives its own op back this way — its
  `ack` frame is the only confirmation it gets, by design
  (`internal/session`'s `Subscriber` doc comment).
- **`error`** — sent for a malformed message, an unknown `op` variant, a
  `"text"` op naming a block that doesn't exist, an op that fails to apply
  (e.g. an `Anchor` naming an `ItemID` this session never saw, or a
  `SetBlockContent` whose `prev` doesn't match), or a `can_apply` denial.
  **The connection stays open** — one bad message doesn't end the
  session, so a client can recover from a transient bug without
  reconnecting.

### `boundaries` — for a client with no `Anchor` of its own

`ack` and `broadcast` both carry the *submitted-or-affected block's*
current start/end anchors, only when that op was a `"text"` op:

```json
"boundaries": {
  "start": { "item": { "actor": "...", "counter": 1 }, "bias": "before" },
  "end":   { "item": { "actor": "...", "counter": 5 }, "bias": "after" }
}
```

— **omitted** for a `"block"` (structural) op, and for a `"text"` op that
leaves the block empty. A plain-text client (a `<textarea>`, say) that
only ever sees rune offsets, never an `ItemID`, has no way to build a
`DeleteText` naming the text it wants removed — its only source of a
valid `Anchor` is a frame the server already sent it. `boundaries` names
a block's whole live text as one range, so such a client can implement
editing as "replace this block's text": send a `DeleteText` (scoped to
that block) using its most recent `boundaries`, followed by an
`InsertText` at `at: null` with the new full text, then use the next
ack's `boundaries` for that block's following edit. This is
`doctext.Text.Boundaries`'s whole reason to exist — see that method's doc
comment for why `DeleteText` only ever needs a range's first and last
item, not every item in between.

### The committed op (`ack`/`broadcast`'s `op` field)

The full `oplog.LoggedOp`, RFC-002 §4's permanent wire format:

```json
{
  "id": "...",
  "version": 1,
  "page_id": "...",
  "actor_id": "...",
  "actor_kind": "user",
  "undo_group": null,
  "vector_clock": { "<actor-id>": 3 },
  "op": { "scope": "text", "block": "...", "op": { "type": "InsertText", "at": null, "text": "hello" } },
  "created_at": "2026-08-26T08:29:32.966828Z"
}
```

`id` is a UUIDv7 — every consumer that ever needs to deduplicate (there is
none yet on the wire side; `internal/opstore`'s Postgres flush already
dedupes on it) should key on this field, per RFC-002 §4 rule 5.

---

## 4. Disconnection

A connection is closed by the server, without a specific error frame
first, in exactly one situation: **its own outbound buffer filled up**
(64 frames) because it stopped reading fast enough. This is deliberate,
not a bug — `internal/session.ApplyClientOp` broadcasts to every
subscriber while holding the whole session's lock, so a slow reader must
never be allowed to block delivery to everyone else on the page. A
disconnected client just reconnects; §1's snapshot-on-connect makes that
safe and cheap (no state to reconcile client-side beyond re-rendering the
fresh snapshot).

There is no explicit ping/pong or idle-timeout policy documented yet —
left to `coder/websocket`'s defaults for now.

---

## 5. `GET /collab/pages/{id}/trace` — the op-log debugger's data source

Plain HTTP, not a WebSocket — "give me the whole confirmed replay once,"
not a live session. Backs `docs/ui-mockups/v2/index.html § 13 TRACE`'s "real ops, real
inverses" claim against an actual page's actual op log
(`internal/session.Trace`), rather than that mockup's own synthetic,
fixed nine-op sequence. Read-only: never touches a live `Session`, so it's
safe to call for a page someone else has open right now, and it only ever
sees ops that have already reached `collab.ops` (a session's own
not-yet-flushed WAL tail is invisible to it, same as any other reader of
the confirmed log).

```
GET /collab/pages/{id}/trace
```

```json
{
  "steps": [
    {
      "op": { /* the full oplog.LoggedOp — §3's "the committed op" shape, unchanged */ },
      "inverse": { /* that op's own inverse, in the same "scope"-tagged pageop.Op envelope */ },
      "law_holds": true,
      "after": { /* session.Snapshot — the whole document once this step's op has been applied */ }
    }
  ]
}
```

One entry per confirmed op, oldest first. `law_holds` is
`apply(invert(op), apply(op, doc)) == doc`, **checked for real** by
replaying the log a second time and comparing observable document state
(title, block order/containment/kind/text/marks — not raw CRDT tombstone
bookkeeping, which can legitimately differ between "never touched" and
"deleted then reinserted" while meaning the same thing), not asserted from
`Invert()`'s own claim — `docs/ui-mockups/README.md`'s own principle for
this page: "the badge turns amber the moment it fails," not "the badge
always says holds." `after` is *the whole document*, not a diff — a
client renders "the document at step N" by picking one precomputed entry,
never by re-running `apply()` itself: the algorithm lives in Go
(`ADR-012`), the client only draws what Go already computed.

A malformed or corrupt log entry (one that fails to even replay) is a
`500` — this endpoint has no partial-result contract; either the whole
log replays cleanly and every step is reported, or none are.

---

## 6. `GET /collab/pages/{id}/blocks/{blockId}/palimpsest` — one block's whole character history

Plain HTTP, read-only, same "give me the whole replay once" shape as §5
— never touches a live `Session`. Backs `docs/ui-mockups/v2/index.html § 17 HISTORY`'s
own central claim, made real: "the palimpsest paragraph is a real
persistent sequence. Its whole edit history is a list of ops applied to
a tombstoned char array: a delete sets a version stamp, it never
removes. Reading version v is the filter `ins <= v < del`, so every
version is addressable from ONE structure."

```
GET /collab/pages/{id}/blocks/{blockId}/palimpsest
```

```json
{
  "chars": [
    { "rune": 104, "insert_step": 0, "insert_actor": "...", "delete_step": 1, "delete_actor": "..." },
    { "rune": 105, "insert_step": 0, "insert_actor": "..." }
  ],
  "current_step": 1
}
```

One entry per character this block's live text has *ever* held, oldest-
inserted first, kept forever — never shrinks back down when something is
deleted. `insert_step`/`delete_step` index into the same confirmed op
log §5's `steps` array is indexed by, so a client drives one scrubber
against both endpoints; `delete_step`/`delete_actor` are absent for a
character still live right now. A client reads "this block's text as of
step v" by filtering to `insert_step <= v && (no delete_step || v <
delete_step)` — `internal/palimpsest.AtVersion`'s own job, real Go, not
re-derived in the browser (`ADR-012`). Palimpsest mode (revealing the
tombstones) renders every character regardless of `delete_step`, tinting
a dead one by `delete_actor` and fading it by how long ago `delete_step`
was relative to `current_step`.

Neither `doctext.Text` nor its own `anchor.Log` already gives this for
free — both exist to answer "what does this block look like right now,"
and `anchor.Log`'s tombstoning keeps only enough to resolve an `Anchor`
(identity and liveness), never the character's own value or who deleted
it. `internal/palimpsest.Build` is a second, parallel replay over the
same confirmed ops, scoped to one block — RFC-002's op log is still the
only source of truth; this is a projection of it, the same "a projection,
never a second writer" precedent `document-service`'s `blockproj` already
sets, just read fresh per request instead of materialized.

`chars` is `[]`, never `null`, for a block with no character-tier ops
yet (matching `docs/api/diagnostics.md`'s own empty-array convention);
`current_step` is `-1` in that case. An invalid page or block id is a
`400`; a malformed confirmed log (fails to even replay) is a `500`, same
partial-result contract as §5.

---

## 7. `GET /collab/pages/{id}/diff?from={n}&to={m}` — two revisions, plus every block move between them

Plain HTTP, read-only, same shape as §5/§6. Backs `docs/ui-mockups/
diff.html`: "block-level MOVE detection, which a flat text diff cannot
express... a moved block must read as MOVED, not as delete + insert."

```
GET /collab/pages/{id}/diff?from=3&to=9
```

```json
{
  "before": { /* session.Snapshot — steps[from].After, unchanged */ },
  "after":  { /* session.Snapshot — steps[to].After, unchanged */ },
  "moves": [
    { "block_id": "...", "from_parent": null, "from": null, "to_parent": null, "to": "...", "step": 6 }
  ]
}
```

`from`/`to` index into the same confirmed step array §5's `steps` is
indexed by — `from == to` is a valid, zero-length diff; `from > to` is a
`400`. `before`/`after` are exactly two already-computed `Trace` entries,
picked rather than recomputed — the same "the client draws what Go
already computed" principle §5 states, extended to picking two entries
instead of one.

`moves` is every `MoveBlock` op strictly after `from` and at or before
`to` — a filter over the confirmed log, not a second algorithm:
`documentcore.MoveBlock` already carries `from`/`to` (RFC-002 §3), so
detecting a move between two revisions needs no geometry or heuristic,
only reading what the op log already recorded. `moves` is `[]`, never
`null`, when nothing moved.

**This endpoint does not compute the LCS text diff itself.** That runs
client-side, compiled to wasm (`services/textdiff` via `document-service/
cmd/diffwasm`) — `diff.html`'s own "token granularity switching (word ↔
character), recomputed live" needs interactive response to a toggle, the
same reasoning `graph.html`'s force layout/Voronoi views already have
for running in the browser (`ADR-012`). A client picks one block's text
out of `before`/`after`, tokenizes it (word or character — its own
choice), and calls the wasm bridge directly; this endpoint's only job is
handing over the two document states to pick from.

`from`/`to` out of `[0, len(steps))`, or malformed, is a `400`; a
malformed confirmed log (fails to even replay) is a `500`, same
partial-result contract as §5.

---

## 8. `GET /collab/stats` — this instance's queue depths

Plain HTTP, read-only, no page id: it is an *instance* fact,
not a resource. Backs `docs/ui-mockups/v2/index.html` § 16's
QUEUE DEPTH panel, which is the one panel on a benchmark
screen that is not measured locally.

Reached directly, never through `api-gateway`, the same
convention §§5–7 already follow — the gateway maps the REST
resource contracts (`pages.md`, `auth.md`), and none of these
is a resource.

```json
{
  "outbox_depth": 0,
  "outbox_oldest_seconds": 0,
  "ops": 2422,
  "pages": 140,
  "lag_seconds": 17553.75,
  "open_sessions": 0,
  "database_bytes": 12900031,
  "ops_per_hour": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
}
```

`open_sessions` is pages with a live rope in memory — **not**
people signed in (`auth-service`'s number, `auth.md`
`/admin/people`) and not editors connected. Three meanings of
"sessions"; every surface that shows one says which. Note also
that `Manager` never evicts (`CLAUDE.md`'s stated demo-scale
limitation), so this only grows until restart: a rising number on
an idle instance is that, not load.

`database_bytes` is **this service's own** database.
Database-per-service means an instance-wide "DB size" is a number
nobody owns, so each service reports its own or none at all.

`ops_per_hour` is the last 14 hours, oldest first, with quiet
hours present as **zero rather than absent** — a sparkline that
omits them draws a busy day where there was a gap. It is built
with `generate_series`, not `GROUP BY` alone, for exactly that
reason. Always an array, never `null`.

**Two numbers per queue, not one.** `outbox_depth` alone
cannot tell a healthy burst from a stopped poller: 400 events
draining in 200 ms and 3 events whose oldest has waited four
minutes are opposite conditions, and the count calls the
second one fine. `outbox_oldest_seconds` is what actually
distinguishes them.

`lag_seconds` is time since the newest accepted op. On an
idle instance it is large and perfectly healthy, so the
screen labels it rather than colouring it — a UI that turns
red because nobody is typing teaches its reader to ignore
red.

Four aggregate queries plus one in-memory count, no session
state mutated, safe to poll while people are editing — which is the only way it is
useful. It is deliberately **not** authenticated, for the
same reason and with the same caveat as §§5–7: these are
local debug surfaces, and the RS256 verification `ADR-011`
defers to a hardened gateway is not built in this repo yet.
It exposes counts, never content.


---

## 9. `GET /collab/audit` — the audit log's content half

Plain HTTP, read-only, no page id. Backs `docs/ui-mockups/v2/
index.html` § 18b AUDIT LOG, whose subtitle is the whole design:
**derived from the op log rather than written beside it.**

That is the only claim worth making about an audit log. A
separately-written audit table can drift from what actually
happened; a projection cannot. There is no code path that edits
a page without producing the row that says so, because the row
*is* the op.

```
GET /collab/audit?limit=120&class=destructive
```

```json
{
  "rows": [
    { "id": "01a04d86-…", "seq": 2422,
      "page_id": "01a04d85-…", "actor_id": "01a048f4-…",
      "actor_kind": "user", "kind": "block:InsertBlock",
      "class": "content", "undo_group": "e6c68b88-…",
      "created_at": "2026-08-29T12:37:07.826Z" }
  ],
  "counts": { "content": 2416, "destructive": 6 },
  "total": 2422,
  "kinds": [ { "kind": "text:DeleteText", "class": "destructive", "n": 6 } ]
}
```

**The payload is deliberately not selected.** An audit row says
who did what to which page; the text somebody typed is the
document's business, and an admin surface that quietly includes
it is a more invasive feature than the one anybody asked for.

`class` is `content` or `destructive`, and the classification
lives in Go, not in SQL — the database should not know what the
product considers destructive. Kinds arrive tier-prefixed
(`block:DeleteBlock`, `text:DeleteText` — RFC-002's two op
tiers) and are matched on the part after the colon; the tier is
not what makes something destructive. `MoveBlock` is **not**
destructive: it carries `from` as well as `to` and inverts
exactly (RFC-002 §3).

Anything unrecognised is `content`, on purpose: a new op kind
should appear in the log as ordinary rather than vanish from it
because nobody remembered to classify it.

`counts` and `total` are over the **whole log**, not the page
returned — a panel headed "by class" would answer a different
question otherwise.

**Auth events are not here.** They are `auth-service`'s
(`auth.md` `/admin/auth-events`), and the two are merged **in
the client**, by timestamp. That is deliberate: `DATA_MODEL.md`
forbids cross-schema joins, and the honest place for a join
across a service boundary is the caller that wanted both.

`limit` defaults to 100 and is capped at 500. Read-only over an
append-only table, so it is safe to call while people are
editing — and unauthenticated, with the same caveat as §§5–8.
