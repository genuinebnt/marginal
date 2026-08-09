# RFC-003 — Diagnostics Engine

**Status:** Accepted
**Date:** 2026-08-06
**Affects:** diagnostics-service, editor core, document-service
**Related:** ADR-001 (degradable service), RFC-001 (document model), RFC-002 (op stream)

---

## 1. What This Is

Squiggles under prose, as you type. IDE-grade static analysis applied to a notebook rather than to code — the product's main differentiator, and the reason `diagnostics-service` exists as its own boundary.

**Diagnostics are not parse errors.** The block tree cannot be syntactically malformed — structure is stored, not parsed (RFC-001 §1). So every diagnostic here is **semantic or structural analysis over a well-formed AST**, which is a different discipline: symbol resolution, well-formedness lints, and reference integrity.

That distinction is what makes this tractable. There is no error recovery problem, because there is nothing to recover from.

---

## 2. The Analyzer Set

Each analyzer is an independent function over the tree plus a resolution context. Adding one is additive — no infrastructure changes.

| Analyzer | Detects | Severity | Quick fix |
|---|---|---|---|
| **DanglingPageLink** | `[[Meeting Note]]` → no such page | hint | *Create page and link* |
| **AmbiguousPageLink** | `[[Notes]]` matches two pages | warning | Disambiguate by id |
| **HeadingSkip** | H1 → H3 with no H2 | hint | Demote to H2 |
| **EmptyCodeBlock** | Code block with no language set | hint | Pick a language |
| **DuplicateTitle** | Two pages share a title — makes links ambiguous | warning | Rename |
| **OrphanPage** | Nothing links here and it is not a root | info | — |
| **BrokenImage** | `file_id` resolves to nothing | warning | Re-upload / remove |
| **SelfLink** | Page links to itself | info | — |
| **LinkCycle** | `A → B → A` link cycle | info | — |

**Deliberately not analyzers:** spelling and grammar. See §2.1 — they arrive as a
*second source*, not as entries in this table.

### 2.1 Grammar and spelling — a second source, not an analyzer

**Amended 2026-08-07.** The original text ruled grammar out entirely: it needed a
dictionary and a language model, it was noisy, and it taught nothing the analyzers above
did not. Two of those three no longer hold.

