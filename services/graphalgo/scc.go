package graphalgo

import "sort"

// StronglyConnected partitions the DIRECTED graph into strongly connected
// components — Tarjan's algorithm, one DFS, O(V+E).
//
// This is a different question from Components, and the difference is the
// reason both exist. Components asks "can I walk between these two pages if I
// ignore which way the links point" and answers with the shape of the
// workspace. An SCC asks "can I walk from A to B AND back, following links
// the way they are written" — every page in one SCC can reach every other,
// which for a notebook means a set of pages that cite each other in a loop.
//
// An SCC of size 1 is the normal case and is not interesting on its own; an
// SCC of size > 1 is a citation loop, and a loop is the thing that makes an
// ordering impossible (see TopologicalSort, which fails exactly when one
// exists). DetectCycle finds ONE such loop as a path; this finds ALL of them
// as sets, which is what you need to say "these six pages are tangled",
// rather than "here is one lap around the tangle".
//
// The returned map is node -> component index. Indices are assigned so that
// the component containing the lexicographically smallest node id is 0, then
// the next, and so on: Tarjan's own discovery order depends on which node the
// outer loop happens to start from, and a component index that changes
// between two runs over identical input is an index nothing can be coloured
// by.
func StronglyConnected(g Graph) map[NodeID]int {
	adj := buildAdjacency(g)

	// Iterate nodes in a fixed order so the DFS itself is deterministic,
	// before the relabelling below makes the OUTPUT deterministic too.
	nodes := append([]NodeID(nil), g.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })

	var (
		index    = 0
		indexOf  = make(map[NodeID]int, len(nodes))
		lowlink  = make(map[NodeID]int, len(nodes))
		onStack  = make(map[NodeID]bool, len(nodes))
		stack    []NodeID
		assigned = make(map[NodeID]int, len(nodes))
		next     = 0
	)

	// Recursive Tarjan. Recursion depth is bounded by the longest simple
	// path, which at this repo's scale (hundreds of pages) is nowhere near
	// the stack limit; an explicit stack would buy nothing but a harder read.
	var strongConnect func(v NodeID)
	strongConnect = func(v NodeID) {
		indexOf[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		out := append([]NodeID(nil), adj.directed[v]...)
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		for _, w := range out {
			if _, seen := indexOf[w]; !seen {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				// w is on the current stack, so it is in the component being
				// built. Its INDEX, not its lowlink — using lowlink here is
				// the classic bug, and it merges components that only touch.
				if indexOf[w] < lowlink[v] {
					lowlink[v] = indexOf[w]
				}
			}
		}

		if lowlink[v] == indexOf[v] {
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				assigned[w] = next
				if w == v {
					break
				}
			}
			next++
		}
	}

	for _, n := range nodes {
		if _, seen := indexOf[n]; !seen {
			strongConnect(n)
		}
	}

	return relabelBySmallestMember(assigned)
}

// relabelBySmallestMember renumbers component ids so that the component
// holding the smallest node id becomes 0, the next 1, and so on. Without it
// the numbering is Tarjan's discovery order, which depends on where the outer
// loop started — fine for correctness, useless as a colour key.
func relabelBySmallestMember(assigned map[NodeID]int) map[NodeID]int {
	smallest := map[int]NodeID{}
	for node, comp := range assigned {
		if cur, ok := smallest[comp]; !ok || node < cur {
			smallest[comp] = node
		}
	}
	order := make([]int, 0, len(smallest))
	for comp := range smallest {
		order = append(order, comp)
	}
	sort.Slice(order, func(i, j int) bool { return smallest[order[i]] < smallest[order[j]] })
	rank := make(map[int]int, len(order))
	for i, comp := range order {
		rank[comp] = i
	}
	out := make(map[NodeID]int, len(assigned))
	for node, comp := range assigned {
		out[node] = rank[comp]
	}
	return out
}

// SCCSizes counts how many nodes each strongly connected component holds,
// largest first. A workspace where every component has size 1 is a workspace
// with no citation loops at all — which is the healthy default, and worth
// being able to state rather than infer from an empty cycle list.
func SCCSizes(scc map[NodeID]int) []int {
	counts := map[int]int{}
	for _, c := range scc {
		counts[c]++
	}
	out := make([]int, 0, len(counts))
	for _, n := range counts {
		out = append(out, n)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}
