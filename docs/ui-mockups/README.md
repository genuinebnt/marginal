# UI Mockups

Static, non-functional visual specs. Open one directly in a browser — there is no build step, no backend, no real CRDT. The pages link to each other through the top navigation, so the set browses like the real app.

These exist to make interaction decisions concrete before any code is written, and to check that the UX the architecture implies is actually usable.

| File | Shows | Asserts |
|---|---|---|
| `home.html` | Landing page — the pitch, self-hosting, pricing | ADR-001 — self-hosting is feature-identical, not a cut-down build |
| `signin.html` | Sign in **and first-run setup** | Phase 2 — a fresh instance's first screen is not a login |
| `editor.html` | Full editor chrome: page tree, input rules, diagnostics gutter, inspector tabs, panel takeover, bubble menu, ⌘K, presence | RFC-001, RFC-003, ADR-004 |
| `reader.html` | Reader: outline rail, reading tools, sidenotes, progress, reactions, comments | ADR-009 §9 — view state never enters the tree |
| `search.html` | Results, facets, link graph, fuzzy suggestions | Phase 7 — the index has its own cadence and may lag |
| **`graph.html`** | Explore the `[[link]]` graph — clusters, drag, filter, neighbourhood, **territory** | **Real simulation** plus an **exact Voronoi diagram** and its Delaunay dual |
| **`graph-algorithms.html`** | The same graph, analysed — components, cycles, paths, blast radius, **wavefront**, **topology** | **Real algorithms**: BFS, flood fill, three-colour DFS, reachability, Betti numbers |
| **`discover.html`** | Semantic related pages, with the index search visible | **Real HNSW**: layers, greedy descent, neighbour pruning, recall@5 |
| **`facts.html`** | Named definitions, transclusion, and what goes stale when one changes | **Real invalidation**: a dependency DAG walked in topological order, cycles rejected — and **pages are editable**, so edges can be added, not just dirtied |
| **`compiler.html`** | **Paste and import** as a compiler — buffer, tokens, AST, block tree, ops — against the two triggers that run almost none of it | **Real pipeline**, a **real input-rule scanner**, and the projection check can go red |
| **`analytics.html`** | Workspace analytics — the sketches *are* the privacy mechanism | **Real HyperLogLog, Count-Min, t-digest**, each beside its exact answer |
| **`trace.html`** | An op-log debugger — step, invert, watch the document change | **Real `apply` and `invert`**, with the invertibility law re-checked every step |
| **`netcode.html`** | One editor, four lenses — prediction, rollback, the transform, the block tree, the op DAG, the stored log | **Real OT** over text *and* block ops, **real Merkle comparison**, DP over the DAG, and a **real LSM-shaped log** |
| **`diff.html`** | Revision diff with the LCS table exposed | **Real algorithm**: full O(n·m) DP and its traceback |
| **`perf.html`** | Latency percentiles, queue depth, bundle treemap, flame graph | **Real measurement**: the scan benchmark runs on your machine |
| `history.html` | Version scrubber, op stream by actor, per-actor undo, **palimpsest** | DATA_MODEL §1, RFC-002 §3 — and one **real persistent sequence** |
| `admin.html` | Instance health, services, people, backups | CLOUD_ROADMAP §5 — outbox depth and op-log lag are the two that matter |
| `settings.html` | The three settings scopes and the startup/runtime split | ADR-009 §9, Phase 20 |
| `mockup.css` | Shared tokens and components — both themes | — |

Start at `home.html` for the product, `editor.html` for the app, and `graph.html` if you
want to see an algorithm from the DSA map actually running.

Every surface here has an owning phase in `ROADMAP.md` § Mockup Coverage. If something is
drawn that the table cannot place, either the roadmap is short or the mockup is wrong.

**Design tokens live in `mockup.css`, not in each page.** The original single-file rule stopped being right once there was more than one page; a shared stylesheet loads fine over `file://`. Page-specific styles stay inline in the page that needs them.

### The colour system carries meaning

Four hues, each with exactly one job, and none of them themeable:

| | Means |
|---|---|
| **Amber** `#B8791E` | A diagnostic. Never red — a notebook has no compile step, so nothing is ever "broken" |
| **Teal** `#1F8A75` | You, and healthy state |
| **Violet** `#7A5AC2` | Another person — presence, comments, mentions |
| **Slate** `#4F6D9A` | The assistant. Cool against a warm ground because it is not a person (ADR-009 §7) |

