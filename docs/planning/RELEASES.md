# Marginal — Release Plan (`v2.0.0` → `v4.0.0`)

**Read this alongside `docs/porting/PROGRESS.md`** — this file is the version-level plan
("what's next, in what order"); `PROGRESS.md` stays the session-level log of what
actually landed. See `ADR-012` for why releases are shaped this way: each **major**
version is sized to be its own complete, self-contained Go→Rust porting unit — the same
size class as `v1.0.0` (the MVP: Documents, Auth, Collaboration) — so the user can port
one major version to Rust, then come back and keep building the next major in this repo,
rather than accumulating everything into one undifferentiated pile. Each **minor** is one
feature, built completely — backend and UI, real, usable from a browser — the same bar
each of `v1.0.0`'s own three phases had to clear.

**The frontend (TypeScript/HTML/CSS) is never ported** — only the Go services are,
one major version at a time. The same browser UI stays as the permanent visual
verification harness: point it at the Go backend or the in-progress Rust port and compare
behavior directly. This is the concrete reason a minor's UI has to be real and complete,
not stubbed — a half-built screen gives a future porting pass nothing to check itself
against.

**The acceptance bar is the whole mockup set, not a subset of it.** `docs/ui-mockups/`
is seventeen static pages, eleven of which run a real algorithm client-side (force-directed
graph layout + exact Voronoi, BFS/DFS/flood-fill, HNSW, HyperLogLog/Count-Min/t-digest, LCS
DP, a dependency DAG with topological invalidation, op apply/invert, OT with Merkle/DAG/LSM
views, and more — `docs/ui-mockups/README.md` § The algorithm pages). The end product for
`v2`–`v4` is **every one of those seventeen screens working against the real system, with
no missing feature** — not just the notebook-editing screens. `ROADMAP.md` § Mockup Coverage
already maps each mockup surface to an owning phase; this file's minors are that same mapping,
re-cut into shippable slices. Where a table below doesn't obviously look like a "product
feature," it's because it's one of these algorithm/observability pages — called out explicitly
in each minor's "Ships" column so it doesn't get silently dropped.

**Where the algorithm lives matters, and it's the same rule the editor core already
follows.** `CLAUDE.md`'s stack table already states it for `documentcore`: business logic is
Go, TypeScript is a view and a JSON bridge, never a second implementation. Every mockup
page above that "runs a real algorithm" extends that same rule — BFS/DFS/components/cycle
detection over the link graph, HNSW, the sketches, the LCS table, the dependency-DAG
invalidation, Merkle subtree comparison, the LSM-shaped log — all of it is **Go**, either
computed server-side and shipped to the client as data, or compiled to wasm (the same
`GOOS=js GOARCH=wasm` boundary `documentcore` already uses) when it has to run against
live client-side state. TypeScript only draws what Go computed: canvas/SVG rendering,
layout, controls. This isn't a style preference — it's *why* the Rust port carries real
learning weight per `ADR-011`/`ADR-012`: this algorithmic depth is exactly what gets
hand-ported to Rust, major version by major version, while the TS/HTML/CSS view layer
never moves. A minor that reimplements one of these algorithms twice (once in Go, once in
TS "for the demo") has done the porting work backwards — write it once, in Go, and let the
browser call it.

Scope is drawn from `ROADMAP.md`'s Tracks 2–5 (the full original 21-phase design),
**re-cut into shippable, browser-usable slices** instead of that document's Rust/DSA-
density ordering, and adapted to this repo's actual Go+TS stack (no `tonic`/`tantivy`/
`wasmtime`/JetStream — see each minor's own "stack" note where a substitution matters).
Track 6 (cloud hardening) is **not** its own major or minor — per `ROADMAP.md`'s own
words, it was "never a track at the end"; it stays continuous, patch-level work woven
into whichever minor needs it. Phases 9 (API Gateway hardening) and 10 (session routing)
are the same kind of thing — infrastructure/reliability depth with no browser-visible
feature of its own — and are treated the same way, not given their own minor.

