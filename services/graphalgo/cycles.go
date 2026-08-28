package graphalgo

// color is three-colour DFS's own state per node — RFC-less, this is
// plain Skiena-style graph theory, not a Marginal-specific invariant.
type color int

const (
	white color = iota // unvisited
	gray               // on the current DFS path (the call stack), not yet fully explored
	black              // fully explored — every reachable node from here has already been visited
)

// DetectCycle runs three-colour DFS over the DIRECTED graph and returns
// the first cycle found as an ordered node path (closed: first and last
// entries are the same node), or nil if the graph is acyclic.
//
// A plain visited set is the wrong tool here and would produce false
// positives: it only answers "seen before," not "on the current path."
// A diamond — A -> B, A -> C, B -> D, C -> D — visits D twice (once via B,
// once via C) with no cycle anywhere; a visited-set check that treats
// "already seen" as "found a cycle" would wrongly flag it. Three colours
// fix this: D is BLACK (fully explored, off the stack) the second time
// it's reached, not GRAY, so it's correctly not a cycle. A real cycle
// only exists when a DIRECTED edge leads to a node that is currently GRAY
// — still an ancestor on the active path.
func DetectCycle(g Graph) []NodeID {
	adj := buildAdjacency(g)
	colorOf := make(map[NodeID]color, len(g.Nodes))
	var stack []NodeID
	var cycle []NodeID

	var visit func(n NodeID) bool
	visit = func(n NodeID) bool {
		colorOf[n] = gray
		stack = append(stack, n)
		for _, next := range adj.directed[n] {
			switch colorOf[next] {
			case white:
				if visit(next) {
					return true
				}
			case gray:
				cycle = closedCycleFrom(stack, next)
				return true
			case black:
				// Fully explored on an earlier, unrelated branch — not on
				// the current path, so this is not a cycle.
			}
		}
		stack = stack[:len(stack)-1]
		colorOf[n] = black
		return false
	}

	for _, n := range g.Nodes {
		if colorOf[n] == white {
			if visit(n) {
				return cycle
			}
		}
	}
	return nil
}

// closedCycleFrom builds the cycle path once a back edge to target (still
// GRAY, hence still on stack) is found: everything from target's own
// position through the top of the stack, then target again to close it.
func closedCycleFrom(stack []NodeID, target NodeID) []NodeID {
	for i, n := range stack {
		if n == target {
			cycle := make([]NodeID, 0, len(stack)-i+1)
			cycle = append(cycle, stack[i:]...)
			cycle = append(cycle, target)
			return cycle
		}
	}
	return nil // unreachable: target is GRAY, so it must be on stack
}