Actor colour appears in the presence stack, comment threads, and the history scrubber's ticks, so "who changed this" reads at a glance in three places without a legend.

### Details that carry the polish

- **No scrollbars.** A system bar is a light rectangle stapled to a dark ground, and it changes the layout width the moment it appears. Scroll position is carried by the reading-progress rule instead.
- **Tints are derived, never hand-picked.** `--amber-soft` is `color-mix(in srgb, var(--amber) 14%, var(--bg))`, so it stays correct on both grounds without a second table of hex values.
- **One motion vocabulary** — `--dur-1/2/3` and `--ease-out` / `--ease-spring`. Everything pressable moves half a pixel on `:active`; content reveals on mount, chrome does not.
- **Floating panes are glass** — blur plus saturation lifts the colour beneath, and an inset top highlight is the specular edge that stops it reading as a translucent rectangle.
- **Type is configured once.** Fraunces is variable, so `font-optical-sizing: auto` lets the optical axis actually track rendered size; ligatures are on in prose and off in code; anything that columns up gets `tabular-nums`.
- **Charts are drawn, not authored.** Sparklines and the link graph are generated from arrays, with an area fill, a soft line, and an emphasised endpoint — the only value anyone reads off a sparkline.

### The algorithm pages

Eleven of these are a different kind of mockup: the algorithm they visualise **actually runs**,
so the picture cannot drift from the thing it describes. `trace.html` and `compiler.html` go
further and *verify an invariant* rather than demonstrating one.

| Page | What genuinely executes |
|---|---|
| `graph.html` | A seeded force simulation that **cools to a stop** and reheats on drag — a settled layout redrawing at 60fps is a bug you cannot see |
| `graph-algorithms.html` | BFS shortest path, connected components by flood fill, three-colour DFS cycle detection, forward reachability, all-pairs BFS for diameter |
| `trace.html` | `apply` and `invert` for seven op kinds, and `apply(invert(op), apply(op, doc)) == doc` **verified on every step** rather than asserted. Stepping backwards runs inverses; it never restores a snapshot |
| `analytics.html` | HyperLogLog over 64 registers, a 4×24 Count-Min table, and a t-digest — all consuming a live stream, each displayed **next to the exact answer and the error**. A sketch that hides its error is indistinguishable from a wrong number |
| `discover.html` | An HNSW built and queried in the page — layer assignment, greedy descent, `Mmax` pruning — plus **recall@5 by brute-force comparison**, because speed without recall is half a result |
| `netcode.html` | One editor over a wire you control, and four views of the same live session. Ops are predicted locally, **rolled back with real inverses**, and transformed on both sides — over InsertText, DeleteText, InsertBlock and DeleteBlock. **Tree · Merkle** compares your predicted tree's AHU subtree hashes against the server's confirmed one, so the first amber node *is* the divergence point. **Causality · DAG** lays out the op graph by `basedOn` and lights the longest causal chain by DP over the total order. **Log · LSM** stores the sequencer's log the way an LSM engine would — a hot memtable flushing into progressively larger immutable levels that compact as they age. Two instruments disagree on purpose: a structural digest (the desync alarm) and an **intent ledger**. Turning the transform off leaves every replica in perfect agreement and the document quietly wrong, which is the page's argument. A fifth invariant replays the log **from empty** and compares it against the confirmed view the client assembled incrementally — two constructions of one state, by different code |
| `diff.html` | The full LCS dynamic-programming table and its traceback — the rendered matrix *is* the computed table, recomputed live when you switch word/character granularity |
| `compiler.html` | Block lexer → inline parser → AST → block tree → ops, then **the ops are replayed into a second tree and compared field by field**. The header pill says HOLDS from the comparison, not from confidence. Marks are byte ranges, and the default text contains an em dash and `café` so char and byte offsets visibly diverge. The **trigger selector** is the second algorithm: an input rule implemented as a genuine bounded backward scan — no grammar, no recursion, a hard 48-byte lookbehind — beside a keypress that reads nothing at all. **Bytes read** is the only figure on the page that does not grow with the document |
| `facts.html` | A dependency DAG with **topological dirty propagation** — edit a definition and only what is genuinely downstream is marked, transitively. Cycles rejected by three-colour DFS, duplicate definitions caught as a hash collision. Page bodies are editable, so the graph's **shape** changes too: adding a `{{ref}}` seeds only the page that gained the edge, dropping one seeds nothing, and editing prose walks nothing at all — three edits, three costs. The counter shows nodes *visited* against nodes that exist, which is the whole argument for incremental over full recompute |
| `history.html` | One paragraph backed by a **tombstoned persistent sequence**. A delete writes a version stamp and never removes, so every version is the filter `ins ≤ v < del` over one array — 229 chars stored, 145 live at head, zero copies. Palimpsest mode reads the tombstones the live text is read from |
| `perf.html` | A timed scan benchmark on your own machine, percentiles over log-spaced buckets, a squarified treemap, and a flame layout walked from a call tree |