**The major boundaries below aren't arbitrary** — they were checked against
`ROADMAP.md`'s own dependency ordering (its Track 4 intro: "comments need anchors,
notifications need comments, publishing needs RBAC, plugins need the diagnostics engine,
and the assistant needs the search index"), and every one of those holds across the
`v2`→`v4` boundary in the right direction: `v2.3.0` (Diagnostics) comes before `v4.2.0`
(Plugins, which needs it); `v2.5.0` (Search) comes before `v4.4.0` (Assistant, which
needs the index); `v3.1.0` (RBAC) comes before `v4.1.0` (Publishing, which needs it);
`v3.2.0` (Comments) comes before `v3.3.0` (Notifications, which needs it), both inside
the same major. `v4.3.0`/`v4.4.0`'s own order (Related Content before Assistant) is
deliberately the other way around from a naive "AI last" reading: the lexical- and
graph-centrality-based ranking in `v4.3.0` needs no embedding index at all, so it ships
first; `v4.4.0`'s embedding index then completes `v4.3.0`'s Discover panel with real
semantic similarity as a staged enhancement, not a second version of the same screen.
The graph-algorithm work this same reasoning applies to even more directly —
`graph.html`/`graph-algorithms.html` in full — is pulled forward much further still, into
`v2.2.0`: highest DSA-learning density per unit of build effort, every row independently
demoable the moment it lands, and no dependency on anything past `v1.0.0`'s own link
graph (`v2.0.0`'s own intro paragraph has the detail). If a future session's own judgment
says a different cut fits the porting goal better once a major is actually underway, that
judgment wins over this table — the table is a plan, not a constraint on it.

**On the full `v3` grammar (RFC-001 §10):** the target is full coverage, not the subset
that was easy. The one carve-out is `Table`/`CommTable` and the four cross-page query
blocks (`TableOfContents`, `FeaturedArticles`, `FeaturedProjects`, `PortfolioProjects`) —
`v4.5.0` below schedules them explicitly rather than leaving them permanently unscoped,
but their first deliverable is the ADR `CLAUDE.md`'s "Still Out" rule already requires:
cross-page aggregation has no owner in this architecture today, and that has to be
resolved in writing before any block-kind work starts, the same discipline as every other
decision in this plan. Every other §10 kind is real product scope inside `v3.4.0`/`v4`.

`CLAUDE.md`'s remaining "Still Out" items (a formula language, a spatial canvas, mobile
apps, and database/rollup semantics beyond what `v4.5.0` resolves) are **not** in this
plan. They still need their own ADR before they're scoped at all — unchanged by `ADR-012`.

Status legend: **planned** (scoped here, no branch yet) · **in progress** (branch open)
· **shipped** (merged to `master`, tagged).

---

## `v2.0.0` — Depth, Confidence & Insight

*Milestone claim: the editor is trustworthy — you can undo, see history, search, and
delete safely, and every algorithm that backs those guarantees is visible, not just
trusted. Track 2, plus the parts of Track 3 that are product depth rather than
distributed-systems scaling.*

**`graph.html`/`graph-algorithms.html` are pulled forward into their own early minor
(`v2.2.0`), not spread across four later ones the way an earlier pass at this table had
them.** Every row on that pair of mockups — components/orphans, three-colour-DFS cycle
detection, shortest path, BFS-as-wavefront, forward reachability/blast radius, Betti
numbers, the Voronoi/Delaunay territory view — is a pure graph algorithm over
`blockproj`'s already-existing `docs.page_links` (shipped in `v1.0.0`), with **no real
dependency** on Diagnostics, Search, or Assistant; the earlier spread across those
phases followed `ROADMAP.md`'s ownership table, not an actual build-order constraint.
Consolidating it earlier is a deliberate reprioritization (highest DSA-learning density
per unit of build effort, and every one of these algorithms is independently demoable
the moment it lands — faster payoff than waiting on the features it used to ride along
with). `v2.6.0` (Page-Delete Saga) now consumes `v2.2.0`'s own reachability computation
rather than building it a second time.

