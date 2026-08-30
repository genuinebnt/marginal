package netsim

// DAGNode is one op in § 14's CAUSALITY · DAG.
type DAGNode struct {
	ID    string   `json:"id"`
	Actor string   `json:"actor"`
	Label string   `json:"label"`
	Deps  []string `json:"deps,omitempty"`
	// Depth is the longest causal distance from a root — the layer
	// the node is drawn on. Two ops on the same layer are concurrent
	// as far as the log can tell, which is exactly what makes them
	// each other's transform problem.
	Depth int `json:"depth"`
	// OnLongest marks the critical path.
	OnLongest bool `json:"on_longest"`
}

// DAGView is the graph plus the one number worth reading off it.
type DAGView struct {
	Nodes []DAGNode `json:"nodes"`
	// LongestChain is the longest causal chain: the minimum number of
	// sequential round trips this session could NOT have avoided.
	// Everything off it happened concurrently.
	LongestChain int `json:"longest_chain"`
	// Concurrent is how many ops share a layer with another — the
	// work OT had to do rather than serialise.
	Concurrent int `json:"concurrent"`
	Width      int `json:"width"`
}

// BuildDAG lays the confirmed log out by causal depth.
//
// Longest path in a DAG is the DP over a topological order, and the
// log is already in one — the server assigned it. That is the whole
// reason an op log is worth having: the ordering problem was solved
// once, on write, and every reader gets it for free.
func BuildDAG(log []Op) DAGView {
	view := DAGView{Nodes: []DAGNode{}}
	depth := map[string]int{}
	best := map[string]string{} // node → the dep that gave it its depth

	for _, op := range log {
		d, from := 0, ""
		for _, dep := range op.Deps {
			if depth[dep]+1 > d {
				d, from = depth[dep]+1, dep
			}
		}
		depth[op.ID] = d
		best[op.ID] = from
		view.Nodes = append(view.Nodes, DAGNode{
			ID: op.ID, Actor: op.Actor, Label: op.String(),
			Deps: op.Deps, Depth: d,
		})
	}

	perLayer := map[int]int{}
	deepest, tail := -1, ""
	for _, n := range view.Nodes {
		perLayer[n.Depth]++
		if n.Depth > deepest {
			deepest, tail = n.Depth, n.ID
		}
	}
	for _, c := range perLayer {
		if c > view.Width {
			view.Width = c
		}
		if c > 1 {
			view.Concurrent += c
		}
	}

	idx := map[string]int{}
	for i, n := range view.Nodes {
		idx[n.ID] = i
	}
	for id := tail; id != ""; id = best[id] {
		if i, ok := idx[id]; ok {
			view.Nodes[i].OnLongest = true
			view.LongestChain++
		}
	}
	return view
}
