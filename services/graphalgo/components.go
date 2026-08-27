package graphalgo

import "sort"

// Components assigns every node a component id (small non-negative
// integers, first-discovered order — deterministic given a deterministic
// g.Nodes order) via flood fill (BFS) over the UNDIRECTED graph: two
// pages share a component if a chain of links connects them in either
// direction. graph-algorithms.html's own "connected components by flood
// fill" row.
func Components(g Graph) map[NodeID]int {
	adj := buildAdjacency(g)
	comp := make(map[NodeID]int, len(g.Nodes))
	next := 0
	for _, n := range g.Nodes {
		if _, seen := comp[n]; seen {
			continue
		}
		floodFill(adj.undirected, n, next, comp)
		next++
	}
	return comp
}

func floodFill(undirected map[NodeID][]NodeID, start NodeID, id int, comp map[NodeID]int) {
	queue := []NodeID{start}
	comp[start] = id
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range undirected[cur] {
			if _, seen := comp[nb]; !seen {
				comp[nb] = id
				queue = append(queue, nb)
			}
		}
	}
}

// Orphans returns every component id that contains none of roots, sorted
// — graph-algorithms.html's own definition: "OrphanPage is a connected
// components question, not backlinks == 0 — a mutually-linked pair with
// nothing pointing in is still orphaned." Two pages that only link to
// each other each have a nonzero backlink count, so a naive per-page
// check misses them; the pair's whole component is unreachable from any
// root all the same, exactly like a single unlinked page would be. roots
// is the caller's own notion of "reachable from the start" — in this
// service, every top-level page (no parent) in the page tree, since
// that's what a person can actually navigate to without already knowing
// a page exists.
//
// A component can never share an edge with a different component (that
// would make them the same component by construction) — this is why
// orphan status has to be checked against an externally supplied root
// set, not derived from the edges alone.
func Orphans(comp map[NodeID]int, roots []NodeID) []int {
	rootComponents := make(map[int]bool, len(roots))
	for _, r := range roots {
		if id, ok := comp[r]; ok {
			rootComponents[id] = true
		}
	}

	seen := make(map[int]bool)
	var orphans []int
	for _, id := range comp {
		if seen[id] {
			continue
		}
		seen[id] = true
		if !rootComponents[id] {
			orphans = append(orphans, id)
		}
	}
	sort.Ints(orphans)
	return orphans
}