| Version | Feature | Ships | Source | Status |
|---|---|---|---|---|
| `v2.1.0` | **Undo / Redo** | Per-actor undo/redo across collaborative edits — a keyboard shortcut and a visible undo stack, correct even when someone else edited the same page in between. `documentcore.History` (already built, Track 1) is the primitive; this wires it through `collaboration-service`'s session and a real UI affordance. **Shipped** (`b9c61fe`, `15335c8`). | Phase 5 | shipped |
| `v2.2.0` | **Graph Explorer** | `graph.html`/`graph-algorithms.html`, made real, in full, over the real `[[link]]` graph: connected components and orphan detection (flood fill), cycle detection (three-colour DFS — a visited set alone answers "seen before," not "on the current path"), shortest path and link distance (BFS), BFS rendered as an animated wavefront with per-level frontier widths, forward reachability/blast radius, Betti numbers (β₁ from the GF(2) rank of the triangle boundary map, β₂ from the Euler characteristic), and the graph's exact Voronoi territory view (half-plane intersection, Delaunay dual read back off the cells) plus a seeded force simulation that cools to a stop and reheats on drag. **Shipped** (`98d20ef`, `c76dace`, `215efa4`, `2d098eb`, `5b7f4d0`, `3c8851d`, `2429d40`, `59408b2`, `a7f9694`). | Phases 4, 7, 8, 21 (the graph-only rows) | shipped |
| `v2.3.0` | **Diagnostics & the fact graph** | RFC-003's analyzer engine — all nine §2 analyzers, resolved against a real cross-page symbol table (`GraphService.GetLinkGraph`), computed fresh per request rather than RFC-003 §4's own incremental re-analysis (a stated, honest scope cut at this repo's demo scale, not silently dropped) — surfaced as real left-gutter markers in the editor (dotted amber, RFC-003 §2, never a red squiggle) and a real `InspectorRail` "Checks" tab. Also makes `facts.html` real: named definitions and transclusion backed by a genuine dependency DAG with topological dirty-propagation (`StaleReferences` walks forward from a changed definition), cycle rejection by three-colour DFS, and duplicate-definition detection by hash collision — a second, product-coupled graph (definitions, not pages), reusing `v2.2.0`'s own `graphalgo.DetectCycle`/`ForwardReachable` unchanged. New service: `diagnostics-service` (stateless, a `document-service` gRPC client — RFC-003 §5's own degradation argument justifies the separate deployable). **Shipped** (`a390bdc`, `f2eac11`, `d089b1e`, `eb00ce7`, `a8905a0`, `395b045`, `4ddb2eb`). | Phase 4 | shipped |
| `v2.4.0` | **History, Trace & Diff** | One `InspectorRail`/nav-reachable "History" feature, not three disconnected pages — `history.html`, `trace.html`, and `diff.html` all belong to it and ship together. Event-sourced replay of `collab.ops` into a scrubbable version-history timeline (`Session.Trace`), restore-to-a-point (`Session.RestoreTo` — repeated undo through `Trace`'s own precomputed inverses, never a snapshot swap), and `history.html`'s own palimpsest mode (one block's real tombstoned character array, `internal/palimpsest`, a second parallel replay scoped to one block — neither `doctext.Text` nor `anchor.Log` keeps a deleted character's rune or deleter for free). The op-log debugger (`trace.html`: every `apply`/`invert` runs for real, the invertibility law re-checked per step, not asserted — backend landed during `v2.1.0`'s branch, UI ships here). The revision diff (`diff.html`: real LCS — the DP table and its traceback, `marginal/textdiff`, compiled to wasm for the word/character granularity toggle — plus block-move detection, a plain filter over the op log's own `MoveBlock` ops, no heuristic). `InspectorRail`'s "History" tab is a launcher into all three screens now. **Shipped** (`4c80d9a`, `a2fdc2e`, `8f60ee9`, `a594646`, `82a5401`, `f69c644`). | Phase 6 | shipped |
| `v2.5.0` | **Search & Backlinks** | Real full-text search across pages (Postgres FTS, in place of Tantivy — an in-process, embeddable-index choice, not a new service), BK-tree fuzzy title matching for "did you mean," and `[[link]]` autocomplete via a real prefix trie while typing (wasm-compiled, same interactivity reasoning as the graph/diff wasm bridges). `search.html` becomes real. Backlinks already exist (`blockproj`'s `docs.page_links`); this adds the search surface — `graph.html`'s own shortest-path/wavefront rows already shipped in `v2.2.0`. **Shipped** (`8404cd1`, `5301708`, `3b63d6a`, `5311f99`). | Phase 7 | shipped |
| `v2.6.0` | **Page-Delete Saga** | Safe, resumable cascading delete — a crash mid-delete resumes instead of leaving orphaned descendants; idempotent, with a real "deleting…" state visible in the page tree. Consumes `v2.2.0`'s own forward-reachability computation to know what a delete will actually take with it, rather than building it a second time. `§ 23c` (trash, restore, the saga's own stage list) is real. **Shipped** (`b2142c4`). | Phase 8 | shipped |

---

## `v3.0.0` — Multi-User Platform

*Milestone claim: Marginal stops being one shared workspace and becomes a real
multi-user platform — many people, spaces, permissions, discussion, a finished
editor, and an admin surface that can actually see what the system is doing.
Still one self-hosted instance, one database — not multi-tenancy (`ADR-001`
cuts that; `CLAUDE.md`'s hosted-tier note is unchanged: hosted means one
single-tenant deployment per customer, and shared tenancy needs its own ADR
first, `tenant_id`-on-every-table work `v3.1.0` deliberately does not do).*

| Version | Feature | Ships | Source | Status |
|---|---|---|---|---|
| `v3.1.0` | **Identity, Spaces & RBAC** | Users beyond one shared pool, spaces, roles, invitations, and real permission enforcement inside `can_apply(op, actor)` — no second authorization path. A workspace switcher and a role-management screen — `admin.html`'s people/spaces/roles row becomes real. Every change here gets `/security-review` before merge (this is the phase where a bug is a breach). | Phase 13 | planned |
| `v3.2.0` | **Comments & Reactions** | Anchored comment threads that survive concurrent edits (the same `Anchor`/`AnchorRange` machinery RFC-002 already built for marks), @mentions, reaction pills. `InspectorRail`'s "Comments" tab and `reader.html`'s reactions/comment-thread row stop being an honest empty state. | Phase 14 | planned |
| `v3.3.0` | **Notifications** | The feature, not the skeleton `notification-service` already is — per-user preferences, a notification inbox/bell UI, digesting instead of one-event-one-row. | Phase 15 | **inbox landed early** — see below |
| `v3.4.0` | **Full Editor: remaining block kinds** | RFC-001 §10's still-undone *static* slices: the `List`/`ListItem`-shaped structured collections (`Timeline`, `Grid`, `Tabs`, `Accordion`, `ServiceCards`, `SignalList`, `Stack`, `MetaPills`, `FooterLinks`, `UsesSection`), plus `SyncedBlock`, `ColumnList`, and the `Diagram`/`Mermaid`/`Equation` family — each its own design pass per §10.4's own ledger. Continues directly from `v1.0.0`'s post-ship nesting work (`Callout`/`Aside`/`List`/`Toggle`/`Image`, already shipped on `master`). The cross-page query kinds (`Table`/`CommTable` and friends) are deliberately *not* here — see `v4.5.0`. | Phase 16 | planned |
| `v3.5.0` | **Settings, Admin & Observability Console** | Instance/space/user settings, feature flags, theme families (`settings.html`) — needed for RBAC (`v3.1.0`) and spaces to feel like a finished product. `admin.html`'s health/outbox-depth/op-log-lag/backups rows become real (Phases 11, 12). Also where `netcode.html` and `perf.html` land as a real debug console: `netcode.html`'s four lenses (prediction/rollback and the transform, over both text and block ops; a real AHU/Merkle subtree comparison against the server's confirmed tree; the op DAG with its longest causal chain by DP; the op log rendered LSM-style — memtable, levels, compaction) surfacing `collaboration-service`'s actual live OT engine (already real since `v1.0.0`) rather than re-deriving it; `perf.html`'s latency percentiles, queue depth, and a real scan benchmark run against this instance. | Phases 11, 12, 20 | planned |

---

## `v4.0.0` — Reach & Intelligence

*Milestone claim: Marginal can be published to, extended by, and reasoned about by
something other than the person typing — the most ambitious and most differentiated
remaining work.*

| Version | Feature | Ships | Source | Status |
|---|---|---|---|---|
| `v4.1.0` | **Publishing, Distribution & Workspace Analytics** | Public pages via static pre-render, a feed, a sitemap — a page becomes a thing you can share outside the login wall (`home.html`'s pitch, `reader.html`'s published badge). `analytics.html` becomes real: workspace analytics where the sketches *are* the privacy mechanism — HyperLogLog, Count-Min Sketch, and a t-digest, each computed live over the real event stream and shown beside its exact answer and error, not a canned example. | Phase 17 | **analytics landed early** — see below |
| `v4.2.0` | **Plugins & Extensibility** | A real plugin surface: custom block kinds and custom diagnostic analyzers, sandboxed. Go substitution note: `wazero` (a pure-Go WebAssembly runtime, no cgo) stands in for `wasmtime` — same component-model/capability-based-security shape RFC-005 (once written) should specify, different host embedding. | Phase 18 | planned |
| `v4.3.0` | **Related Content** | A Discover panel ranking related pages by what's available *without* an embedding index yet — lexical similarity (SimHash+LSH) and graph centrality (PageRank, over the same link graph `v2.2.0`'s Graph Explorer already made real) — plus the similar-block hint, merge assistant, bridge suggestions, reading order, and minimum reading set that `ROADMAP.md` § Not Drawn Yet lists against this same phase. `graph.html`'s own rows (Betti numbers, Voronoi territory) already shipped in `v2.2.0`. Semantic similarity is layered into this same panel once `v4.4.0` ships its embedding index — staged, not blocked on it. | Phase 21 | planned |
| `v4.4.0` | **Assistant & Semantic Search** | An assistant that edits by emitting real `Op`s (never raw text — per-actor undo, collaboration, and audit all come free that way), backed by one embedding index (`pgvector`) that also completes `v4.3.0`'s Discover panel with real semantic similarity. Local embeddings (no required API key) for the self-hosted build. `discover.html` becomes fully real: an HNSW index actually built and queried in Go — layer assignment, greedy descent, `Mmax` pruning — with recall@5 measured live against brute force, not asserted. | Phase 19 | **HNSW/Discover landed early** — see below |
| `v4.5.0` | **Structured & Query Blocks** | `Table`/`CommTable` and the cross-page query kinds (`TableOfContents`, `FeaturedArticles`, `FeaturedProjects`, `PortfolioProjects`) — the last of RFC-001 §10's v3 grammar. First deliverable is the ADR this repo's own "Still Out" rule requires: fixed row/cell arity under concurrent edits, and exactly where cross-page aggregation gets an owner without becoming the "databases/rollups" boundary `DATA_MODEL.md` already draws. Only after that ADR lands does block-kind work start. | RFC-001 §10.4 (documented-but-not-recommended, pending this ADR) | planned |

---

## Landed early, out of order

Three later minors have pieces already on `master`, built during `v2.6.0`'s
branch because the mockup set is the acceptance bar (`CLAUDE.md`) and these
screens were the ones a sweep of it found unbuilt. The *rest* of each minor is
still ahead; what shipped is listed here so the table above is not quietly
wrong.

| From | What is real now | What is still that minor's job |
|---|---|---|
| `v4.1.0` | **`§ 12` ANALYTICS** — `marginal/sketch`: HyperLogLog (64 registers, linear-counting small-range correction), a 4×24 Count-Min, and a t-digest, each shown beside the exact answer and its own error bound. Compiled to wasm; the stream is an editable buffer that recomputes on every keystroke, so a duplicated actor visibly fails to move the cardinality estimate. | Publishing itself — static pre-render, the feed, the sitemap, the published badge. And a real event stream: § 12 reads a text box today, not `docs.page_views`, because nothing writes page views yet. |
| `v4.4.0` | **`§ 09` DISCOVER** — `marginal/semantic`: hashed IDF-weighted TF vectors and a real HNSW (layer assignment, greedy descent, heuristic pruning, filter-during-descent), with recall@5 measured against a brute-force scan on every query. | The assistant, `pgvector`, and actual learned embeddings. The screen states plainly that there is no model in this repo — the vectors are lexical. |
| `v3.5.0` | **`§ 16` PERF** — `marginal/bench`: a clock-resolution probe, batch calibration (the same trick `testing.B` uses, and for the same reason — a browser's clock is coarser than the work), log-spaced buckets, nearest-rank percentiles, and a flame graph walked from instrumented spans. Its four workloads are real paths (`Page.Apply`, `mdc.Compile`, `netsim.Run`, the search vector), so the numbers are about this codebase. QUEUE DEPTH reads a real `GET /collab/stats`. | The rest of the observability console, and the settings/admin surfaces around it. |
| `v3.5.0` | **`§ 14` NETCODE** — `marginal/netsim`: TP1 transform, a two-replica prediction/rollback simulation over a seeded lossy wire, an AHU-style Merkle comparison, the causal DAG with its longest chain by DP, and the log drawn as LSM levels. The wire is a set of sliders and the edit script a textarea, so the transform-off argument is one click away rather than a paragraph. | `§ 16` PERF, and the rest of the settings/admin console. § 14 is a *simulation* — the observability console is about this instance, measured. |
| `v3.3.0` | **`§ 20` inbox + `§ 24c` panel** — `notification-service`'s `GET /notifications` behind a real bell, panel and inbox screen. | Per-user preferences, digesting, and every topic that needs a feature `v3.1.0`/`v3.2.0` has not built yet (mentions, comments, sharing). |

---

## Beyond `v4.0.0`, not yet scoped

- **Session routing** (Phase 10, multi-instance `collaboration-service` with consistent
  hashing) and **API Gateway hardening** (Phase 9, RS256 verification/rate limiting/
  circuit breaker) — real work, but infrastructure/reliability depth with no browser-
  visible feature of its own; picked up as continuous, patch-level work per `ADR-012`,
  not a dedicated minor.
- A structural query language over the block tree (`ROADMAP.md` § Deferred pending an
  ADR) — same cross-page-aggregation ownership question as `v4.5.0`, recorded there but
  not committed to a version.
- Formula language, spatial canvas, mobile apps — `CLAUDE.md`'s "Still Out," unchanged.
