package graphalgo

import "sort"

// TopologicalSort orders the DIRECTED graph so every link points forward —
// Kahn's algorithm, O(V+E).
//
// For a notebook this is "a reading order that never asks you to understand a
// page before the page it depends on". It exists only for a DAG, so the
// second return value is not an afterthought: `ok` is false exactly when the
// graph contains a cycle, and the partial order returned alongside is every
// page that COULD be placed before the algorithm ran out of nodes with no
// remaining prerequisites. That partial order is the useful half of a
// failure — it says "these 40 pages are orderable, the other 6 are tangled",
// which is actionable, where a bare error is not.
//
// Ties — several nodes ready at once — break on node id, so the order is
// stable across runs. Kahn's leaves that free, and an unstable reading order
// is one you cannot diff or link to.
//
// Note this is a different failure from DetectCycle's: DetectCycle returns
// one cycle as a path, useful for pointing at the problem; the unplaced set
// here is every node involved in or downstream of any cycle, useful for
// measuring it. StronglyConnected splits that set into the individual
// tangles.
func TopologicalSort(g Graph) (order []NodeID, ok bool) {
	adj := buildAdjacency(g)

	indegree := make(map[NodeID]int, len(g.Nodes))
	for _, n := range g.Nodes {
		indegree[n] = 0
	}
	for _, e := range g.Edges {
		// An edge whose endpoints are not both in Nodes would corrupt the
		// count; Graph's own contract says Nodes holds every page, so this
		// only guards against a malformed caller rather than a real case.
		if _, ok := indegree[e.To]; ok {
			indegree[e.To]++
		}
	}

	ready := make([]NodeID, 0, len(g.Nodes))
	for n, d := range indegree {
		if d == 0 {
			ready = append(ready, n)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })

	order = make([]NodeID, 0, len(g.Nodes))
	for len(ready) > 0 {
		// Pop the smallest ready node. A heap would be the textbook choice;
		// at this repo's scale re-sorting a short slice costs less than the
		// heap does to read.
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)

		out := append([]NodeID(nil), adj.directed[n]...)
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		for _, m := range out {
			indegree[m]--
			if indegree[m] == 0 {
				ready = append(ready, m)
			}
		}
		sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
	}

	return order, len(order) == len(g.Nodes)
}

// Unplaced is every node TopologicalSort could not order — the nodes inside a
// cycle, plus everything downstream of one. Derived from the order rather
// than tracked during it, because the order is the thing worth returning and
// this is a set difference over it.
func Unplaced(g Graph, order []NodeID) []NodeID {
	placed := make(map[NodeID]bool, len(order))
	for _, n := range order {
		placed[n] = true
	}
	out := make([]NodeID, 0, len(g.Nodes)-len(order))
	for _, n := range g.Nodes {
		if !placed[n] {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Layers groups a topological order into dependency LEVELS: level 0 is every
// page with no prerequisites, level 1 is every page whose prerequisites are
// all in level 0, and so on.
//
// The order alone is a line; the levels are the shape. Two pages in the same
// level can be read in either order — or by two people at once — and the
// number of levels is the length of the longest dependency chain, which is
// the real answer to "how deep does this go".
//
// Only meaningful for a DAG. On a cyclic graph it levels whatever
// TopologicalSort managed to place and ignores the rest, matching that
// function's own partial-result contract.
func Layers(g Graph, order []NodeID) [][]NodeID {
	placed := make(map[NodeID]bool, len(order))
	for _, n := range order {
		placed[n] = true
	}
	prereqs := make(map[NodeID][]NodeID, len(order))
	for _, e := range g.Edges {
		if placed[e.From] && placed[e.To] {
			prereqs[e.To] = append(prereqs[e.To], e.From)
		}
	}

	level := make(map[NodeID]int, len(order))
	deepest := 0
	// One pass in topological order is enough: every prerequisite of a node
	// is placed before it, so its level is final by the time it is read.
	for _, n := range order {
		l := 0
		for _, p := range prereqs[n] {
			if level[p]+1 > l {
				l = level[p] + 1
			}
		}
		level[n] = l
		if l > deepest {
			deepest = l
		}
	}

	out := make([][]NodeID, deepest+1)
	for _, n := range order {
		out[level[n]] = append(out[level[n]], n)
	}
	return out
}
