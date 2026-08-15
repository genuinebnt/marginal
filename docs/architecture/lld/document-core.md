# LLD — `crates/document-core`

**Owns:** the document model itself — the parser, the block tree, the rope, anchors, marks, and the op ISA. **The most important crate in the project.**
**Transport:** none. It is a library.
**Depends on:** `crates/domain` only. **No tokio, no sqlx, no filesystem, no network** — `wasm32`-clean by rule (`CLAUDE.md` § Crate Layout), enforced in CI.
**Related:** `RFC-001` (document model, §9 anchors) · `RFC-002` (op ISA) · `lld/collaboration-service.md` (its largest consumer) · `ui-mockups/compiler.html` (the pipeline, running)

**Not a service, and that is why this document exists.** Every other LLD covers one deployable.
This covers one *crate* with **three consumers and four phases**, which is exactly the shape that
falls between documents and ends up unspecified.

---

## 1. Scope — who links this, and what each one needs

| Consumer | Needs | Phase |
|---|---|---|
| **The browser** (`wasm32`) | **Everything** — and it is the *only* consumer of the front end, which is why the front end lives in `crates/editor-wasm` rather than here | 1, 3, 16 |
| **`collaboration-service`** (native) | `tree`, `ops`, `anchor`, `rope`, `marks`. **Not the front end** | 3 |
| **`document-service`** (native) | `tree` and `ops::apply`, to materialise `blocks` by replaying op events (ADR-003). **Not anchors or the rope** — marks persist as byte offsets in JSONB | 1 |

**The parser has exactly one runtime: the browser.** Paste comes off the clipboard, input rules must
fire without a round trip, and file import can read the file client-side too. **No server-side code
path parses markdown**, and if one ever appears that is a design change, not an optimisation.

> **`crates/document-core` is where Phase 1's editor half lives.** `lld/document-service.md` covers the backend
> of Phase 1 and deliberately contains no parser. If you build only from that document you will ship
> a service with no editor — this is the other half.

---

## 2. Module map

**The editor core spans three crates, split by who links them.**

```
crates/document-core/     ── the model · linked by everything ──
├── page.rs           Page, PageId, apply                          ✔ exists
├── block.rs          Block, BlockId, BlockKind                    ✔ exists
├── operation.rs      Op, apply, invert          RFC-002 §2        ✔ exists
├── history.rs        undo/redo stacks, atomic on failure          ✔ exists
├── inline.rs         flat text + marks over BYTE ranges  RFC-001 §2   ← NEXT
├── tree.rs           block tree ↔ `blocks.content` JSONB       all three consumers
├── anchor.rs         Anchor, ItemId, Bias, Resolved  RFC-001 §9  browser + collab
└── rope/             mod.rs · node.rs · summary.rs   Phase 3     browser + collab

crates/document-parser/   ── what the user types · linked by editor-wasm only ──
├── lex/
│   ├── block.rs      line classification → tokens with byte spans
│   ├── inline.rs     `**` `*` `_` backtick `[[…]]` `[…](…)`, nested
│   └── scan.rs       the BOUNDED backward scanner — a different shape
├── parse.rs          recursive descent over block tokens → Ast
├── ast.rs            Ast — TRANSIENT. Lives microseconds, never stored
├── lower.rs          Ast → block tree + marks; the one place marks are produced
├── normalise.rs      mark coalescing; must be idempotent        RFC-001 §2
└── sanitise.rs       HTML paste allowlist — AN XSS BOUNDARY     RFC-001 §4

crates/editor-wasm/       ── the crossing · browser only ──
├── lib.rs            every #[wasm_bindgen] export, one reviewable surface
└── highlight.rs      syntect + two-face — rendering, not modelling
```

> **Why three and not two.** The parser could live inside `editor-wasm`, since the browser is its
> only consumer. It does not, because the dependency concern — *no backend service should link an
> XSS-boundary HTML parser or `syntect`* — is satisfied just as well by a separate crate that only
> `editor-wasm` depends on, and a plain library fuzzes, tests and reviews better than a module
> inside a cdylib. `sanitise.rs` is the file this matters most for.

**Phase 1 builds the top half. Phase 3 builds the bottom half.** `tree.rs`, `anchor.rs`, and
`ops.rs` are needed by both, which is why the anchor decision had to be made before either
(RFC-001 §9).

`rope/`, `anchor.rs`, `marks.rs`, and `ops.rs` are specified in
[`collaboration-service.md`](collaboration-service.md) §3–§5. **That is the authority for them;**
this document does not repeat it. Sections 3–5 below cover the front end, which nothing else specs.

### Why the split falls exactly there

**The line is drawn by who links the crate, not by what the code is about.** All three consumers
need the model; only the browser needs the parser; only the browser needs the crossing.

The dependency consequence is the visible proof. `sanitise.rs` needs an HTML parser and highlighting
needs `syntect` + `two-face`; with either in `crates/document-core`, **two backend services would
link an XSS-boundary HTML parser they never invoke**, plus the largest dependency in the project.
Not fatal — compile time and review surface — but paid for nothing.

