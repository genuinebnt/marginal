// Package graphalgo makes docs/ui-mockups/v2/index.html § 08 GRAPH ALGORITHMS real:
// connected components, orphan detection, cycle detection, BFS shortest
// paths (and the per-level frontiers that animate as a "wavefront"),
// forward reachability ("blast radius"), and graph diameter — pure
// functions over an in-memory graph, no I/O, no database. The caller
// (this service's own gRPC handler) builds a Graph from docs.page_links
// and ships whichever result the client asked for; every law here is a
// plain unit test, independent of Postgres.
//
// This is Go, not TypeScript, per ADR-012: the algorithm lives here and
// the browser only draws what this package already computed — never a
// second implementation in JS "for the demo."
package graphalgo

// NodeID identifies one page in the link graph — a page id, as a plain
// string rather than uuid.UUID so this package has zero dependencies of
// its own; the caller converts at the boundary.
type NodeID string

// Edge is a directed page_links row: From links to To. A dangling link
// (target_page is null — the linked page doesn't exist yet) is not
// representable as an Edge; there is nothing on the other end to draw a
// line to or walk toward.
type Edge struct {
	From NodeID
	To   NodeID
}

// Graph is the whole link graph: Nodes is every page, even one with no
// links at all (orphan detection needs to see it sitting alone); Edges is
// every resolved [[link]].
type Graph struct {
	Nodes []NodeID
	Edges []Edge
}

// adjacency is Graph's adjacency-list view, built fresh per algorithm call
// rather than cached on Graph — this package holds no state of its own,
// and a Graph is cheap to rebuild from docs.page_links on every request
// at this repo's demo scale.
type adjacency struct {
	directed   map[NodeID][]NodeID // out-edges only, as given
	undirected map[NodeID][]NodeID // both directions — "a link exists," not "which way it points"
}

func buildAdjacency(g Graph) adjacency {
	adj := adjacency{
		directed:   make(map[NodeID][]NodeID, len(g.Nodes)),
		undirected: make(map[NodeID][]NodeID, len(g.Nodes)),
	}
	for _, n := range g.Nodes {
		adj.directed[n] = nil
		adj.undirected[n] = nil
	}
	for _, e := range g.Edges {
		adj.directed[e.From] = append(adj.directed[e.From], e.To)
		adj.undirected[e.From] = append(adj.undirected[e.From], e.To)
		adj.undirected[e.To] = append(adj.undirected[e.To], e.From)
	}
	return adj
}
