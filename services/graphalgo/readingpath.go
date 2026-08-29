package graphalgo

import "sort"

// PathStep is one page on the way to another, with why it is there.
type PathStep struct {
	ID NodeID `json:"id"`
	// Depth is how many links deep this page sits in the prerequisite set —
	// its dependency LAYER, not its distance from the target. Two pages at
	// the same depth can be read in either order.
	Depth int `json:"depth"`
	// Destination marks the page the path was computed for. It is always the
	// last step: the list is what to read UP TO it, so ending anywhere else
	// would leave the reader wondering where they were going.
	Destination bool `json:"destination"`
}

// ReadingPath answers "what should I read before this page, and in what
// order" over the real link graph.
//
// THE DEFINITION, because there are several plausible ones and they give
// different answers. A prerequisite of P is a page that reaches P by
// following links FORWARD — if A links to B and B links to P, then reading A
// then B then P follows the author's own arrows. So:
//
//  1. Reverse the graph and walk forward from P. That set is everything that
//     can reach P: the prerequisites.
//  2. Order that set by dependency LAYER in the original direction, so a page
//     with no prerequisites of its own comes first.
//  3. Append P.
//
// Note what this is NOT. It is not the shortest path (that answers "how are
// these two connected", which is a different question and already has a
// function). It is not "everything tagged similarly" — shared tags give you
// neighbours, not prerequisites, and a reader asking what to read first is
// asking about order, which only the arrows carry.
//
// A page nothing links to has a path of one step: itself. That is a real and
// common answer — it is what "start here" looks like — and returning an empty
// list instead would make the caller invent the same special case.
//
// Ties inside a layer break on node id, so the path is stable across requests
// (I0.6): a reading order that reshuffles between two loads is one nobody can
// link to or follow.
func ReadingPath(g Graph, target NodeID) []PathStep {
	reversed := Graph{Nodes: g.Nodes, Edges: make([]Edge, len(g.Edges))}
	for i, e := range g.Edges {
		reversed.Edges[i] = Edge{From: e.To, To: e.From}
	}
	// Forward-reachable in the reversed graph = everything that reaches
	// target in the original one.
	prereq := ForwardReachable(reversed, target)

	// The induced subgraph over the prerequisites, in the ORIGINAL direction —
	// which is what makes the layering an order to read in rather than an
	// order to have arrived from.
	sub := Graph{}
	for n := range prereq {
		if n == target {
			continue
		}
		sub.Nodes = append(sub.Nodes, n)
	}
	sort.Slice(sub.Nodes, func(i, j int) bool { return sub.Nodes[i] < sub.Nodes[j] })
	in := make(map[NodeID]bool, len(sub.Nodes))
	for _, n := range sub.Nodes {
		in[n] = true
	}
	for _, e := range g.Edges {
		if in[e.From] && in[e.To] {
			sub.Edges = append(sub.Edges, e)
		}
	}

	order, _ := TopologicalSort(sub)
	layers := Layers(sub, order)

	out := make([]PathStep, 0, len(sub.Nodes)+1)
	for depth, level := range layers {
		for _, n := range level {
			out = append(out, PathStep{ID: n, Depth: depth})
		}
	}
	// Anything TopologicalSort could not place (inside a citation loop) still
	// belongs on the path — dropping it would silently shorten the list
	// exactly where the graph is most tangled. It goes last among the
	// prerequisites, at the deepest layer.
	deepest := len(layers)
	for _, n := range Unplaced(sub, order) {
		out = append(out, PathStep{ID: n, Depth: deepest})
	}

	return append(out, PathStep{ID: target, Depth: deepest + 1, Destination: true})
}
