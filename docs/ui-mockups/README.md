# UI Mockups

**One file: [`v2/index.html`](./v2/index.html).** Forty screens, no build step, no
backend. Open it in a browser.

Alongside it, **[`v2/DESIGN_GUIDELINES.md`](./v2/DESIGN_GUIDELINES.md)** — the
prescriptive spec for implementing those screens 1:1. Read the guidelines before
writing UI; open the mockup while writing it.

These exist to make interaction decisions concrete before code is written, and to
check that the UX the architecture implies is actually usable. They are the
acceptance bar for `v2.0.0`–`v4.0.0` (`docs/planning/RELEASES.md`) — the full set,
not just the editing screens.

---

## The screens

| | Screen | Shows |
|---|---|---|
| `00` | Build map | routes → components → owned state |
| `01` | Page graph | how the 38 routes connect, and which named routes are not yet drawn |
| `02` | Home | the pitch, self-hosting, pricing |
| `03` | Register | create an account |
| `03b` | Log in | the returning door |
| `03c` | Dashboard | where you land after signing in — resume, not a blank page |
| `04` | Editor | page tree · diagnostics gutter · inspector · bubble menu · presence |
| `05` | Reader | outline rail · sidenotes · reading tools · progress |
| `05b` | Published page | the same page with the workspace taken away |
| `06` | Search | results · facets · fuzzy suggestions · the index may lag |
| `07` | Graph | clusters · drag · filter · neighbourhood · territory |
| `08` | Graph algorithms | components · cycles · paths · blast radius · wavefront |
| `09` | Discover | semantic neighbours · HNSW layers · recall@5 |
| `10` | Facts | definitions · transclusion · what goes stale when one changes |
| `10b` | Topics & tags | one owned classification, many free labels |
| `10c` | Lab index | the hub for six screens where the algorithm actually runs |
| `11` | Compiler | paste and import as a pipeline — buffer → tokens → AST → tree → ops |
| `12` | Analytics | the sketches *are* the privacy mechanism |
| `13` | Trace | an op-log debugger — step, invert, watch the document change |
| `14` | Netcode | one editor, four lenses — prediction, rollback, transform, log |
| `15` | Diff | revision diff with the LCS table exposed |
| `16` | Perf | latency percentiles · queue depth · bundle treemap · flame graph |
| `17` | History | version scrubber · op stream by actor · per-actor undo · palimpsest |
| `18` | Admin | workspace health · services · people · backups |
| `18b` | Audit log | who did what, derived from the op log rather than written beside it |
| `18c` | API keys & webhooks | the other way in — same ops, same permissions |
| `19` | Settings | three scopes, and the startup / runtime split |
| `20` | Notifications | mentions, proposals and checks — one inbox, cleared by acting |
| `21` | Media library | what the object store holds, and where each file is used |
| `22` | Plugins | a directory where every entry states what it may touch |
| `23` | Spaces & roles | who is in which space, and what the role actually permits |
| `23b` | Profile | a person as their op log |
| `23c` | Trash & restore | the delete saga, visible — deleting is a state, not an event |
| `23d` | Import / export | the way out is the whole argument for the way in |
| `24` | Offline / reconnect | the state every route can enter |
| `24b` | Command palette | ⌘K — one input that resolves to a page, an action, an answer |
| `24c` | Notifications panel | the bell, opened — triage without leaving the page |
| `24d` | Assistant | answers with citations, edits by emitting ops — never raw text |
| `24e` | Empty & not found | the two states a new workspace and a wrong URL land in |
| `25` | Components & motion | the pieces every screen is built from |

Start at `02` for the product, `04` for the app, `10c` for the algorithm screens.

Every surface has an owning version in `docs/planning/RELEASES.md`. If something is
drawn that the plan cannot place, either the plan is short or the mockup is wrong.

---

## The V1 set was removed

Nineteen standalone files (`editor.html`, `graph.html`, `mockup.css`, …) were
superseded by `v2/index.html` and deleted on 2026-08-28. Every screen survived the
move; only the packaging changed. Path-form citations throughout the codebase were
rewritten to `docs/ui-mockups/v2/index.html § NN NAME`.

**Bare-filename references were deliberately left alone.** `PROGRESS.md`, the
roadmap, and `docs/rust/`'s frozen archive refer to these screens in prose
(``makes `facts.html` real``) hundreds of times. Those entries were true when
written, and rewriting a chronological log to match a later reorganisation makes
it a worse record. Resolve them here:

| V1 file | V2 screen |
|---|---|
| `home.html` | `02 HOME` |
| `signin.html` | `03 REGISTER` + `03b LOG IN` |
| `editor.html` | `04 EDITOR` |
| `reader.html` | `05 READER` |
| `search.html` | `06 SEARCH` |
| `graph.html` | `07 GRAPH` |
| `graph-algorithms.html` | `08 GRAPH ALGORITHMS` |
| `discover.html` | `09 DISCOVER` |
| `facts.html` | `10 FACTS` |
| `compiler.html` | `11 COMPILER` |
| `analytics.html` | `12 ANALYTICS` |
| `trace.html` | `13 TRACE` |
| `netcode.html` | `14 NETCODE` |
| `diff.html` | `15 DIFF` |
| `perf.html` | `16 PERF` |
| `history.html` | `17 HISTORY` |
| `admin.html` | `18 ADMIN` |
| `settings.html` | `19 SETTINGS` |
| `mockup.css` | `v2/index.html`'s own `<style>` block |