They exist because these are ROADMAP § Rust, DSA & Concepts Map items, and a picture of an
algorithm teaches much less than one you can poke. Each states its cost honestly too —
`diff.html` shows the O(n·m) table *and* argues against using it.

**Building these found two bugs.** In `trace.html`, two ops carried payloads that did not match
their offsets, so their inverses reinserted the wrong text — the law check caught it on the first
run. In `compiler.html`, the parser could not advance past an unterminated fence and hung the
page; the fix was to stop emitting an error token the parser had to remember to consume. Both are
the argument for making these pages compute rather than illustrate.

`compiler.html` and `graph.html` were also checked outside the browser before being called real:
12,000 randomised token-soup inputs through the compiler with zero projection mismatches, and the
Voronoi cells verified to tile the viewport with no residual, with 20,000 random points each
landing in the cell of their nearest site.

---

## `editor.html`

**What is genuinely interactive** (implemented so the interaction can be judged, not just looked at):

- `## ` + space converts to a heading — RFC-001 §3 input rules
- Selecting text raises the floating format toolbar — ADR-004
- The toggle block expands and collapses as **view state only** — RFC-001 §1
- The `[[Meeting Notes]]` quick fix transitions the diagnostic to resolved — RFC-003 §6

**What is faked:** the remote collaborator. Ada's typing is a `setTimeout`, not a WebSocket. There is no CRDT, no op log, no server.

### What it asserts about the design

| Decision | Doc |
|---|---|
| Per-block `contenteditable`, never document-wide | ADR-004 |
| Diagnostics in a **left gutter** — dotted underline, amber, **never red** | RFC-003 §2 |
| Ops address content by **anchor**, never integer offset | RFC-002 §2 |
| Every op is invertible — `MoveBlock` carries `from` as well as `to` | RFC-002 §3 |
| Toggle collapse is view state, not model state | RFC-001 §1 |

The colour choice is load-bearing, not decorative: a notebook has no compile step, so nothing in it is ever "broken." A red squiggle on prose reads as an accusation. Dotted amber reads as *heads up*.

### Deliberately absent

Still undrawn, each with an owning phase in `ROADMAP.md` § Mockup Coverage: notifications
inbox, media library, plugin directory, the space and role editor, and offline/reconnect state.

Out of scope per ADR-001 and therefore intentionally **not** shown: **databases, tables, relations, rollups, formula language, spatial canvas.** If one of them appears in a mockup, either the scope changed — which needs an ADR — or the mockup is wrong.

Comments, permissions, spaces, and templates were on that list until **ADR-009** brought them into scope. They are now drawable, but only from Track 4 onward.

---

## Editor chrome — the spec behind the pixels

Drawn in `editor.html` and `reader.html`. Kept here in prose because the reasoning is what
a future mockup has to be checked against, and a rendered page cannot argue for itself.

### Left rail — the page tree

The LTREE tree from Phase 1, made visible.

| Element | Backed by |
|---|---|
| Drag to reorder or reparent | `sort_key` + `reparent` — **one row written per move**, no sibling renumbering |
| Filter box | Phase 7's BK-tree fuzzy title matching |
| Status affordance | `lifecycle_state` — a page mid-`deleting` saga must not look active |
| Collapse / expand | View state. **Never** stored on the block (RFC-001 §1) |

**The open-tabs question, before it is built.** In a single-author CMS a tab is a loaded
document. Here an open page is a **live WebSocket session**, routed by `hash(page_id)` to
one owning `collaboration-service` instance (ADR-001). Five tabs is five sessions, five
ropes in memory, five presence registrations — potentially on five instances.

Decide explicitly whether a background tab keeps its session or drops to a cold read and
re-opens on focus. This is a Phase 10 routing decision wearing a UI costume.

### Right panel — the inspector

