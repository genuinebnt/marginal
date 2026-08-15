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

```ebnf
Document  ::= Block*
Block     ::= Paragraph | Heading | List | Quote | Code | Toggle | Image | Divider
Paragraph ::= Spans
Heading    ::= Level Spans                  (* Level = 1 | 2 | 3 *)
List       ::= ListKind ListItem+           (* ListKind = bulleted | numbered | todo *)
ListItem  ::= Spans Block*                  (* nesting via children *)
Quote     ::= Spans Block*
Toggle    ::= Spans Block*                  (* collapsed state is view, not model *)
Code      ::= Language? RawText             (* no Spans — code is unformatted *)
Image     ::= FileId Caption?
Spans     ::= Span*
Span      ::= Text Mark*
Mark      ::= bold | italic | strike | code | link(Url) | pagelink(PageId)
```

Two rules that fall out of writing it down: **`Code` has no `Spans`** (code is never bold), and **toggle collapse is view state, not model state** — it must not be stored in the block or it becomes a collaborative edit when someone expands a toggle.

### Persisted form is not the in-memory form

The block tree is stored as an **adjacency list**: rows with `parent_id`, an LTREE `path`, and a fractional `sort_key`. The nested tree is materialised at read time. So "surface syntax" is rows; the AST is what you build from them.

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

## Resources

| Resource | For |
|---|---|
| [Peritext (Ink & Switch)](https://www.inkandswitch.com/peritext/) | **Read before §2.** The definitive treatment of rich-text CRDT marks under concurrency |
| [Yjs Y.Text](https://docs.yjs.dev/api/shared-types/y.text) | Position-anchored marks over a sequence CRDT |
| [ProseMirror document model](https://prosemirror.net/docs/guide/#doc) | A well-designed block/inline model to *study* — the library is prohibited (ADR-004) |
| [Why ContentEditable Is Terrible](https://medium.engineering/why-contenteditable-is-terrible-122d8a40e480) | Read before §3 and §4 |
| [proptest book](https://proptest-rs.github.io/proptest/) | §6's laws |