A `parse` feature gate was the first answer here and it was a workaround: it hid a placement problem
rather than fixing it. Separate crates remove the gate, the conditional compilation, and the
question.

| | |
|---|---|
| **`syntect` + `two-face`** | `crates/editor-wasm` only. Highlighting is *rendering*, not modelling — `document-core`'s job ends at producing a block tree (RFC-001 §5) |
| **Still testable natively** | `crates/editor-wasm` is `crate-type = ["cdylib", "rlib"]`, so `cargo test` needs no wasm toolchain. `document-parser` is a plain lib and needs nothing |
| **`sanitise` is the reason for the seam** | It is the XSS boundary and the `cargo-fuzz` target (TASKS.md D-08). A fuzz target inside a cdylib is harder to run than one in a plain library — that alone earns the crate |
| **If a server-side consumer appears** | File import, say — it depends on `document-parser` directly. No move required, which is the point of having split it |

---

## 3. `lex/` — three lexers, and they are not the same shape

### `block.rs` — line classification

Each line becomes one token carrying its **byte** span in the source. Not a character span:
marks are byte ranges and the two diverge on the first non-ASCII input.

```rust
pub enum BlockToken {
    Hash { level: u8, text: Span },      // # .. ###
    Bullet { indent: usize, text: Span },
    Ordered { indent: usize, start: u32, text: Span },
    Quote { indent: usize, text: Span },
    FenceOpen { lang: Option<Span> },
    FenceClose,
    CodeLine(Span),
    Divider,
    Blank,
    Text(Span),
}
```

**An unterminated fence is not a token.** It is a property of the `Code` node the parser builds.
Emitting an error sentinel the parser must remember to consume is how a front end hangs — see §7.

### `inline.rs` — text plus marks, never nested nodes

Returns **plain text and marks over byte ranges into that text** — the single most important
contract in the crate (RFC-001 §2). A rope holds text; marks are intervals into it.

```rust
pub struct Inline { pub text: String, pub marks: Vec<RawMark> }
pub struct RawMark { pub kind: MarkKind, pub start: usize, pub end: usize, pub attr: Option<String> }
```

Nesting is handled by recursion with an offset, not by a node tree. `` `code` `` does **not**
recurse — `**` inside it survives verbatim, which is why `Code` has no `Spans` in RFC-001 §1's
grammar.

### `scan.rs` — the input-rule scanner, and it is a different problem

Not a stream lexer. A **bounded backward scan from the cursor**, run per keystroke on one block,
never on the document (RFC-001 §3).

```rust
/// Look back at most `LOOKBEHIND` bytes from `cursor` within one block.
/// Returns the rule that fired and the ops it implies, or None.
pub fn scan_input_rule(text: &str, cursor: usize) -> Option<InputRule>;
```

**The bound is the contract.** An unbounded scan makes every keystroke O(block length) and it is
the difference between an editor and a toy. State the constant and test that it holds.

---

## 4. `parse.rs` + `ast.rs` — recursive descent, and the tree is transient

Recursive descent over `BlockToken`s. Nested lists come from indentation, so `list_at(indent)`
recurses; quote runs group; fences consume to their terminator.

```rust
pub enum Ast { Doc(Vec<Ast>), Heading { level: u8, inline: Inline }, Para(Inline),
               List { ordered: bool, items: Vec<Item> }, Quote(Vec<Ast>),
               Code { lang: Option<String>, code: String, closed: bool }, Divider }
```

**The AST is transient and nothing stores it.** RFC-001 §1 — the *block tree* is the AST that
persists; this one exists for microseconds during a paste and is discarded after `lower`. Unlike a
batch compiler, no later stage holds a reference.

