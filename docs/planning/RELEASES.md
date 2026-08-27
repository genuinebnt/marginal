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
`v2`→`v4` boundary in the right direction: `v2.2.0` (Diagnostics) comes before `v4.2.0`
(Plugins, which needs it); `v2.4.0` (Search) comes before `v4.4.0` (Assistant, which
needs the index); `v3.1.0` (RBAC) comes before `v4.1.0` (Publishing, which needs it);
`v3.2.0` (Comments) comes before `v3.3.0` (Notifications, which needs it), both inside
the same major. `v4.3.0`/`v4.4.0`'s own order (Graph Explorer & Related Content before
Assistant) is deliberately the other way around from a naive "AI last" reading: the
graph-algorithm and lexical-similarity work in `v4.3.0` needs no embedding index at all,
so it ships first; `v4.4.0`'s embedding index then completes `v4.3.0`'s Discover panel
with real semantic similarity as a staged enhancement, not a second version of the same
screen. If a future session's own judgment says a different cut fits the porting goal
better once a major is actually underway, that judgment wins over this table — the
table is a plan, not a constraint on it.

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

| Version | Feature | Ships | Source | Status |
|---|---|---|---|---|
| `v2.1.0` | **Undo / Redo** | Per-actor undo/redo across collaborative edits — a keyboard shortcut and a visible undo stack, correct even when someone else edited the same page in between. `documentcore.History` (already built, Track 1) is the primitive; this wires it through `collaboration-service`'s session and a real UI affordance. **Shipped** (`b9c61fe`, `15335c8`). | Phase 5 | shipped |
| `v2.2.0` | **Diagnostics & the fact graph** | RFC-003's analyzer engine: a symbol table, a reverse index (page-rename invalidates referrers), incremental re-analysis, and real inline squiggles/quick-fixes in the editor — `InspectorRail`'s "Checks" tab stops being an honest empty state. Also makes `facts.html` real: named definitions and transclusion backed by a genuine dependency DAG with topological dirty-propagation (editing a definition marks only what's transitively downstream), cycle rejection by three-colour DFS, and duplicate-definition detection by hash collision — plus the graph's own components/orphans/cycles view in `graph.html`/`graph-algorithms.html` (that pair's Phase-4-owned rows). | Phase 4 | planned |
| `v2.3.0` | **History, Trace & Diff** | One `InspectorRail`/nav-reachable "History" feature, not three disconnected pages — `history.html`, `trace.html`, and `diff.html` all belong to it and ship together, matching `trace.html`'s own nav (its crumb reads "Product · Op trace," linked from History). Event-sourced replay of `collab.ops` into a scrubbable version-history timeline, snapshots for performance, restore-to-a-point (`history.html`: one paragraph backed by a genuine tombstoned persistent sequence, palimpsest mode reading the tombstones the live text is filtered from). The op-log debugger (`trace.html`: every `apply`/`invert` runs for real, `apply(invert(op), apply(op, doc)) == doc` re-checked on every step, not asserted — backend already real and tested, `internal/session.Trace` + `GET /collab/pages/{id}/trace`, landed during `v2.1.0`'s branch as reusable op-log infrastructure; its UI ships here). The revision diff (`diff.html`: the actual LCS dynamic-programming table and its traceback between two real revisions). `InspectorRail`'s "History" tab stops being an honest empty state. | Phase 6 | planned |
| `v2.4.0` | **Search, Backlinks & Graph Explorer** | Real full-text search across pages (Postgres FTS or Bleve, in place of Tantivy — an in-process, embeddable-index choice, not a new service), BK-tree fuzzy title matching for "did you mean," and `[[link]]`/`/command` autocomplete via a trie while typing. `search.html` becomes real. Backlinks already exist (`blockproj`'s `docs.page_links`); this adds the search surface. `graph.html`/`graph-algorithms.html` gain their Phase-7-owned rows: shortest path and link distance (real BFS), and BFS rendered as an animated wavefront with per-level frontier widths. | Phase 7 | planned |
| `v2.5.0` | **Page-Delete Saga** | Safe, resumable cascading delete — a crash mid-delete resumes instead of leaving orphaned descendants; idempotent, with a real "deleting…" state visible in the page tree. `graph.html`'s blast-radius/forward-reachability row (Phase 8) becomes real here — it's the same reachability computation the saga itself needs to know what a delete will actually take with it. | Phase 8 | planned |

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
| `v3.3.0` | **Notifications** | The feature, not the skeleton `notification-service` already is — per-user preferences, a notification inbox/bell UI, digesting instead of one-event-one-row. | Phase 15 | planned |
| `v3.4.0` | **Full Editor: remaining block kinds** | RFC-001 §10's still-undone *static* slices: the `List`/`ListItem`-shaped structured collections (`Timeline`, `Grid`, `Tabs`, `Accordion`, `ServiceCards`, `SignalList`, `Stack`, `MetaPills`, `FooterLinks`, `UsesSection`), plus `SyncedBlock`, `ColumnList`, and the `Diagram`/`Mermaid`/`Equation` family — each its own design pass per §10.4's own ledger. Continues directly from `v1.0.0`'s post-ship nesting work (`Callout`/`Aside`/`List`/`Toggle`/`Image`, already shipped on `master`). The cross-page query kinds (`Table`/`CommTable` and friends) are deliberately *not* here — see `v4.5.0`. | Phase 16 | planned |
| `v3.5.0` | **Settings, Admin & Observability Console** | Instance/space/user settings, feature flags, theme families (`settings.html`) — needed for RBAC (`v3.1.0`) and spaces to feel like a finished product. `admin.html`'s health/outbox-depth/op-log-lag/backups rows become real (Phases 11, 12). Also where `netcode.html` and `perf.html` land as a real debug console: `netcode.html`'s four lenses (prediction/rollback and the transform, over both text and block ops; a real AHU/Merkle subtree comparison against the server's confirmed tree; the op DAG with its longest causal chain by DP; the op log rendered LSM-style — memtable, levels, compaction) surfacing `collaboration-service`'s actual live OT engine (already real since `v1.0.0`) rather than re-deriving it; `perf.html`'s latency percentiles, queue depth, and a real scan benchmark run against this instance. | Phases 11, 12, 20 | planned |

---

## `v4.0.0` — Reach & Intelligence

*Milestone claim: Marginal can be published to, extended by, and reasoned about by
something other than the person typing — the most ambitious and most differentiated
remaining work.*

| Version | Feature | Ships | Source | Status |
|---|---|---|---|---|
| `v4.1.0` | **Publishing, Distribution & Workspace Analytics** | Public pages via static pre-render, a feed, a sitemap — a page becomes a thing you can share outside the login wall (`home.html`'s pitch, `reader.html`'s published badge). `analytics.html` becomes real: workspace analytics where the sketches *are* the privacy mechanism — HyperLogLog, Count-Min Sketch, and a t-digest, each computed live over the real event stream and shown beside its exact answer and error, not a canned example. | Phase 17 | planned |
| `v4.2.0` | **Plugins & Extensibility** | A real plugin surface: custom block kinds and custom diagnostic analyzers, sandboxed. Go substitution note: `wazero` (a pure-Go WebAssembly runtime, no cgo) stands in for `wasmtime` — same component-model/capability-based-security shape RFC-005 (once written) should specify, different host embedding. | Phase 18 | planned |
| `v4.3.0` | **Graph Explorer & Related Content** | `graph.html`'s remaining rows become real: Betti numbers (β₁ from the GF(2) rank of the triangle boundary map, β₂ from the Euler characteristic) and the graph's Voronoi territory view (exact Voronoi by half-plane intersection, Delaunay dual read back off the cells). A Discover panel ranking related pages by what's available *without* an embedding index yet — lexical similarity (SimHash+LSH) and graph centrality (PageRank) — plus the similar-block hint, merge assistant, bridge suggestions, reading order, and minimum reading set that `ROADMAP.md` § Not Drawn Yet lists against this same phase. Semantic similarity is layered into this same panel once `v4.4.0` ships its embedding index — staged, not blocked on it. | Phase 21 | planned |
| `v4.4.0` | **Assistant & Semantic Search** | An assistant that edits by emitting real `Op`s (never raw text — per-actor undo, collaboration, and audit all come free that way), backed by one embedding index (`pgvector`) that also completes `v4.3.0`'s Discover panel with real semantic similarity. Local embeddings (no required API key) for the self-hosted build. `discover.html` becomes fully real: an HNSW index actually built and queried in Go — layer assignment, greedy descent, `Mmax` pruning — with recall@5 measured live against brute force, not asserted. | Phase 19 | planned |
| `v4.5.0` | **Structured & Query Blocks** | `Table`/`CommTable` and the cross-page query kinds (`TableOfContents`, `FeaturedArticles`, `FeaturedProjects`, `PortfolioProjects`) — the last of RFC-001 §10's v3 grammar. First deliverable is the ADR this repo's own "Still Out" rule requires: fixed row/cell arity under concurrent edits, and exactly where cross-page aggregation gets an owner without becoming the "databases/rollups" boundary `DATA_MODEL.md` already draws. Only after that ADR lands does block-kind work start. | RFC-001 §10.4 (documented-but-not-recommended, pending this ADR) | planned |

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
