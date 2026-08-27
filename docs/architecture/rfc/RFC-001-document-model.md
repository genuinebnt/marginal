# RFC-001 — Document Model: Block Tree, Rich Text, and Rendering

**Status:** Accepted
**Date:** 2026-08-06
**Affects:** document-service, collaboration-service, editor core
**Related:** ADR-001 (scope), ADR-004 (Rust editor core), RFC-002 (operation model)

---

## 1. The Block Tree Is the AST

```
   markdown-source products              Marginal
   ─────────────────────────             ───────
   text  (source of truth)               block tree  (source of truth)
        │                                     │
        │ parse on EVERY render               │ NO parse on render
        ▼                                     ▼
   AST → HTML                            tree walk → DOM / HTML
```

Marginal parses **once, at input time**, and stores the tree. There is no text to re-parse and no ambiguity to re-resolve per keystroke. This is the architectural reason a block editor supports live collaborative editing while a markdown document does not.

The tree is an **abstract** syntax tree: it deliberately discards how content was authored. `**bold**`, `<b>bold</b>`, and Cmd+B all converge on `{"bold": true}`.

### Write the grammar before the code

This is the **logical grammar** — the resolved view a renderer, a projector, or a REST response sees. The CRDT machinery (RFC-002's rope, anchors, the op log) lives underneath and is fully resolved away by the time anything reaches this shape; `MarkRange`'s `Offset`s are logical offsets into an already-resolved `RunText`, not anchors.

```ebnf
Page      ::= PageId Document
Document  ::= Block*

Block     ::= BlockId BlockKind

BlockKind ::= Paragraph
            | Heading
            | List
            | Quote
            | Code
            | Toggle
            | Image
            | Divider

Paragraph ::= Spans
Heading   ::= Level Spans
Level     ::= 1 | 2 | 3
Divider   ::= ε

Quote     ::= Spans Block*
Toggle    ::= Spans Block*                  (* collapsed state is view, not model *)

List      ::= ListKind ListItem+
ListKind  ::= bulleted | numbered | todo
ListItem  ::= BlockId Checked? Spans ListChild*
ListChild ::= List | Paragraph
Checked   ::= true | false                  (* meaningful only when the enclosing List's ListKind = todo *)

Code      ::= Language? RawText             (* no Spans — code is unformatted *)
Language  ::= String
RawText   ::= String

Image     ::= FileId Caption?
Caption   ::= Spans

Spans     ::= Run*
Run       ::= RunText MarkRange*
RunText   ::= String

MarkRange ::= Mark Offset Offset            (* [start, end) into RunText — logical, resolved *)

Mark      ::= bold
            | italic
            | strike
            | code
            | link(Url)
            | pagelink(PageId)

BlockId   ::= UUID
PageId    ::= UUID
FileId    ::= UUID
Offset    ::= Integer
Url       ::= String
```

Rules that fall out of writing it down:

- **`Code` has no `Spans`** — code is never bold.
- **Toggle collapse is view state, not model state** — it must not be stored in the block or it becomes a collaborative edit when someone expands a toggle.
- **`Quote`, `Toggle`, `List`, and `ListItem` are the only container kinds** — every other `BlockKind` is a leaf and can never have children. `documentcore` enforces this as a real precondition (`NotAContainerError`), not a convention callers are trusted to follow.
- **`List` is two nesting levels, not one**: a `List` block's children (via the same containment mechanism every other container uses) are `ListItem` blocks; a `ListItem`'s own children are, per `ListChild`, restricted to `List` (a nested sub-list) or `Paragraph` (continuation text under that item) — `documentcore` checks this restriction specifically for `ListItem` parents, where every other container accepts any block kind as a child.
- **`Checked` lives on the `ListItem`, not the `List`** — each item tracks its own completion; it's meaningful only when the enclosing `List`'s `ListKind` is `todo`, the same "field meaningful only for one tag" shape `Level`/`Language` already have on `Heading`/`Code`.
- **`Image`'s `Caption` is the block's own `Content`** — reuses the exact mechanism `Quote`/`Toggle`'s own inline text already uses, not a second text-storage path.
- **`FileId` has no backing upload/asset pipeline in this repo yet** (`CLOUD_PORTABILITY.md`'s object-storage port is defined but unused by any Track 1 service) — an `Image` block holds an opaque `FileId` reference only; resolving it to bytes is out of scope until an upload flow exists. A stated gap, not a silent one.

### Persisted form is not the in-memory form

The block tree is stored as an **adjacency list**: rows with `parent_id`, an LTREE `path`, and a fractional `sort_key` — the same shape `docs.pages` already uses for pages-within-pages, applied one level deeper (blocks-within-a-page). The nested tree is materialised at read time. So "surface syntax" is rows; the AST is what you build from them.

`documentcore`'s own in-memory model does **not** need the materialised LTREE path — that exists to make "find all descendants of X" a cheap indexed Postgres query, and only `document-service`'s projection (`docs.blocks`) needs that query shape. In memory, a block only needs to know its immediate `Parent` (nil for a top-level block); `Page.Blocks` stays one flat, depth-first-ordered slice — a parent immediately followed by all its descendants, then the next top-level sibling — so a linear walk from the start already produces reading order, and `document-service`'s materialised `position` column keeps meaning exactly what it means today. Reparenting keeps this order invariant the same way `MoveBlock` already keeps sibling order invariant: it is what `Page.Apply` restores after every op, not something a reader has to reconstruct.

Two invariants nesting adds, both `Page.Apply` preconditions (checked uniformly with every other "this op's recorded prior state must match reality" check RFC-002 §1 already requires):

1. **No cycles.** A block can never become its own ancestor — the same rule `ReparentPage`'s `ErrCycle` already enforces for pages, checked here by walking the `Parent` chain from the proposed new parent up to the root and rejecting if the moved block appears in it (`documentcore` has no LTREE prefix check to lean on, so it's a bounded walk instead — bounded by the page's own block count).
2. **A container must be empty to delete or to change kind away from a container tag.** Cascading a delete through a whole subtree has no clean `Invert()` — reinserting a whole subtree atomically is a different, harder operation than reinserting one block — so this repo's scope stops at rejecting the op (`ContainerNotEmptyError`) rather than building subtree-delete's own undo semantics. A caller that wants to remove a non-empty container deletes its children first, one block at a time, each individually invertible.

---

## 2. Rich Text: Storage Format vs CRDT Working Format

**This is the section to get right; everything else here is comparatively mechanical.**

Marks are stored as a flat span array:

```json
{ "spans": [{ "text": "Hello ", "bold": true }, { "text": "world" }] }
```

### Why a span array cannot be the CRDT representation

```
  Start:   [ {"Hello world"} ]

  Bold "world":
           [ {"Hello "}, {"world", bold} ]                    1 span → 2

  Italicise "lo wo":
           [ {"Hel"}, {"lo ", italic},
             {"wo", bold+italic}, {"rld", bold} ]             2 spans → 4

  Now two users concurrently type at offset 8.
  Which span does each character join? The answer depends on
  array indices the other user's edit has already invalidated.
```

Array indices are not stable under concurrent edit. A CRDT needs identity that survives remote operations — a rope with **position-anchored marks**, as Yjs and Automerge do.

### Why a CRDT and not OT — the argument stated, including against

Google Docs uses **operational transformation with a central authoritative server**, not a CRDT,
and the case for that is stronger than it first appears:

> The server must see every operation anyway — for authorization, storage, and rendering. So
> centralising the transform costs almost nothing and **dramatically simplifies the data model.**

**Marginal runs server-authoritative too** — one owner per page, `ARCHITECTURE.md` §4 — which is
exactly the configuration where a CRDT's headline advantage is unused. The choice therefore needs a
reason, not an assumption.

**Three, in order of weight:**

1. **Offline editing.** A roadmap item, and the one thing OT genuinely cannot do well: OT requires
   the central authority to order operations, so a client that has been disconnected for a day has
   nothing to transform against. A CRDT merges on reconnect by construction
2. **OT correctness is famously hard.** Several published OT algorithms were later shown incorrect —
   the TP2 property in particular is subtle and has broken multiple implementations. CRDT
   convergence is easier to *establish* even where it is harder to *implement*, and this project
   asserts convergence as a proptest rather than a claim (`crates/document-core/tests/convergence.rs`)
3. ADR-002 — Rust depth wins ties, and a sequence CRDT is the richer target

**The cost being accepted, stated plainly.** OT transforms indices and retains **no tombstones**. A
CRDT must keep deleted items so anchors resolve (§9), so a document edited for years grows in a way
an OT document does not. That is a real and permanent tax, it is why tombstone GC is on the Phase 3
list rather than optional, and **it is part of why Google chose OT at their scale.**

> **This is a trade, not a victory.** If offline is ever cut from the roadmap, reason 1 disappears
> and the remaining case is reason 2 plus a learning goal — at which point the honest answer is that
> OT would have been the simpler engineering choice.

### Decision: two representations, one conversion site each way

```
  ┌────────────────────────────┐        ┌─────────────────────────────┐
  │     STORAGE / WIRE         │        │   CRDT WORKING FORMAT       │
  │  content JSONB, OpenAPI    │        │  collaboration-service +    │
  │                            │◄──────►│  the WASM editor core       │
  │  { "spans": [              │  ONE   │                             │
  │      {text, bold, italic,  │ SITE   │  rope:  Rope<char>          │
  │       strike, code, link}  │  EACH  │  marks: Vec<Mark {          │
  │  ] }                       │  WAY   │    kind, anchor_start,      │
  │                            │        │    anchor_end }             │
  │  · compact, queryable      │        │  · anchors move with text   │
  │  · readable in psql        │        │  · concurrent-edit safe     │
  └────────────────────────────┘        └─────────────────────────────┘
         ▲                                          │
         │  flush (batched)                         │
         └──────────────────────────────────────────┘
```

1. `spans` is the **storage and wire format** — Postgres, the OpenAPI contract, non-collaborative reads.
2. `rope + anchored marks` is the **working format**, live only for the duration of a session.
3. **Exactly one conversion site each way.** `spans → rope` on session open, `rope → spans` on flush. Never elsewhere; never a third representation.
4. While a session is live **the rope is authoritative**; the JSONB is a checkpoint, not the truth.

### What a keystroke actually costs

Stated explicitly, because the rest of §2 implies it without saying it and both wrong readings are
expensive: **the document is never re-parsed.** Note the precise claim — parsing *does* happen, on a
**bounded window**, and never on the whole document.

```
   ordinary keypress
     → Op::InsertText { block, at: Anchor, text }      the UI never mutates the tree (RFC-002 §1)
     → apply to the rope                                O(log n)
     → span normalisation                               merge adjacent identical mark sets
     → WAL · ack · broadcast                            RFC-002 §6
```

For an **ordinary** character that is the whole story — no lexer, no AST, no tree rebuild. Parsing
happens on two other inputs, and the difference between them is *scope*, not kind:

| Input | What runs | Scope |
|---|---|---|
| **Ordinary character** | Nothing. `InsertText` → rope | — |
| **Trigger character** — the space in `## `, a closing `` ` `` | An input rule: **bounded backward scan** from the cursor, emitting e.g. `SetBlockKind` + `DeleteText` | **One block**, bounded by the rule's lookbehind (§3) |
| **Paste / import** | The full pipeline in §4 — sanitise → tree → lower → normalise | The pasted fragment, once. The syntax tree is transient, discarded after lowering |
| **Render** | Nothing (§1) | — |

**Rows two and three are the same pipeline at different granularity** — paste runs all of it, an
input rule runs one line of it against one block. What neither ever does is re-parse the document.

**This is the difference from a markdown editor, and it is the reason live collaboration has no
merge-conflict UI.** A markdown document is parsed on every render, so two concurrent editors must
reconcile *text* — and, as the failure above shows, the parse result depends on indices the other
edit has already invalidated. A block tree is parsed **once at input time**, so two editors apply
ops to *nodes* instead.

> `ui-mockups/compiler.html` runs all six stages because it is showing **paste**. Reading it as the
> per-keystroke path is the misunderstanding this section exists to prevent.

### Invariant: span normalisation

After every mutation, adjacent spans with identical mark sets **must** merge.

```
  BAD  (fragments without bound as the user types):
    [ {"H",bold}, {"e",bold}, {"l",bold}, {"l",bold}, {"o",bold} ]

  GOOD (canonical):
    [ {"Hello",bold} ]
```

Canonical form also requires: empty spans removed · mark keys serialised in a fixed order so equality is comparable and diffs are stable · absent means false, never `"bold": false` · **`normalise(normalise(x)) == normalise(x)`**.

Without this, JSONB grows monotonically, every diff is noise, and equality assertions in tests become meaningless.

### Why there is no formatter, and why normalisation is not one

A document-wide *format* command — the `rustfmt`/`prettier` shape — is rejected. The reason is
specific to a CRDT and it is not "merge conflicts":

> **In a CRDT the cost of an operation is not its compute, it is how much identity it destroys.**

A formatter rewrites text, rewritten text carries new `ItemId`s, and every anchor into that region
then resolves to `Detached` (§9). Comments detach, marks die, diagnostic spans reset — **document
wide, at once.** `Detached { nearest_live }` exists to handle an occasional deletion gracefully,
not to absorb a global identity reset. Three consequences follow:

- **Convergence is not correctness.** Format on one replica while another types and the CRDT *will*
  converge — to text interleaved from a formatted and an unformatted version. Two replicas agreeing
  on garbage is still garbage. This is why collaborative code editors disable format-on-save or
  serialise it behind a lock
- The history diff (Phase 6) degrades to *everything changed*, and a tree diff cannot rescue it
  because the nodes are new
- A whole-document write enters the log for a change with no semantic content

**And the job a formatter does is already done by construction.** §1 makes the tree *abstract*:
there is no surface syntax to canonicalise, no stored `**bold**`, no indentation, no line width.

**Normalisation is the formatter, and it is safe because it inverts every dangerous property:**
incremental rather than whole-document, local rather than global, continuous rather than
on-command, and **idempotent** — a non-idempotent normaliser under format-on-save on two clients
ping-pongs forever.

| Shape | Verdict |
|---|---|
| Continuous span normalisation | **Mandatory** — the invariant above |
| **Scoped structural cleanup** — "tidy this section": fix a skipped heading level, drop an empty block, flatten a one-item list | **Allowed.** A bounded op batch through `can_apply`, invertible, touching *structure* rather than rewriting text (`ROADMAP.md` § The tree is the analyser's cheapest advantage) |
| **Formatting one `code` block** | **The one place a genuine formatter fits** — a single block whose contents are opaque text carrying no marks. Still destroys anchors inside that block, so it is user-initiated and never automatic |
| Document-wide reformat, or any delete-all + insert-all | **Rejected** |

The rule that generalises: **prefer transformations that preserve item identity.** A structural op
moving a node keeps every anchor inside it; a text rewrite producing byte-identical output still
destroys them all.

---

## 3. Input Rules — the only place a user types syntax

Runs **per keystroke, on one block**, never on the document.

| Trigger | Effect | Scope |
|---|---|---|
| `# ` … `### ` at block start | → `heading_1..3` | Block |
| `> ` | → `quote` | Block |
| `- ` / `* ` / `1. ` / `[] ` | → list variants | Block |
| ` ``` ` at block start | → `code` | Block |
| `**text**` / `_text_` / `` `text` `` / `~~text~~` | mark applied, delimiters removed | Inline |
| `[text](url)` | `link` mark | Inline |
| `[[` | page-link autocomplete | UI |
| `/` at block start | slash-command menu | UI |

**Constraints:**

- **A scanner, not a parser.** No grammar, no recursion — a bounded backward scan from the caret for a closing delimiter with a matching opener in the same span. Cheap enough for every keypress
- **Left-to-right, innermost first** — `**a _b_ c**` resolves the inner emphasis first
- **Escapable.** `\**not bold**` inserts literal asterisks; record the escape in the span so re-editing does not re-trigger
- **One undo step.** A rule firing must be a single undo unit: `Cmd+Z` restores `**bold**` as literal text, a second removes the typing
- **Never re-parse stored content.** Once `{"bold": true}` is stored the asterisks are gone. Input rules apply to *typing*, not to loading

---

## 4. Paste Normalisation — the hardest transducer, and a security boundary

Users paste constantly and the input is adversarial: Google Docs emits nested `<span style>` soup, Word emits `<o:p>` and conditional comments.

```
   clipboard
       ├── text/marginal-blocks ──▶ deserialize directly (own format, no parse)
       ├── text/html ──▶ sanitise ──▶ tree ──▶ lower to blocks ──▶ normalise
       │                (allowlist)   (walk)    (§1 AST)          (§2)
       └── text/plain ─▶ split on blank lines ─▶ paragraphs
```

1. **Prefer the richest format understood** — own MIME type, then HTML, then plain text
2. **Sanitise before parsing, on an allowlist.** Unknown tags are **unwrapped** (children kept), never dropped. `<script>`, `<style>`, event handlers, and `javascript:` URLs removed outright. **This is an XSS boundary** — treat it as security code and cover it in `/project:security-review`
3. **Map presentation to semantics.** `<b>`/`<strong>`/`font-weight:600..900` → `bold`; `<i>`/`<em>`/`font-style:italic` → `italic`. Ignore all other inline styles — do not import arbitrary CSS
4. **Report what was dropped.** Return `Vec<Diagnostic>` and surface a non-blocking notice. Silent loss of pasted content is the worst outcome
5. **Nesting maps to the block tree**, not to inline marks — `<ul><li>` becomes child blocks with LTREE paths and fractional keys
6. **Idempotence:** pasting Marginal's own copy output reproduces the source blocks exactly

---

## 5. Rendering — One Tree, Two Backends

```
                    block tree
                        │
            ┌───────────┴───────────┐
            ▼                       ▼
      DOM (editor)            HTML (Rust, askama)
      React over WASM         export / print view
      model state
```

**The risk is drift** — the export rendering subtly differently from the editor.

- **The block tree is the only contract.** No renderer consults data outside the tree it was handed
- **Derived content is computed once.** Backlink lists and table-of-contents are resolved in `document-service` and attached to the response, not recomputed per renderer
- **Golden-file tests share one fixture corpus.** A fixture added for one renderer guards the other

---

## 6. Testing: Laws, Not Examples

Example-based tests under-cover this code badly. Use `proptest`:

| Law | Statement |
|---|---|
| Normalisation idempotence | `normalise(normalise(x)) == normalise(x)` |
| Span coverage | Concatenated span texts equal the block's plain text, always |
| Paste round-trip | `paste(copy(blocks)) == normalise(blocks)` |
| Never-panic | For **any** byte string, paste and input rules return a result — never panic |
| Anchor stability | A mark's covered text is unchanged by an insert outside its range |
| CRDT convergence | Any interleaving of concurrent ops converges to the same rope |

The never-panic law is the one that finds real bugs — generate arbitrary bytes with `cargo-fuzz` and assert no panic.

---

## 7. Code Block Highlighting: One Highlighter

Code needs highlighting in the **editor** (live) and in **HTML export** (server). Two implementations means two theme definitions and guaranteed drift.

**`syntect` + `two-face`, compiled to `wasm32` and shared — but *not* inside `crates/document-core`.** Highlighting is rendering, not the document model, and `crates/document-core` is linked by every service (`lld/document-core.md` §2 § The dependency problem). It belongs in the wasm binding layer for the browser and in `publishing-service` for static HTML.

 One grammar set, one theme, identical output both sides. Fits the `wasm-bindgen` boundary ADR-004 already establishes. Trim grammars to a supported-language allowlist against the WASM bundle budget — `two-face` bundles `bat`'s full collection, which is large.

---

## 8. Open Questions

1. **Flush cadence** for `rope → spans` — every N ops, every T ms, or on idle? Interacts with the WAL and snapshot frequency (RFC-002)
2. ~~**Anchor representation**~~ — **resolved, see §9**
3. **Is `text/marginal-blocks` on the clipboard from day one**, or does cross-page paste go through HTML initially?

---

## 9. Anchor Representation — Resolved

**Decision: Yjs/Peritext-style item ids.** Every inserted run carries an identity assigned once
and never reused; an anchor is that identity plus a side. Rejected: offset-plus-origin.

This must be settled **before Phase 3 ships**, because anchors live inside op payloads
(`InsertText { at: Anchor }`, RFC-002 §1) and the op log is append-only. `encoding_version`
makes a later change survivable, not free: you would keep a decoder for the old representation
forever, and translating an old anchor would mean reconstructing the rope state it referred to.

### Why not offset-plus-origin

An `(base_version, offset)` anchor is only usable at a later version by transforming it through
the intervening ops. **That is operational transformation** — the model §1 rejected. Running OT
for positions inside a CRDT document means maintaining two position models, in a corner, without
the convergence tests the main path has. Two position models in one editor is how editors get
subtly and unreproducibly wrong.

Three further reasons, by weight:

1. **Comments get a detached state for free.** An item id stays valid after its text is deleted
   — it resolves to a tombstone, and "resolves to a tombstone" *is* `Detached`. Under
   offset-plus-origin the orphaned-comment policy gets decided implicitly inside a transform
   function instead of explicitly by a type. Comments are Phase 14 and marks are Phase 3, so
   this is the decision that has to be made eleven phases early.
2. **Long-lived anchors resolve without replay.** A six-month-old comment anchor is a lookup.
   Under offset-plus-origin it needs the op log from its base version forward.
3. **Published semantics exist.** Peritext and Y.Text both specify marks over a sequence CRDT
   (§ Resources). Read the paper rather than invent the design.

### The costs, and where they are already budgeted

| Cost | Where it is paid |
|---|---|
| Tombstones must be retained, so the id space only grows | Tombstone GC sweep — ROADMAP § stdlib (`retain`/`drain`), Phase 3 |
| Anchor → byte offset needs an order-statistic lookup | The rope is already a balanced tree; subtree sizes *are* that index |
| An anchor is wider than an integer, and ops carry many | Varint/LEB128 op encoding — ROADMAP § Memory & layout, Phase 3 |

### One type, two storage forms

This is the non-obvious consequence and it follows from **lifetime**, not from convenience:

- **Marks persist as byte offsets** in `blocks.content` JSONB (§2). They are regenerable from
  the flushed text, compact, and queryable.
- **Comment anchors persist as item ids.** A comment must survive the session that created it,
  so it cannot be stored as an offset into text that a later session re-anchors. Resolving one
  to a screen position is a session-time operation against the live rope — which is correct,
  since comments only need positions while the page is open.

### The type, and the laws it must satisfy

```rust
// crates/domain/src/anchor.rs — zero deps, wasm32-clean.
// Shared by marks (3), diagnostic spans (4), and comments (14). RFC-003 §Anchoring
// says build anchoring once; this is the type that makes that true.
// Fields stay private: nothing may do arithmetic on an anchor, because integer
// offsets are precisely what invite the bug this design exists to prevent.

pub struct ItemId  { /* replica + counter — the Lamport pair every sequence CRDT uses */ }
pub enum   Bias    { Before, After }   // does text typed at the boundary land inside?
pub struct Anchor      { /* ItemId + Bias */ }
pub struct AnchorRange { /* start, end */ }

/// Three-state on purpose. `Detached` is a NORMAL outcome — the anchored text was
/// deleted — which is exactly why this is not `Option<usize>`.
pub enum Resolved {
    At(usize),                        // byte offset in the current rope
    Detached { nearest_live: usize },  // render the comment beside where it used to be
    Unknown,                          // id absent from this document: a bad client, not a deletion
}
```

Write these as failing tests before any of it has a body:

`anchor_survives_insert_before` · `anchor_survives_insert_after` ·
`anchor_detaches_when_its_item_is_deleted` · `detached_anchor_reports_nearest_live_offset` ·
`bias_before_excludes_text_typed_at_start` · `bias_after_includes_text_typed_at_end` ·
`resolve_stable_under_concurrent_remote_insert` (proptest) · `two_anchors_never_cross` (proptest)

---

## 10. Target Grammar (v3) — Extended Block Kinds

**Status: aspirational.** This section documents where the block-kind set is
headed, not what `documentcore` implements today — §1's grammar (currently:
`Paragraph`/`Heading`/`Quote`/`Code`/`Divider`/`List`/`ListItem`/`Toggle`/
`Image`) is the actual, current contract. Sections below name each kind's
implementation status explicitly; nothing here is built until §1 (or a
future §1 revision) says so.

**Provenance.** Adapted from `genuine-folio`'s own `:::directive` markdown
family (`backend/src/infra/render.rs`, `frontend/lib/directives.ts` — a
different, single-author static-site repo, not part of Marginal) — but not
a port. That system's directive bodies are flat, single-string fields
(`desc: string`, `body: string`); every genuine change here is upgrading
each of those to `Block*`, the same nesting §1 already gives `Quote`/
`Toggle`/`List`/`ListItem` — a directive's body becomes real, independently
collaboratively-editable content instead of one opaque text blob owned by
whoever's cursor is in it last. The directive→block-kind mapping and the
raw-markdown-directive syntax itself are `genuine-folio`'s own input
format, not part of this grammar — Marginal has no markdown-directive input
surface; a client constructs `Op`s directly (RFC-002), so only the
*resolved shape* below is relevant here.

### 10.1 Design principles

1. **Every block kind is usable everywhere.** Any `BlockKind` may appear at
   the top level of a `Document` or inside any container's `Block*` — a
   callout inside a timeline entry inside a column inside a toggle is
   legal. No page-type gating, no "only nests one level" rule. A renderer
   may *style* a block differently by context but must not reject it.
2. **Structured content is `Block*`, not flat text.** Wherever the source
   directive family treated an item body as one opaque string, the block
   kind below gives it `Block*` instead — RFC-002 §2's block-granular ops
   (and, inside it, §2's character-granular ones) apply to that content
   exactly as they do to a top-level page, not a second content model.
3. **Every `Block` and every repeatable sub-item carries a `BlockId`.**
   Timeline entries, cards, tabs, diagram steps, decisions, table rows,
   columns, list items — all of them, the same `BlockId`-per-node rule §1
   already establishes for the currently-implemented kinds. This is what
   makes concurrent insert/delete/reorder/edit of *different* items merge
   cleanly (RFC-002 §1) — only edits to the *same* field of the *same*
   item conflict.
4. **View state is never in the model.** Collapsed/expanded, active tab,
   current diagram step, selection, hover — all client-only, the same
   rule §1 already states for `Toggle`'s own collapse state. The grammar
   carries at most a *seed* hint (`Open?`) the client consumes once and
   then owns.
5. **Dynamic blocks store parameters, not content.** `TableOfContents`,
   `FeaturedArticles`, `FeaturedProjects`, `PortfolioProjects` hold only
   query/label arguments; their body is a live projection computed at
   render time, never persisted into the document tree. **These need a
   query/aggregation engine across pages that does not exist in this repo
   and is explicitly out of scope (`CLAUDE.md`'s "databases/tables/views/
   relations/rollups... a second ownership tier, not a feature") — an ADR
   is required before any of these four is implemented, not just this
   RFC entry.**

### CRDT mapping (informative)

| Grammar construct | CRDT primitive |
|---|---|
| `Block*`, `X+` ordered children | replicated sequence of `BlockId` (RFC-002 §2's block-granular ISA) |
| `Spans` / `Run` / `MarkRange` | inline text CRDT (§2's rope + Peritext-style marks) |
| `Level`, `CalloutTone`, `Ratio`, `Checked`, `Language`, every boolean flag | last-writer-wins register (`SetBlockKind`/`SetBlockContent`) |
| `RawText`, `SvgSource`, `MermaidSource`, `LatexSource` | text-sequence CRDT *or* LWW blob (implementation's choice) |
| `BlockId`, `PageId`, `FileId` | assigned once at creation, never reused |

`MarkRange` offsets are logical positions within the merged `RunText`,
resolved after the text CRDT has converged — identical to §2's existing
`Mark{Start,End}`.

### 10.2 Grammar (EBNF)

```ebnf
(* ========================================================= *)
(*     TARGET DOCUMENT GRAMMAR (v3) — aspirational, §10       *)
(*   CRDT machinery is underneath and resolved before this    *)
(*   logical representation is consumed by the renderer.      *)
(*   Every BlockKind may nest inside any container.            *)
(* ========================================================= *)

Page      ::= PageId Document
Document  ::= Block*

Block     ::= BlockId BlockKind

BlockKind ::= (* -- prose (§1, implemented) --------------------------- *)
              Paragraph
            | Heading
            | List
            | Divider
              (* -- containers (Quote/Toggle: §1, implemented) ------- *)
            | Quote
            | Toggle
            | Callout
            | Aside
            | ColumnList
              (* -- code, math, diagrams (Code: §1, implemented) ---- *)
            | Code
            | Equation
            | Mermaid
            | Diagram
            | DiagramSequence
              (* -- media & external refs (Image: §1, implemented) - *)
            | Image
            | Video
            | File
            | Bookmark
            | Embed
              (* -- tables (needs an ADR — see 10.1 point 5's sibling
                   concern: fixed row/cell arity across concurrent
                   edits is its own hard problem, not yet designed) - *)
            | Table
            | CommTable
              (* -- structured collections -------------------------- *)
            | Timeline
            | IconCards
            | ServiceCards
            | Grid
            | Signals
            | SignalList
            | Accordion
            | Tabs
            | Stack
            | MetaPills
            | FooterLinks
            | UsesSection
              (* -- synced / generated ------------------------------ *)
            | SyncedBlock
            | TableOfContents          (* needs an ADR — 10.1 point 5 *)
              (* -- page composition (usable anywhere) -------------- *)
            | Hero
            | Eyebrow
            | SectionLabel
            | Rainbow
            | HomeDivider
            | NowStatus
            | NowProgress
            | NowChips
            | NowReading
            | FeaturedArticles         (* needs an ADR — 10.1 point 5 *)
            | FeaturedProjects         (* needs an ADR — 10.1 point 5 *)
            | PortfolioProjects        (* needs an ADR — 10.1 point 5 *)


(* ========================================================= *)
(*                       CONTAINERS                          *)
(* ========================================================= *)

Callout     ::= Icon? CalloutTone? Spans Block*
Aside       ::= Emoji Spans Block*
                (* the "character aside" — emoji speaker + note *)
Icon        ::= Emoji | FileId
CalloutTone ::= note | info | tip | warn | danger | success
                (* genuine-folio's own six semantic tones — "warn" is
                   the historical default. The source system also has a
                   Notion-palette-shaped superset (gray/brown/orange/…);
                   not adopted here, since nothing in Marginal imports or
                   exports Notion content — add it if that ever changes,
                   not speculatively now. *)


(* ========================================================= *)
(*                      SYNCED BLOCK                         *)
(* ========================================================= *)

SyncedBlock ::= Original | Reference
Original    ::= Block*
Reference   ::= BlockId        (* resolves to the corresponding Original *)


(* ========================================================= *)
(*                  CODE / MATH / DIAGRAMS                   *)
(* ========================================================= *)

Equation        ::= LatexSource        (* target of a math block *)
LatexSource     ::= String

Mermaid         ::= MermaidSource
MermaidSource   ::= String

Diagram         ::= Wide? SvgSource    (* inline SVG, theme-adaptive *)
SvgSource       ::= String

DiagramSequence ::= Wide? DiagramStep+
DiagramStep     ::= BlockId Caption SvgSource
Caption         ::= String
Wide            ::= true | false       (* lifts the reading-width cap *)


(* ========================================================= *)
(*                          MEDIA                             *)
(* ========================================================= *)

Video     ::= FileId FileName? Caption?
File      ::= FileId FileName Caption?
FileName  ::= String


(* ========================================================= *)
(*                  EXTERNAL REFERENCES                       *)
(* ========================================================= *)

Bookmark      ::= Url BookmarkMeta?
BookmarkMeta  ::= Title? Description? PreviewImage?
Title         ::= String
Description   ::= String
PreviewImage  ::= FileId | Url

Embed         ::= Url EmbedProvider?
EmbedProvider ::= String               (* dispatch key: youtube, figma, … *)


(* ========================================================= *)
(*                          LAYOUT                             *)
(* ========================================================= *)

ColumnList ::= Column+
Column     ::= BlockId Ratio? Block*
Ratio      ::= Float                   (* sibling ratios normally sum to 1.0 *)


(* ========================================================= *)
(*                    TABLE OF CONTENTS                        *)
(* ========================================================= *)

TableOfContents ::= ε                  (* live view generated from headings *)


(* ========================================================= *)
(*                          TABLES                             *)
(* ========================================================= *)

Table           ::= TableStyle? TableWidth HasColumnHeader? HasRowHeader? TableRow+
TableStyle      ::= plain | matrix
TableWidth      ::= Integer
TableRow        ::= BlockId Cell{TableWidth}   (* row arity == TableWidth *)
Cell            ::= Spans
HasColumnHeader ::= true | false
HasRowHeader    ::= true | false

CommTable       ::= CommRow+           (* service-to-service comms table *)
CommRow         ::= BlockId Call Protocol ProtocolClass Pattern Failure
Call            ::= Spans
Protocol        ::= String
ProtocolClass   ::= String
Pattern         ::= Spans
Failure         ::= Spans


(* ========================================================= *)
(*                 STRUCTURED COLLECTIONS                     *)
(* ========================================================= *)

Timeline      ::= TimelineItem+
TimelineItem  ::= BlockId Term Title Block* DirectiveIcon? Current?
Term          ::= Spans
Title         ::= Spans
Current       ::= true | false

IconCards     ::= Accent? IconCardItem+
IconCardItem  ::= BlockId DirectiveIcon Title Block*
Accent        ::= true | false

ServiceCards  ::= ServiceCard+
ServiceCard   ::= BlockId CardColor Name Owns? Tag* Block*
Name          ::= Spans
Owns          ::= Spans
Tag           ::= String

Grid          ::= GridItem+
GridItem      ::= BlockId Title Block*

Signals       ::= TitledNote+
TitledNote    ::= BlockId Spans Block*

SignalList    ::= SignalTag+
SignalTag     ::= BlockId TagLabel TagClass Block*
TagLabel      ::= String
TagClass      ::= String

Accordion         ::= AccordionSection+
AccordionSection  ::= BlockId Ordinal Title Subtitle? PhaseColor? Open? Decision+
Ordinal           ::= Integer
Subtitle          ::= Spans
Decision          ::= BlockId Spans Block*
PhaseColor        ::= default | blue | purple | pink | green | warn
Open              ::= true | false

Tabs        ::= Tab+
Tab         ::= BlockId Label ConceptCard+
ConceptCard ::= BlockId ConceptDomain Name Block*
Label       ::= Spans

Stack       ::= StackEntry+
StackEntry  ::= BlockId Name Role?
Role        ::= Spans

MetaPills   ::= MetaPill+
MetaPill    ::= BlockId PillLabel PillValue?
PillLabel   ::= Spans
PillValue   ::= Spans

FooterLinks ::= FooterLink+
FooterLink  ::= BlockId Label Url

UsesSection    ::= AccentColor? SectionHeading UsesItem+
SectionHeading ::= Spans
UsesItem       ::= BlockId Name Description Tag?


(* ========================================================= *)
(*        PAGE COMPOSITION  (no gating — usable anywhere)      *)
(* ========================================================= *)

Hero        ::= Eyebrow? Title Lead? HeroPill*
Lead        ::= Spans
HeroPill    ::= BlockId PillLabel PillValue?

Eyebrow      ::= Spans
SectionLabel ::= Spans

Rainbow     ::= BandColor*
BandColor   ::= CssColor

HomeDivider ::= ε

NowStatus      ::= NowStatusCard+
NowStatusCard  ::= BlockId StatusLabel StatusValue StatusSub? StatusTone?
StatusLabel    ::= Spans
StatusValue    ::= Spans
StatusSub      ::= Spans
StatusTone     ::= default | acc | purple | warn

NowProgress    ::= ProgressTitle? NowProgressRow+
ProgressTitle  ::= Spans
NowProgressRow ::= BlockId ProgressLabel Percent ProgressColor?
ProgressLabel  ::= Spans
Percent        ::= Integer            (* clamped 0..=100 *)
ProgressColor  ::= acc | blue | purple | warn

NowChips  ::= NowChip+
NowChip   ::= BlockId Accent? Spans

NowReading    ::= NowReadingRow+
NowReadingRow ::= BlockId SpineColor BookTitle Author Progress
SpineColor    ::= CssColor
BookTitle     ::= Spans
Author        ::= Spans
Progress      ::= Spans

FeaturedArticles  ::= SlotLabel?
FeaturedProjects  ::= SlotLabel?
PortfolioProjects ::= SlotLabel?
SlotLabel  ::= String


(* ========================================================= *)
(*                    VALUE-OBJECT VOCAB                       *)
(* ========================================================= *)

DirectiveIcon ::= home | work | layers | book | shield | terminal
                | flag | star | chip | network | clock

CardColor     ::= blue | teal | green | purple | amber | pink

ConceptDomain ::= dist | sysdes | micro | rust | perf | dsa
```

### 10.3 Semantic invariants (not expressible in the grammar)

- **Table row arity.** Every `TableRow` has exactly `TableWidth` cells.
  Widening or narrowing is a structural edit that must add/remove one
  `Cell` (its own `BlockId`) in every row atomically — needs its own
  design (this is exactly the "needs an ADR" gap 10.1 flags for tables).
- **Column ratios.** Sibling `Column.Ratio` values are advisory and
  normally sum to `1.0`; a renderer normalises rather than rejecting.
- **`Current` / `Open` uniqueness.** At most one `TimelineItem` per
  `Timeline` should carry `Current = true`; if none does, the renderer
  highlights the first. `Open` is a one-shot seed the client consumes on
  first mount, same rule as `Toggle`'s own collapse state (§1).
- **`Reference` resolution.** A `SyncedBlock` `Reference` must resolve to
  an `Original` on some page; a dangling reference renders as an inert
  placeholder, never an error — the same "dangling is data, not a fault"
  choice `docs.page_links.target_page = NULL` already makes for `[[wikilinks]]`
  (`DATA_MODEL.md`).
- **Dynamic blocks are read-only projections** — never written back into
  the document tree (10.1 point 5).
- **`Percent`** is clamped to `0..=100` on construction (parse, don't
  validate — documentcore's own `NewHeading`/`NewList` convention).
- **Icon vocabulary is closed.** `DirectiveIcon` values outside the listed
  set are a diagnostic, not a passthrough — the field is plain text and
  must not become raw markup.

### 10.4 Implementation status

Not an exhaustive per-kind ledger — just where the line is drawn today and
why, so a future session doesn't have to re-derive it:

- **Zero-new-mechanism containers** (`Callout`, `Aside`): the exact same
  shape `Quote`/`Toggle` already have — `BlockTag.IsContainer()` extended
  by two entries, a `CalloutTone`/`Icon`/`Emoji` field each following the
  existing `Level`/`Language`/`ListKindOf` "meaningful only for one tag"
  pattern. The natural next slice.
- **Structured collections shaped like `List`/`ListItem`** (`Timeline`,
  `Grid`, `IconCards`, `Signals`, `Tabs`, `Accordion`, `ServiceCards`,
  `SignalList`, `Stack`, `MetaPills`, `FooterLinks`, `UsesSection`): each
  is `BlockId <fixed inline fields> Block*` — the same shape `ListItem`
  already has, just with different fixed fields per kind. Mechanically
  straightforward once the container mechanism exists, but each is its
  own `BlockKind` with its own fields — real, if repetitive, work; not
  attempted in one pass.
- **New machinery needed, each its own design pass**: `SyncedBlock`
  (cross-block reference resolution nothing in this repo does yet),
  `ColumnList` (a ratio/layout concern layered on containment),
  `Diagram`/`DiagramSequence`/`Mermaid`/`Equation` (raw-blob storage,
  no rich-text marks inside — closer to `Code` than to a text block).
- **Needs an ADR before any implementation** (`CLAUDE.md`'s "still out"
  gate): `Table`/`CommTable` (fixed row/cell arity under concurrent
  edits), `TableOfContents`/`FeaturedArticles`/`FeaturedProjects`/
  `PortfolioProjects` (a cross-page query/aggregation engine this repo's
  architecture has no owner for — `DATA_MODEL.md`'s own "no databases/
  rollups" boundary, `collab.ops.page_id NOT NULL` and one page per
  collaboration-service instance).
- **Site-specific page-composition kinds** (`Hero`, `Eyebrow`, `Rainbow`,
  `HomeDivider`, `NowStatus`, `NowProgress`, `NowChips`, `NowReading`):
  documented here because they're part of the source grammar this
  section adapts, not because they're recommended for Marginal — these
  are `genuine-folio`'s own personal-homepage components, not general
  collaborative-notebook content. Revisit only if an actual Marginal use
  case calls for them.

---

## Resources

| Resource | For |
|---|---|
| [Peritext (Ink & Switch)](https://www.inkandswitch.com/peritext/) | **Read before §2.** The definitive treatment of rich-text CRDT marks under concurrency |
| [Yjs Y.Text](https://docs.yjs.dev/api/shared-types/y.text) | Position-anchored marks over a sequence CRDT |
| [ProseMirror document model](https://prosemirror.net/docs/guide/#doc) | A well-designed block/inline model to *study* — the library is prohibited (ADR-004) |
| [Why ContentEditable Is Terrible](https://medium.engineering/why-contenteditable-is-terrible-122d8a40e480) | Read before §3 and §4 |
| [proptest book](https://proptest-rs.github.io/proptest/) | §6's laws |