If you need the originals — the V1 algorithm pages ran their algorithms live in
JavaScript, which V2 does not — they are in git history:

```
git log --diff-filter=D --oneline -- docs/ui-mockups/
git show <sha>^:docs/ui-mockups/graph-algorithms.html > /tmp/graph-algorithms.html
```

**V1's colour values do not carry over.** V2 runs a lighter, warmer palette on a
darker ground (amber `#E0A34E`, teal `#3FCFA8`, violet `#A98CE8`, slate `#7D9EC9`
over `#0E0F10`, against V1's `#B8791E` / `#1F8A75` / `#7A5AC2` / `#4F6D9A`). The
*meanings* are unchanged. `v2/DESIGN_GUIDELINES.md` §3 is authoritative.

`web/src/design-system.css` was a verbatim copy of the deleted `mockup.css` and
still is — it now has no upstream, and the screens built against it
(Search/History/Trace/Diff/Facts/Graph/GraphAlgorithms) still render V1's palette.
Reconciling them against V2 is outstanding work, tracked in `PROGRESS.md`, not a
silent drift.

---

## The colour system carries meaning

Four hues, each with exactly one job, none of them themeable:

| | Means |
|---|---|
| **Amber** `#E0A34E` | A diagnostic. Never red — a notebook has no compile step, so nothing is ever "broken" |
| **Teal** `#3FCFA8` | You, and healthy state |
| **Violet** `#A98CE8` | Another person — presence, comments, mentions |
| **Slate** `#7D9EC9` | The assistant. Cool against a warm ground because it is not a person (ADR-009 §7) |

Actor colour appears in the presence stack, comment threads, and the history
scrubber's ticks, so "who changed this" reads at a glance in three places without
a legend.

A fifth, categorical ramp colours **topics only** — protocol, storage, interface,
operations, research — and is deliberately disjoint from the four above. If a hue
meant both "diagnostic" and "topic: operations" it would mean neither.

---

## Details that carry the polish

- **No scrollbars.** A system bar is a light rectangle stapled to a dark ground,
  and it changes the layout width the moment it appears. Position is carried by
  the reading-progress rule and the status bar instead.
- **Zero border-radius**, everywhere, except status chips and pill toggles. This
  single decision does more to set the character than any other.
- **One motion vocabulary.** Six animations exist; everything pressable moves half
  a pixel on `:active`; content reveals on mount, chrome never does; anything over
  320 ms is a bug.
- **Floating panes are glass** — blur plus saturation lifts the colour beneath, and
  an inset top highlight is the specular edge that stops it reading as a
  translucent rectangle.
- **Charts are drawn from divs, not authored** and not libraried — a track, a fill
  sized by percentage, and a right-aligned mono value so columns line up.
- **Every panel ends with a sentence arguing for itself.** A panel without one is
  usually a panel that has not decided what it is for.

---

## Editor chrome — the spec behind the pixels

Drawn in `04 EDITOR` and `05 READER`. Kept here in prose because the reasoning is
what a future mockup has to be checked against, and a rendered page cannot argue
for itself.

### Left rail — the page tree

The LTREE tree, made visible.

| Element | Backed by |
|---|---|
| Drag to reorder or reparent | `sort_key` + `reparent` — **one row written per move**, no sibling renumbering |
| Filter box | BK-tree fuzzy title matching |
| Status affordance | `lifecycle_state` — a page mid-`deleting` saga must not look active |
| Collapse / expand | View state. **Never** stored on the block (RFC-001 §1) |

**The open-tabs question, before it is built.** In a single-author CMS a tab is a
loaded document. Here an open page is a **live WebSocket session**, routed by
`hash(page_id)` to one owning `collaboration-service` instance (ADR-001). Five tabs
is five sessions, five ropes in memory, five presence registrations — potentially
on five instances.

Decide explicitly whether a background tab keeps its session or drops to a cold
read and re-opens on focus. This is a routing decision wearing a UI costume.

### Right panel — the inspector

Seven candidate tabs, which is already too many to show at once:

| Tab | Notes |
|---|---|
| Outline | Tree walk filtered to headings and notable kinds; click to jump |
| Diagnostics | Grouped by source — structural, then grammar (RFC-003 §2.1) — plus **checks passed**, not only failures |
| Backlinks | The reverse index, which already exists for incremental diagnostics |
| Comments | Anchored threads (ADR-009 §2) |
| Presence | Who else is here — no equivalent exists in a single-author tool |
| History | Op-log replay, not snapshot diffing |
| Page | Visibility, publish slug, comment policy (ADR-009 §9, page scope) |

**Panel takeover, not an eighth tab.** Selecting a configurable block replaces the
panel with that block's settings plus a back affordance — code language and
filename, image `width_ratio` and caption, link target, directive attributes. A
permanent "Block" tab would be empty most of the time.

Remount per selection so fields reseed from the block's attributes rather than
holding stale state.

### The format strip

**There is no persistent formatting toolbar.** A top strip implies one
document-wide edit surface; Marginal is per-block `contenteditable` (ADR-004).
Three affordances replace it:

| Affordance | Raises | Handles |
|---|---|---|
| **Bubble menu** | Text selection | Inline marks only — bold, italic, strike, code, link, `[[pagelink]]` |
| **Slash menu** | `/` at block start | Block insertion — the trie |
| **Block hover** | Pointer over a block | Drag handle + insert affordance |

One division of labour worth stating, because it prevents a class of bug: **the
bubble menu is hidden for code blocks, images, and directives.** Those are
configured in the inspector. A selection inside a code block must not offer *bold*
— RFC-001's grammar says `Code` has no `Spans`, and the UI should make that
unreachable rather than merely ignored.

The strip never duplicates document actions (save, publish, share) or block
configuration. Those live in the status bar and the inspector respectively.

### What the editor asserts about the design

| Decision | Doc |
|---|---|
| Per-block `contenteditable`, never document-wide | ADR-004 |
| Diagnostics in a **left gutter** — dotted underline, amber, **never red** | RFC-003 §2 |
| Ops address content by **anchor**, never integer offset | RFC-002 §2 |
| Every op is invertible — `MoveBlock` carries `from` as well as `to` | RFC-002 §3 |
| Toggle collapse is view state, not model state | RFC-001 §1 |

The colour choice is load-bearing, not decorative: a notebook has no compile step,
so nothing in it is ever "broken." A red squiggle on prose reads as an accusation.
Dotted amber reads as *heads up*.

---

## Reader chrome

### Reading preferences

Width (narrow / standard / wide) · font family (sans / serif / mono) · scale
(S / M / L / XL) · line spacing (tight / normal / relaxed) · focus mode · reading
progress.

**The rule that makes these safe:** reading preferences are **view state and must
never enter the block tree.** This is the same rule as toggle collapse in RFC-001
§1, and the failure mode is worse — if font size were model state, changing your
own text size would be a collaborative edit that resized the document for everyone
on the page.

They belong to the **user** scope in ADR-009 §9, stored server-side per user rather
than in `localStorage`, because a self-hosted notebook is read from more than one
device.

### Footnotes as sidenotes

A footnote is a block kind. Whether it renders at the bottom or in the margin is a
**rendering choice over the same tree**, driven by available width — margin when
the prose column leaves room, inline collapsed when it does not.

Nothing about the document changes when the window is resized. If a width change
would alter the tree, the design is wrong.

### The reader's left bar

The same outline the editor's Outline tab renders — one tree walk, two placements.
Focus mode hides both rails.

---

## Corrections applied to the original draft

Recorded because the same mistakes are easy to make again. The first editor draft
contradicted three documented rules:

1. **`InsertText { offset: usize }`** → `at: Anchor`. Integer offsets are
   invalidated by concurrent remote edits; this is the single most important
   correctness detail in the operation model.
2. **`SetProperty { key: String, value: Value }`** → `SetMark { mark: Mark, on: bool }`.
   A stringly-typed property bag is exactly the design ADR-001 rejects — it punches
   a hole in the tagged-enum type safety the block model depends on.
3. **`MoveBlock { new_parent, index }`** → `{ from, to }`. An op that records only
   its destination cannot be inverted.

Also fixed: the heading-skip diagnostic pointed at an `h3` following an `h2`, which
is a *correct* progression — it demonstrated nothing. The markup is now an `h4`, so
the skip is real.

---

## Out of scope, and therefore deliberately not drawn

**Databases, tables, relations, rollups, formula language, spatial canvas** — per
ADR-001. If one appears in a mockup, either the scope changed, which needs an ADR,
or the mockup is wrong.

Comments, permissions, spaces, and templates were on that list until **ADR-009**
brought them into scope, and **ADR-012** scheduled them into `v2`–`v4`.

---

## Adding a screen

- Add it to `v2/index.html` — one file, so the set stays browsable and the tokens
  stay shared. A new file would immediately drift.
- Give it a `<div class="tag">` with a number, a name, and a one-line claim.
- Add it to `00 BUILD MAP` and `01 PAGE GRAPH`, or the accounting silently rots.
- Follow `v2/DESIGN_GUIDELINES.md`. If you need a colour, a radius, a font, or a
  motion it does not have, that is a **spec change** — state the reason and update
  the guidelines in the same commit.
- Every string must be a specific, true claim. No placeholders.
- Measure the result: nothing over ~150px of dead space at a column's bottom, and
  nothing overflowing its own frame.
- Never show a feature that is out of scope.
- **If a mockup and a doc disagree, the doc wins** — or the doc needs changing
  first.
