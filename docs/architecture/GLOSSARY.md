# Marginal — Glossary

Ubiquitous language. These terms mean exactly this in code, docs, and conversation.

---

## Document

| Term | Definition |
|---|---|
| **Page** | A document. Pages form a shallow tree via `parent_id`. A page is a container for blocks and the unit of collaboration — one page, one CRDT session. |
| **Block** | The atomic content unit. Everything in a page is a block. Blocks form a tree; a toggle or list item contains children. |
| **Block Kind** | The semantic variant: `paragraph`, `heading_1..3`, `bulleted_list`, `numbered_list`, `todo_list`, `quote`, `toggle`, `code`, `image`, `divider`. Serialised `snake_case`. |
| **Block Tree** | The recursive structure of blocks within a page. Stored as an adjacency list plus LTREE path; **materialised** as a tree at read time. |
| **Span** | A run of text with a set of marks. A block's text is an ordered list of spans. |
| **Mark** | Inline formatting on a span: `bold`, `italic`, `strike`, `code`, `link(url)`, `pagelink(page_id)`. Absent means false — never `"bold": false`. |
| **Normalised** | Canonical span form: adjacent spans with identical mark sets merged, empty spans removed, mark keys in fixed order. `normalise ∘ normalise = normalise`. |
| **Sort Key** | A fractional index string ordering siblings. Reordering writes **one** row; siblings are never renumbered. |
| **Path** | An LTREE materialised ancestry path (`root.a1b2.c3d4`). Enables subtree queries without a recursive CTE. |

## Operations

| Term | Definition |
|---|---|
| **Op** | An operation — the *only* way anything changes. UI compiles intents to ops; nothing mutates the tree directly. |
| **Op Log** | The append-only sequence of ops. **The source of truth.** Block rows are a projection of it. |
| **Invertible** | Every op defines its inverse, and `apply(invert(op), apply(op, t)) == t`. This is why `DeleteBlock` carries the removed subtree. |
| **Anchor** | A position that survives concurrent remote edits. Never an integer offset — offsets are invalidated by other people's edits. |
| **Encoding Version** | A `u16` on every persisted op. The log is a permanent wire format; old versions must decode forever. |
| **Actor** | Whoever authored an op — a user in a session. Undo is scoped to an actor's own ops. |
| **Vector Clock** | Per-actor logical timestamps establishing causal ordering between ops. |
| **Tombstone** | A deleted CRDT element retained for merge correctness until all peers acknowledge the deletion. |

## Collaboration

| Term | Definition |
|---|---|
| **CRDT** | Conflict-free Replicated Data Type. Concurrent edits converge to the same state with no coordination and **no merge-conflict UI, ever**. |
| **Rope** | The in-memory text structure used during a live session — efficient insert/delete at arbitrary positions. |
| **Working Format** | `rope + anchored marks`, live only inside `collaboration-service` and the editor core. Authoritative while a session is open. |
| **Storage Format** | The `spans` JSONB array. What Postgres holds and the API returns. A checkpoint, not the truth, during a live session. |
| **Presence** | Who is currently on a page — cursor position, selection, colour, display name. Ephemeral; Redis with a TTL. |
| **Session** | One page open by one or more actors. Also a history grouping: a burst of edits close in time. |
| **Convergence** | The property that any interleaving of the same op set yields identical state. |

## Diagnostics

| Term | Definition |
|---|---|
| **Diagnostic** | A hint, warning, or info attached to a span. **Never an error** — a notebook has no compile step, so nothing is ever "broken". |
| **Analyzer** | An independent function producing diagnostics from the tree plus a resolution context. Adding one is purely additive. |
| **Resolution Context** | Everything an analyzer needs beyond the block itself — chiefly the page-title symbol table. |
| **Symbol Table** | The page-name → page-id map. `[[Page Link]]` resolution is symbol resolution. |
| **Reverse Index** | page-title → pages-linking-here. Serves both diagnostic invalidation on rename **and** the backlinks panel. |
| **Quick Fix** | A remedy expressed as ops, so it is undoable, syncs to peers, and lands in history for free. |
| **Incremental** | Re-analysing only what an edit could have affected. A correctness requirement, not an optimisation — a lagging pass squiggles text already fixed. |
| **Dangling Link** | `[[Name]]` with no matching page. A hint with a *create page* fix, not an error. |

## Architecture

| Term | Definition |
|---|---|
| **Slice** | A vertical feature module owning `model.rs` + `repo.rs` + `handlers.rs` (+ `service.rs` only when logic exists). The unit of organisation. |
| **Outbox** | An event row written in the **same transaction** as the state change, published later by a poller. Fixes the dual-write between Postgres and NATS. |
| **Dual Write** | Writing to two systems without a distributed transaction. The bug the outbox exists to prevent. |
| **At-least-once** | The only delivery guarantee available. Therefore every consumer must be **idempotent**, deduping on op or event id. |
| **Saga** | A multi-service transaction coordinated by events, with forward-only compensation. Page deletion is the one saga here. |
| **Degradable** | A service whose absence removes a feature without breaking the product. `diagnostics-service` is the example. |
| **Projection** | Derived state rebuildable from the op log — block rows, history snapshots, the search index. |
| **CQRS** | Separating the write model (op log) from read models (`history-service`, search index). |
| **Fencing Token** | A monotonically increasing number proving lock ownership, so a paused-then-resumed worker cannot corrupt state. |

## Terms deliberately absent

**Workspace · Member · Role · Permission · Database · Property · Formula · View · Relation · Rollup · Template.**

These belong to the cut scope (ADR-001). If one appears in code or conversation, either the scope changed — which needs an ADR — or the wrong word is being used.