Seven candidate tabs, which is already too many to show at once:

| Tab | Phase | Notes |
|---|---|---|
| Outline | 1 | Tree walk filtered to headings and notable kinds; click to jump |
| Diagnostics | 4 | Grouped by source — structural, then grammar (RFC-003 §2.1) — plus **checks passed**, not only failures |
| Backlinks | 7 | The reverse index, which already exists for incremental diagnostics |
| Comments | 14 | Anchored threads (ADR-009 §2) |
| Presence | 3 | Who else is here — no equivalent exists in a single-author tool |
| History | 6 | Op-log replay, not snapshot diffing |
| Page | 13 | Visibility, publish slug, comment policy (ADR-009 §9, page scope) |

**Panel takeover, not an eighth tab.** Selecting a configurable block replaces the panel
with that block's settings plus a back affordance — code language and filename, image
`width_ratio` and caption, link target, directive attributes. A permanent "Block" tab
would be empty most of the time.

Remount per selection so fields reseed from the block's attributes rather than holding
stale state.

### The format strip

**There is no persistent formatting toolbar.** A top strip implies one document-wide edit
surface; Marginal is per-block `contenteditable` (ADR-004). Three affordances replace it:

| Affordance | Raises | Handles |
|---|---|---|
| **Bubble menu** | Text selection | Inline marks only — bold, italic, strike, code, link, `[[pagelink]]` |
| **Slash menu** | `/` at block start | Block insertion — the Phase 7 trie |
| **Block hover** | Pointer over a block | Drag handle + insert affordance |

One division of labour worth stating, because it prevents a class of bug: **the bubble
menu is hidden for code blocks, images, and directives.** Those are configured in the
inspector. A selection inside a code block must not offer *bold* — RFC-001's grammar says
`Code` has no `Spans`, and the UI should make that unreachable rather than merely ignored.

The strip never duplicates document actions (save, publish, share) or block configuration.
Those live in the status bar and the inspector respectively.

---

## Reader chrome

### Reading preferences

Width (narrow / standard / wide) · font family (sans / serif / mono) · scale (S / M / L /
XL) · line spacing (tight / normal / relaxed) · focus mode · reading progress.

**The rule that makes these safe:** reading preferences are **view state and must never
enter the block tree.** This is the same rule as toggle collapse in RFC-001 §1, and the
failure mode is worse — if font size were model state, changing your own text size would
be a collaborative edit that resized the document for everyone on the page.

They belong to the **user** scope in ADR-009 §9, stored server-side per user rather than
in `localStorage`, because a self-hosted notebook is read from more than one device.

### Footnotes as sidenotes

A footnote is a block kind (Phase 16). Whether it renders at the bottom or in the margin
is a **rendering choice over the same tree**, driven by available width — margin when the
prose column leaves room, inline collapsed when it does not.

Nothing about the document changes when the window is resized. If a width change would
alter the tree, the design is wrong.

### The reader's left bar

The same outline the editor's Outline tab renders — one tree walk, two placements. Focus
mode hides both rails.

### Corrections applied to the original draft

The first draft contradicted three documented rules. Recorded here because the same mistakes are easy to make again:

1. **`InsertText { offset: usize }`** → `at: Anchor`. Integer offsets are invalidated by concurrent remote edits; this is the single most important correctness detail in the operation model.
2. **`SetProperty { key: String, value: Value }`** → `SetMark { mark: Mark, on: bool }`. A stringly-typed property bag is exactly the design ADR-001 rejects — it punches a hole in the tagged-enum type safety the block model depends on.
3. **`MoveBlock { new_parent, index }`** → `{ from, to }`. An op that records only its destination cannot be inverted.

Also fixed: the heading-skip diagnostic pointed at an `h3` following an `h2`, which is a *correct* progression — it demonstrated nothing. The markup is now an `h4`, so the skip is real.

---

## Adding a mockup

- One HTML file per page, linked into the top nav so the set stays browsable
- Tokens and shared components go in `mockup.css`; page-specific styles stay inline
- A header comment stating what is real, what is faked, and which docs it asserts
- **Something must genuinely work.** A mockup where nothing responds cannot be judged, only admired — each page here makes its central claim interactive
- Both themes, via the token block. The toggle stamps `data-theme` and must beat `prefers-color-scheme` in both directions
- Never show a feature that is out of scope
- If a mockup and a doc disagree, **the doc wins** — or the doc needs changing first