**The parser must never reject its input.** The document is mid-keystroke and therefore malformed
most of the time. Every malformed construct produces a node — an unterminated fence is a `Code` with
`closed: false`, an orphaned `]]` is text. Read matklad's
[resilient LL parsing tutorial](https://matklad.github.io/2023/05/21/resilient-ll-parsing-tutorial.html)
before writing this; it is the discipline batch-compiler courses do not teach.

---

## 5. `lower.rs`, `normalise.rs`, `sanitise.rs`

| Module | Contract |
|---|---|
| `lower.rs` | `Ast` → blocks with `BlockKind`, `parent`, fractional `sort_key`, and marks as **byte ranges**. The one place spans are produced |
| `normalise.rs` | Adjacent identical mark sets merge; empty spans removed; mark keys in fixed order; absent means false. **`normalise(normalise(x)) == normalise(x)`** |
| `sanitise.rs` | HTML paste on an **allowlist**. Unknown tags are *unwrapped*, children kept — never dropped. `<script>`, `<style>`, event handlers, `javascript:` removed. Returns `Vec<Diagnostic>` for what was dropped, because silent loss is the worst outcome |

**`sanitise.rs` is security code** and `CLAUDE.md`'s skill workflow puts it under
`/project:security-review`. Fuzz it (`cargo-fuzz`) from the first commit — it is the only module in
this crate that processes attacker-supplied input.

---

## 6. Algorithms — named, not written

| Algorithm | Invariant that must hold | Reference |
|---|---|---|
| **Bounded backward scan** | Cost is bounded by the rule's lookbehind, never by block or document length | RFC-001 §3 |
| **Inline parse → text + marks** | Marks are **byte** offsets into the stripped text; nesting round-trips; `code` does not recurse | RFC-001 §2 |
| **Recursive descent** | **Total** — every input produces a tree; the parser always advances, so it can never hang | matklad, resilient LL |
| **Lowering** | `sort_key`s are strictly increasing among siblings; block ids are fresh; marks land within their block's text | RFC-001 §1 |
| **Span normalisation** | Idempotent, and equality-stable so diffs and test assertions mean something | RFC-001 §2 |
| **Paste sanitisation** | No script executes; unknown tags unwrap rather than drop; **pasting Marginal's own copy output reproduces the source blocks exactly** | RFC-001 §4 |
| **The projection check** | `interpret(emit_ops(lower(ast))) == lower(ast)` — replay reproduces lowering | `compiler.html`, `DATA_MODEL.md` §1 |

**The last row is the one to build early.** `ui-mockups/compiler.html` already runs it and reports
HOLDS or FAILS from an actual comparison; it is the executable form of *the op log is the source of
truth and blocks are a projection replay must reproduce.*

---

## 7. Test map

```
crates/document-core/tests/
├── lex_block.rs       classification, byte spans, indentation, fences
├── lex_inline.rs      nesting, code-is-raw, marks land on byte boundaries
├── scan.rs            the bound holds; a rule fires exactly at its trigger
├── parse.rs           TOTALITY — a proptest over arbitrary bytes must always terminate
├── lower.rs           sort_key ordering, mark containment, block ids
├── normalise.rs       idempotence, as a proptest
├── sanitise.rs        the XSS corpus, unwrap-not-drop, own-output round trip
└── projection.rs      replay(emit(lower(ast))) == lower(ast)
```

**`parse.rs`'s totality proptest is the one that matters.** Generate arbitrary byte strings, assert
the parser terminates and returns a tree. It catches the class of bug that hangs the editor, and
`compiler.html` already found one of them — a parser that could not advance past an unterminated
fence.

---

## 8. Build order

Phase 1 builds steps 1–6. Steps 7 onward are Phase 3 and are specified in
[`collaboration-service.md`](collaboration-service.md) §3–§5.

1. **`tree.rs`** — `BlockKind`, the block tree, JSONB round trip. Needed by `document-service` too.
2. **`lex/block.rs`** — line classification with byte spans. Activate `lex_block.rs`.
3. **`lex/inline.rs`** — text + marks over byte ranges. Activate `lex_inline.rs`.
4. **`parse.rs` + `ast.rs`** — recursive descent. Activate `parse.rs`, **totality proptest first**.
5. **`lower.rs` + `normalise.rs`** — Activate both. Idempotence as a proptest.
6. **`sanitise.rs`** — allowlist, then `cargo-fuzz` immediately.
7. `anchor.rs` → `rope/` → `marks.rs` → `ops.rs` — Phase 3.

**Add the `wasm32` CI gate before step 1**, not after:

```
cargo check -p domain -p doc -p diagnostics --target wasm32-unknown-unknown
```

There is no cloud increment for this phase — it is a library and ships inside its consumers.

---

## 9. Implementation notes — the things that will bite

### Byte offsets, char offsets, and grapheme clusters are three different indices

Marks are **bytes**. Cursor movement is **graphemes**. `String` indexing is bytes but `.chars()` is
scalars. Conflating any two breaks on the first emoji or combining character.

**Your fixtures must contain non-ASCII from the first test.** An all-ASCII corpus passes every
version of this bug — which is why `compiler.html`'s sample carries `café` and an em dash.

### A parser that cannot advance hangs the editor

The failure is silent in tests and fatal in the product: a token no branch consumes, and the loop
spins. `compiler.html` hit exactly this on an unterminated fence. Two defences, use both: **do not
emit error sentinel tokens**, and assert progress in the loop.

### `sanitise` before `parse`, never after

Parsing attacker-supplied HTML and sanitising the result means the parse already ran on hostile
input. Order is a security property here, not a style preference.

### Idempotence is a proptest, not an example

`normalise(normalise(x)) == normalise(x)` fails on inputs nobody writes by hand — an empty span
between two identical mark sets, a zero-length mark at a boundary. Generate them.

### `crates/document-core` will acquire an infrastructure dependency by accident

`std::time::Instant` panics under `wasm32`. `rand` needs `getrandom`'s `js` feature. Someone adds
one for a timestamp and the browser build dies months later. **The CI gate is the only thing that
catches this at the right time.**

### Do not let a `#[cfg(target_arch)]` change behaviour silently

Splitting a code path by target is legitimate — see `ROADMAP.md` § *Rayon and `wasm32` cannot both
be unconditional*. Splitting **semantics** by target means the browser and the server disagree about
the same document, which is the one bug this crate exists to prevent. Performance may differ by
target; output may not.
