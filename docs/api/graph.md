# API — Graph

**Status:** Implemented in Go (`services/document-service/internal/graph`,
`internal/graphalgo`) — `GetLinkGraph`, `AnalyzeGraph`, `GraphNeighborhood`,
all three read-only and computed fresh per request. Backs
`docs/ui-mockups/v2/index.html § 07 GRAPH`/`graph-algorithms.html` (`v2.2.0`,
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
- `betti` — graph-algorithms.html's own topology panel
  (`internal/graphalgo.Betti`): `b0`/`b1` are properties of the graph
  itself (components; cycle rank `E − V + b0`, an exact independent-loop
  count). `b1_clique`/`b2`/`chi`/`triangles`/`rank2` are properties of a
  **chosen** complex — filling every 3-clique (three mutually-citing
  pages) as a 2-simplex is a modelling decision, not a graph fact:
  `rank2` is the GF(2) rank of the triangle boundary map (how many
  independent loops those filled triangles kill), `b1_clique` is what
  survives (`b1 − rank2`), and `b2` (enclosed voids — a hollow
  tetrahedron, four pages whose every triangle is filled but whose
  interior isn't) comes free from the Euler characteristic
  (`chi − b0 + b1_clique`), no second elimination pass needed.

- `strongly_connected` / `scc_sizes` — Tarjan over the **directed** graph
  (`graphalgo.StronglyConnected`). Deliberately *not* the same thing as
  `component_of`: that one ignores which way a link points and answers
  "can I get there at all"; this answers "can I get there **and back**,
  following links as written". A component of size 1 is the normal case
  and means nothing on its own; a component of size > 1 is a set of pages
  citing each other in a loop. Component ids are renumbered so the one
  holding the smallest page id is `0` — Tarjan's own discovery order
  depends on where the outer loop started, and an index that moves
  between requests is an index nothing can be coloured by.
- `topological_order` / `is_dag` / `unplaced` / `layers` — Kahn
  (`graphalgo.TopologicalSort`): a reading order in which no page comes
  before one it links to. Ties break on page id, so the order is stable
  across requests.
  **`topological_order` is PARTIAL when `is_dag` is false** — it holds
  every page that could be placed before the algorithm ran out of
  prerequisite-free nodes, and `unplaced` holds the rest (every page in
  or downstream of a cycle). That partial result is the useful half of
  the failure: "these 40 are orderable, these 6 are tangled" is
  actionable where a bare error is not. Note the three cycle-related
  fields answer three different questions — `cycle` points at *one* loop
  as a path, `strongly_connected` splits *all* of them into sets, and
  `unplaced` *measures* the damage.
  `layers` groups the order into dependency **levels**: everything in one
  level can be read in any order, or by two people at once, and the
  number of levels is the longest dependency chain in the workspace.

**`GraphNeighborhood`** — the one parameterized view, from one chosen
`source_page_id`:
- `undirected_distance` — BFS link-distance (a link's existence, not its
  direction) from source. Grouping this by distance value is what a
  client animates as "BFS as a wavefront" — no separate algorithm needed
  for the frontier levels.
- `forward_reachable` — directed BFS, outbound links only: the blast
  radius a cascading delete starting at source would take with it
  (`v2.6.0`'s Page-Delete Saga consumes this same computation).
- `nearest` — the ranked ring around source, nearest first by hop
  distance, ties broken on page id (`graphalgo.NearestNeighbours`, capped
  at 12). Titles are joined server-side: a "nearest pages" list shipping
  bare ids forces every caller to re-fetch the whole graph to render one
  panel. **Near by LINKS** — deliberately a different question from near
  by *meaning*, which `/discover` answers with cosine distance over
  embeddings, and from near in *space*, which § 07's SPACE lens answers
  over the drawn layout's Delaunay dual. The gap between those three
  answers is the finding: a page about the same thing you have never
  cited is near by meaning and far by links.
- `ring_sizes` — `ring_sizes[d]` is how many pages sit exactly `d` hops
  out, from `d = 0` (the source itself). The shape is the argument: a
  frontier that stops growing is a graph that stops connecting.

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
  "diameter": 4,
  "betti": { "b0": 1, "b1": 3, "b1_clique": 0, "b2": 1, "chi": 2, "triangles": 4, "rank2": 3 },
  "strongly_connected": { "<page-id>": 0 },
  "scc_sizes": [3, 1, 1],
  "topological_order": ["<page-id>", "<page-id>"],
  "is_dag": true,
  "unplaced": [],
  "layers": [["<page-id>"], ["<page-id>", "<page-id>"]]
}

// GET /graph/neighborhood/{id}
{
  "undirected_distance": { "<page-id>": 0, "<page-id>": 1 },
  "forward_reachable": { "<page-id>": 0 },
  "nearest": [{ "page_id": "...", "title": "Anchors vs offsets", "hops": 1 }],
  "ring_sizes": [1, 3, 7]
}
```

`cycle`/`orphan_components`/`scc_sizes`/`topological_order`/`unplaced`/
`layers`/`nearest`/`ring_sizes` are always `[]`, never `null`, when
there's nothing to report — a client that only ever checks `.length`
shouldn't also need a null guard. `strongly_connected` is likewise `{}`
rather than `null`.

Auth follows every other endpoint in this repo's current scope: `X-Actor-Id`
header (or `actor_id` query param), unauthenticated — the same temporary
stand-in `pages.md`/`collaboration.md` already document, not a real trust
boundary yet.
