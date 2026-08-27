# API — Graph

**Status:** Implemented in Go (`services/document-service/internal/graph`,
`internal/graphalgo`) — `GetLinkGraph`, `AnalyzeGraph`, `GraphNeighborhood`,
all three read-only and computed fresh per request. Backs
`docs/ui-mockups/graph.html`/`graph-algorithms.html` (`v2.2.0`,
`docs/planning/RELEASES.md`) — every algorithm those mockups draw runs for
real here, over the actual `[[link]]` graph (`docs.pages`/`docs.page_links`),
never a second implementation in the browser (`ADR-012`).
**Owners:** `document-service` (gRPC `GraphService`) · `api-gateway` (REST translation)
**Related:** `internal/graphalgo`'s own doc comments (the algorithms
themselves) · `docs/api/pages.md` (the `ListBacklinks` RPC this
complements — one page's own backlinks vs. the whole graph) · `DATA_MODEL.md`
§4 (`docs.page_links`)

Same two-contract shape as `pages.md`/`auth.md`:

```
   browser  ──REST/JSON──▶  api-gateway  ──gRPC/protobuf──▶  document-service
            §2 below                      §1 below
```

---

## 1. `GraphService` — the gRPC contract

```protobuf
service GraphService {
  rpc GetLinkGraph(GetLinkGraphRequest) returns (LinkGraph);
  rpc AnalyzeGraph(AnalyzeGraphRequest) returns (GraphAnalysis);
  rpc GraphNeighborhood(GraphNeighborhoodRequest) returns (GraphNeighborhoodResponse);
}
```

Full message shapes: `services/document-service/proto/graph.proto`.

**`GetLinkGraph`** — every live page (a `GraphNode`, even one with zero
links — orphan detection needs to see it sitting alone) and every
*resolved* `[[link]]` (a `GraphEdge`). A dangling link (the target page
doesn't exist) is not an edge — nothing on the other end to draw a line
to. `GraphNode.is_root` is `parent_id IS NULL`: a page reachable without
already knowing another page's title, and the exact root set
`AnalyzeGraph`'s own orphan detection uses.

**`AnalyzeGraph`** — every algorithm that needs no parameter, run once
over the whole graph:
- `component_of` — every page id's connected-component id
  (`internal/graphalgo.Components`, flood fill over the *undirected*
  projection — a link's existence matters, not its direction).
- `orphan_components` — component ids containing none of the root pages.
  **Not** `backlinks == 0`: a mutually-linked pair each has a nonzero
  backlink count individually, but the pair's whole component is just as
  unreachable from any root as one unlinked page (`internal/graphalgo`'s
  own doc comment has the full argument).
- `cycle` — one example cycle (three-colour DFS over the *directed*
  graph — a plain visited set would false-positive on a diamond shape),
  as a closed, ordered list of page ids (first == last); empty if acyclic.
- `diameter` — the longest shortest path between any two pages in the
  same component (all-pairs BFS, undirected); disconnected pairs are
  skipped, not treated as infinite.

**`GraphNeighborhood`** — the one parameterized view, from one chosen
`source_page_id`:
- `undirected_distance` — BFS link-distance (a link's existence, not its
  direction) from source. Grouping this by distance value is what a
  client animates as "BFS as a wavefront" — no separate algorithm needed
  for the frontier levels.
- `forward_reachable` — directed BFS, outbound links only: the blast
  radius a cascading delete starting at source would take with it
  (`v2.6.0`'s Page-Delete Saga consumes this same computation).

`NOT_FOUND` if `source_page_id` doesn't name a live page.

---

## 2. REST mapping (`api-gateway`)

| Method | Path | RPC |
|---|---|---|
| `GET` | `/graph` | `GetLinkGraph` |
| `GET` | `/graph/analysis` | `AnalyzeGraph` |
| `GET` | `/graph/neighborhood/{id}` | `GraphNeighborhood` |

```json
// GET /graph
{
  "nodes": [{ "id": "...", "title": "Operation model", "is_root": true }],
  "edges": [{ "from_page": "...", "to_page": "..." }]
}

// GET /graph/analysis
{
  "component_of": { "<page-id>": 0 },
  "orphan_components": [1],
  "cycle": ["<page-id>", "<page-id>", "<page-id>"],
  "diameter": 4
}

// GET /graph/neighborhood/{id}
{
  "undirected_distance": { "<page-id>": 0, "<page-id>": 1 },
  "forward_reachable": { "<page-id>": 0 }
}
```

`cycle`/`orphan_components` are always `[]`, never `null`, when there's
nothing to report — a client that only ever checks `.length` shouldn't
also need a null guard.

Auth follows every other endpoint in this repo's current scope: `X-Actor-Id`
header (or `actor_id` query param), unauthenticated — the same temporary
stand-in `pages.md`/`collaboration.md` already document, not a real trust
boundary yet.
