package graphalgo

import "sort"

// Neighbour is one page near another, with the hop distance between them.
type Neighbour struct {
	ID   NodeID `json:"id"`
	Hops int    `json:"hops"`
}

// NearestNeighbours returns the k pages closest to source by LINK DISTANCE —
// breadth-first over the undirected graph, nearest first.
//
// "Near" here means near in the link graph, which is deliberately not what
// /discover will mean by it (cosine distance in an embedding space) and not
// what § 07's SPACE lens means by it (adjacency in the drawn layout). Three
// notions of nearness, and the whole value of having them is that they
// disagree: a page you cite is near by links; a page about the same thing you
// have never cited is near by meaning and far by links, and THAT gap is the
// one worth surfacing.
//
// Ties at the same hop distance break on node id, so the list is stable. BFS
// visits in queue order, which depends on insertion order, and a "nearest 5"
// that reshuffles between runs is a list you cannot trust to be about
// distance at all.
//
// The source itself is never in its own result. k <= 0 returns everything
// reachable, which is how a caller asks for the full ranked ring.
func NearestNeighbours(g Graph, source NodeID, k int) []Neighbour {
	dist, _ := BFS(g, source)

	out := make([]Neighbour, 0, len(dist))
	for id, d := range dist {
		if id == source {
			continue
		}
		out = append(out, Neighbour{ID: id, Hops: d})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Hops != out[j].Hops {
			return out[i].Hops < out[j].Hops
		}
		return out[i].ID < out[j].ID
	})
	if k > 0 && len(out) > k {
		out = out[:k]
	}
	return out
}

// RingSizes counts how many pages sit at each hop distance from source —
// 1 at distance 0 (itself), then the frontier widths BFS passes through.
//
// The shape is the argument, which is why this is a separate function rather
// than something the caller folds out of NearestNeighbours: a frontier that
// stops growing is a graph that stops connecting, and the level at which it
// peaks is how far a page's influence actually reaches before the workspace
// runs out of links to follow.
func RingSizes(g Graph, source NodeID) []int {
	dist, _ := BFS(g, source)
	deepest := 0
	for _, d := range dist {
		if d > deepest {
			deepest = d
		}
	}
	out := make([]int, deepest+1)
	for _, d := range dist {
		out[d]++
	}
	return out
}