[`harper-core`](https://github.com/Automattic/harper) is a grammar and spell checker
written in Rust that compiles to `wasm32` and runs **entirely in the browser** — no
dictionary to ship separately, no model server, no network call. It lands exactly on the
boundary `agents.md` already mandates: diagnostics cross the `wasm-bindgen` boundary and
are never reimplemented in JS.

The "noisy" objection stands, and shapes the design:

| | Structural analyzers (§2) | Grammar source |
|---|---|---|
| Input | Block tree + resolution context | Prose text of one block |
| Triggered by | Op applied, symbol table change | Debounced typing pause |
| Scope | Cross-page (links, titles, orphans) | Within a block, never cross-page |
| Severity ceiling | `warning` | `hint` — never higher |
| User control | Always on | **Toggleable, off by default** |

### The tier rule

Two invalidation tiers exist, and every new check must be placed in one of them. The rule:

> **A check belongs in the fast tier only if every one of its inputs lives inside one block.**
> Anything reading another block, another page, or the symbol table is structural.

This is not a performance guideline, it is what keeps §4 tractable. A cross-page dependency in
the per-keystroke path makes every keystroke a candidate for cross-page recomputation.

**The fact dependency graph is structural** (ROADMAP § Phase 4). A `{{transclusion}}` reads a
definition that may live on another page, so editing a definition invalidates through the op
event and the reverse index, debounced — never on the keystroke that typed it. Its severity
ceiling is `hint`: a note whose citation moved on is not broken, which is why
`ui-mockups/facts.html` is amber and never red.

**It is a separate source, not a tenth analyzer.** Two reasons, and both matter for §4:

1. **Different invalidation cadence.** Structural diagnostics invalidate on ops and on
   symbol-table changes. Grammar invalidates on a typing pause within one block. Merging
   them into one dependency graph would make every keystroke a candidate for cross-page
   recomputation.
2. **Different failure posture.** A structural analyzer that disagrees with the tree is a
   bug. A grammar checker that flags a deliberate stylistic choice is working correctly
   and still wrong for the user. Only one of those deserves to be always-on.

They merge only at presentation — one panel, grouped by source, which is where the user
wants them. `EditorDiagTab` in `genuine-folio` groups them exactly this way, for exactly
this reason.

**Phase:** ships with Phase 16 (the full editor), not Phase 4. The analyzer engine must
exist and be correct before a second producer is attached to it.

### Severity drives presentation

`hint` renders as a dotted underline, `warning` as a dashed one, `info` as a gutter marker only. **Nothing renders as a red error squiggle** — a notebook has no compile step, so nothing is ever "broken." A jarring red underline on prose reads as an accusation. This is a UX rule, not a cosmetic one.

---

## 3. The Symbol Table

`[[Page Link]]` resolution is a **symbol table**: names to ids, with dangling-reference detection. This is the project's semantic-analysis component.

```rust
trait ReferenceResolver {
    fn resolve(&self, name: &str) -> Resolution;
}

enum Resolution {
    Unique(PageId),
    Ambiguous(Vec<PageId>),
    Missing,
}
```

Behind a trait per ADR-001's seam — a second implementation would arrive only if a formula language ever needs property lookups.

### The reverse index is what makes it incremental

```
   forward:  page → [outgoing links]        (from the block tree)
   reverse:  page title → [pages linking here]
```

The reverse index does double duty: **diagnostic invalidation** (rename a page and every dangling-link diagnostic pointing at the old name must re-evaluate) and **the backlinks panel**, which is a product feature. One structure, two consumers.

---

## 4. Incrementality Is a Correctness Requirement, Not an Optimisation

> Re-checking the whole document per keystroke is not merely slow — a diagnostics pass that lags behind the caret produces squiggles under text the user already fixed. That reads as broken.

So: **only re-analyse what the edit could have affected.**

This is **query-based incremental computation**. The reference implementation is [`salsa`](https://github.com/salsa-rs/salsa) — what rust-analyzer uses for exactly this problem. Either use it or hand-roll the same model.

```
   op arrives  ──▶  which blocks changed?
                          │
                          ▼
                    invalidate memoised results whose inputs include those blocks
                          │
                          ▼
                    recompute only invalidated queries
                          │
                          ▼
                    stream results back as each completes
```

### The dependency graph

| Query | Depends on | Invalidated by |
|---|---|---|
| `block_spans(block)` | that block's content | an op touching that block |
| `outgoing_links(block)` | `block_spans(block)` | the above |
| `page_titles()` | all page titles | any `SetTitle` |
| `resolve(name)` | `page_titles()` | any page created, renamed, deleted |
| `heading_structure(page)` | ordered heading blocks of the page | heading insert/delete/move/kind change |
| `diagnostics(block)` | that block's analyzers' inputs | any of the above |

**The non-obvious case:** a `SetTitle` on page A invalidates `resolve()` and therefore `diagnostics()` on **every block in every page that links to A** — blocks nobody touched. That is precisely why the reverse index exists, and why naive "re-check the edited block" is wrong.

### Anchoring

A diagnostic's span must survive concurrent remote edits. `"unclosed at bytes 41–52"` is invalidated by someone typing at byte 20.

**Diagnostic spans use the same anchor mechanism as rich-text marks** (RFC-001 §2). This is not a coincidence to exploit later — build anchoring once and both work. It is the strongest structural argument for the block-plus-anchors model.

---

## 5. Transport and Degradation

`collaboration-service → diagnostics-service` is a **gRPC server-streaming** call (ADR-006): one long-lived stream per open document, results pushed as computed.

**Degradation is a transport property, not a code path.** If `diagnostics-service` is unavailable the stream fails to open, the client renders no diagnostics, and editing is unaffected. No request to retry, no queue to drain, no user-visible error.

That property is why this is a separate service. It is also demonstrable in an interview: kill the pod, keep typing.

---

## 6. Quick Fixes

A quick fix is an **op**, not a mutation (RFC-002 §1):

```
DanglingPageLink → [ InsertBlock(new page root), SetTitle(name), SetMark(pagelink) ]
HeadingSkip      → [ SetBlockKind { from: heading_3, to: heading_2 } ]
DuplicateTitle   → [ SetTitle { from, to } ]
```

Because fixes are ops, they are undoable, they synchronise to peers, and they land in history — all for free. A fix that mutated the tree directly would break all three.

---

## 7. Where It Runs

Two callers, one implementation, compiled twice:

| Caller | Purpose |
|---|---|
| `diagnostics-service` (native) | Authoritative pass; results streamed to all sessions on the page |
| Editor core (`wasm32`) | Local pass for instant feedback on the block being typed |

The analyzers live in `libs/diagnostics`, a pure library with no infrastructure dependencies — `cargo test` only, and therefore Miri-reachable and fuzzable.

**Local and authoritative results must agree.** Same crate, same inputs, so a disagreement is a bug in the resolution context passed in, not in the analyzers. A property test should assert agreement given identical context.

---

## 8. Testing

| Law | Statement |
|---|---|
| **Determinism** | Same tree + same context → identical diagnostics, in a stable order |
| **Incremental equivalence** | Incremental result == full recompute, for any op sequence. **The most important test here** |
| **Never-panic** | No block tree, however malformed, panics an analyzer (`cargo-fuzz`) |
| **Fix correctness** | Applying a quick fix clears the diagnostic that offered it |
| **Fix invertibility** | Quick-fix ops obey RFC-002 §3 like any other op |
| **Local == authoritative** | WASM and native passes agree given the same context |

Incremental equivalence is the one that catches real bugs: run a random op sequence, compare incremental output against a from-scratch recompute after each step. Every invalidation bug shows up as a divergence.

---

## 9. Open Questions

1. **Debounce window.** Analyse per keystroke, or after a short idle? Trades feedback latency against churn — and it interacts with op coalescing (RFC-002 §9).
2. **Who owns the reverse index** — `diagnostics-service` locally materialised from NATS events, or `document-service` as the authority queried over gRPC? Locally materialised keeps degradation clean; it needs anti-entropy reconciliation.
3. **Cross-page diagnostics at scale.** A rename invalidating thousands of blocks is a fan-out spike. Batch, rate-limit, or accept it at this scope?
4. **`salsa` or hand-rolled?** `salsa` is the right model but a large dependency with an unstable API. Hand-rolling the memo table teaches more and risks subtle invalidation bugs.

---

## Resources

| Resource | For |
|---|---|
| [salsa](https://github.com/salsa-rs/salsa) | Query-based incremental computation |
| [rust-analyzer architecture](https://github.com/rust-lang/rust-analyzer/blob/master/docs/dev/architecture.md) | How a real incremental analyzer is structured |
| [LSP specification](https://microsoft.github.io/language-server-protocol/) | Diagnostic, severity, and code-action shapes worth imitating |
| [Anders Hejlsberg on incremental compilation](https://www.youtube.com/watch?v=wSdV1M7n4gQ) | Why incrementality changes the architecture, not just the speed |
