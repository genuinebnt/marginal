# API — Diagnostics

**Status:** Implemented in Go (`services/diagnostics-service/internal/analyzers`,
`internal/facts`) — `AnalyzePage`, `AnalyzeFacts`, `StaleReferences`, all
read-only and computed fresh per request. Backs `docs/ui-mockups/v2/index.html § 10 FACTS`
(`v2.3.0`, `docs/planning/RELEASES.md`) and `InspectorRail`'s "Checks" tab,
which stops being an honest empty state here — every diagnostic is RFC-003
§2's own analyzer table, run for real over the actual block tree and link
graph, never a second implementation in the browser (`ADR-012`).
**Owners:** `diagnostics-service` (gRPC `DiagnosticsService`) · `api-gateway` (REST translation)
**Related:** `docs/architecture/rfc/RFC-003-diagnostics-engine.md` (the analyzer
table, severities, quick-fix shapes) · `docs/planning/ROADMAP.md` § Fact
dependency graph (the facts DAG's own design) · `docs/api/pages.md` §1's
`ListBlocks` (this service's read path onto a page's prose)

Same two-contract shape as `pages.md`/`graph.md`:

```
   browser  ──REST/JSON──▶  api-gateway  ──gRPC/protobuf──▶  diagnostics-service  ──gRPC/protobuf──▶  document-service
            §2 below                      §1 below                                 (PageService, GraphService — read-only)
```

`diagnostics-service` is stateless — no database of its own, no NATS
subscription. It reads document-service's `PageService`/`GraphService` as a
gRPC client (authenticating with its own synthetic actor identity, same
temporary stand-in every other service uses) and computes every result
fresh, per request. This is deliberate, not a missing optimization: RFC-003
§5's whole reason for a separate service is degradation as a transport
property — kill this service's pod, and editing (which never calls it) is
unaffected.

---

## 1. `DiagnosticsService` — the gRPC contract

```protobuf
service DiagnosticsService {
  rpc AnalyzePage(AnalyzePageRequest) returns (AnalyzePageResponse);
  rpc AnalyzeFacts(AnalyzeFactsRequest) returns (AnalyzeFactsResponse);
  rpc StaleReferences(StaleReferencesRequest) returns (StaleReferencesResponse);
}
```

Full message shapes: `services/diagnostics-service/proto/diagnostics.proto`.

**`AnalyzePage`** — every RFC-003 §2 analyzer, run over one page:
`DanglingPageLink`/`AmbiguousPageLink` (one shared resolution pass against
the workspace's symbol table — every page's id/title, sourced from
`GraphService.GetLinkGraph`), `SelfLink`, `LinkCycle` (`graphalgo.DetectCycle`,
unchanged, over that same link graph), `HeadingSkip`, `EmptyCodeBlock`,
`DuplicateTitle` (a title resolving `Ambiguous` against itself),
`OrphanPage` (`graphalgo.Components`/`Orphans`, unchanged — the same
connected-components argument `graph-algorithms.html` already makes over
`backlinks == 0`). Each `Diagnostic.severity` is one of `hint`/`warning`/
`info` — RFC-003 §2's own ceiling, nothing renders as an error; `block_id`
is absent for a page-level diagnostic (`DuplicateTitle`, `OrphanPage`,
`LinkCycle` all describe the page, not one block in it).

Two honest, stated scope cuts from RFC-003's own fuller design, not
silently papered over: results are computed fresh per request rather than
RFC-003 §4's salsa-style incremental memoisation (fast enough in practice
at this repo's demo scale); `BrokenImage` flags only a zero-value
`file_id`, since this repo has no upload/asset pipeline yet to check a
`file_id` against a real files table.

**`AnalyzeFacts`** — scans every page for `{{define name = value}}`
definitions and `{{name}}` references and returns the whole dependency
DAG. This repo's own concretization of RFC-003 §4/`ROADMAP.md`'s "define
a value once, reference it anywhere" — neither doc pins down a literal
syntax, so `internal/facts` picks one: a block whose *entire* text matches
`{{define name = value}}` is a definition; `{{name}}` anywhere else
(including inside another definition's own value) is a reference.
`duplicates` groups every name claimed by more than one definition — "a
hash-lookup collision, not a satisfiability problem" — and `cycle` is one
example cycle of fact names (`"a = {{b}}, b = {{a}}"`, `ROADMAP.md`'s own
example), both excluded from `definitions` rather than resolved
arbitrarily.

**`StaleReferences`** — dirty-mark propagation: every reference that is,
or transitively depends on, `fact_name`, walked forward through the
dependency DAG (`graphalgo.ForwardReachable`, unchanged — the same
"blast radius" computation `v2.6.0`'s Page-Delete Saga will use, applied
here to fact names instead of pages).

---

## 2. REST mapping (`api-gateway`)

| Method | Path | RPC |
|---|---|---|
| `GET` | `/pages/{id}/diagnostics` | `AnalyzePage` |
| `GET` | `/facts` | `AnalyzeFacts` |
| `GET` | `/facts/{name}/stale` | `StaleReferences` |

```json
// GET /pages/{id}/diagnostics
{
  "diagnostics": [
    { "analyzer": "LinkCycle", "severity": "info", "message": "This page is part of a link cycle" },
    { "analyzer": "HeadingSkip", "severity": "hint", "message": "Heading level 3 follows level 1 with none in between", "block_id": "..." }
  ]
}

// GET /facts
{
  "definitions": [{ "name": "ack-budget", "value": "40ms", "page_id": "...", "block_id": "..." }],
  "duplicates": [],
  "cycle": [],
  "references": [{ "name": "ack-budget", "page_id": "...", "block_id": "..." }]
}

// GET /facts/ack-budget/stale
[{ "name": "ack-budget", "page_id": "...", "block_id": "..." }]
```

`diagnostics`/`duplicates`/`cycle`/`references`/`definitions` are always
`[]`, never `null`, when there's nothing to report — a client that only
ever checks `.length` shouldn't also need a null guard.

Auth follows every other endpoint in this repo's current scope:
`X-Actor-Id` header (or `actor_id` query param), unauthenticated — the
same temporary stand-in `pages.md`/`graph.md` already document, not a
real trust boundary yet. `diagnostics-service`'s own outgoing calls to
`document-service` use a separate, synthetic actor identity of their own
(`internal/service.Server.withActor`) — this service reads across the
whole workspace on nobody's specific behalf, the same reason
`collaboration-service`'s `serverActor` isn't a real editing user either.
