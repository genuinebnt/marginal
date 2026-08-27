package graphalgo

import "math/big"

// BettiNumbers is graph-algorithms.html's own topology panel, computed
// for real. β₀ and β₁ are properties of the graph alone; β₁ of the clique
// complex, β₂, and everything a triangle touches are properties of a
// CHOSEN complex — filling every triangle (three mutually-citing pages)
// as a 2-simplex is a modelling decision, stated here rather than
// hidden, per the mockup's own argument: "declares that three pages
// citing each other is a local fact rather than a hole. State the choice
// and the numbers mean something; hide it and they are decoration."
type BettiNumbers struct {
	B0        int // components (graphalgo.Components' own count)
	B1        int // cycle rank of the plain graph: E - V + B0, an exact count of independent loops
	B1Clique  int // B1 minus rank(∂₂) — the loops that survive once every triangle is filled
	B2        int // chi - B0 + B1Clique, read off the Euler characteristic for free
	Chi       int // Euler characteristic of the clique complex: V - E + F
	Triangles int // |F| — how many 3-cliques were filled
	Rank2     int // rank(∂₂) over GF(2) — exactly the loops triangles killed
}

// Betti computes every number above over g's undirected projection —
// self-loops and duplicate directed edges between the same pair
// (a page linking to another twice, or both linking to each other)
// collapse to one undirected edge first, the same dedup
// graph-algorithms.html's own UND set performs, since B1/the boundary
// map are about distinct links, not link *rows*.
func Betti(g Graph) BettiNumbers {
	comp := Components(g)
	b0 := countDistinctComponents(comp)

	und := undirectedEdges(g)
	b1 := len(und) - len(g.Nodes) + b0

	tri := triangles(und)
	rank2 := gf2BoundaryRank(und, tri)
	b1Clique := b1 - rank2

	// chi = beta0 - beta1 + beta2 always holds for a 2-complex, so beta2
	// falls out of chi without a third elimination pass — the mockup's
	// own "beta2 comes from chi for free" line.
	chi := len(g.Nodes) - len(und) + len(tri)
	b2 := chi - b0 + b1Clique

	return BettiNumbers{
		B0: b0, B1: b1, B1Clique: b1Clique, B2: b2,
		Chi: chi, Triangles: len(tri), Rank2: rank2,
	}
}

func countDistinctComponents(comp map[NodeID]int) int {
	seen := make(map[int]bool, len(comp))
	for _, c := range comp {
		seen[c] = true
	}
	return len(seen)
}

// undirectedEdge is a canonical (a < b), deduplicated, self-loop-free
// pair — Graph.Edges may carry both directions of the same link (or the
// same direction twice) as separate rows without that meaning two
// distinct undirected edges; a self-loop contributes no independent
// cycle at all.
type undirectedEdge struct{ a, b NodeID }

func canonicalPair(x, y NodeID) undirectedEdge {
	if y < x {
		x, y = y, x
	}
	return undirectedEdge{a: x, b: y}
}

func undirectedEdges(g Graph) []undirectedEdge {
	seen := make(map[undirectedEdge]bool)
	var out []undirectedEdge
	for _, e := range g.Edges {
		if e.From == e.To {
			continue
		}
		key := canonicalPair(e.From, e.To)
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return out
}

// triangle is one 3-clique — three mutually linked pages.
type triangle struct{ a, b, c NodeID }

// triangles enumerates every 3-clique in the undirected graph: for each
// pair a<b sharing an edge, every common neighbor c>b closes a triangle
// — the same nested-adjacency-set walk graph-algorithms.html's own
// TRIANGLES does, generalized from numeric indices to NodeID (any total
// order over NodeID works; it only has to be consistent, so each
// unordered triple is counted exactly once).
func triangles(und []undirectedEdge) []triangle {
	adj := make(map[NodeID]map[NodeID]bool)
	link := func(x, y NodeID) {
		if adj[x] == nil {
			adj[x] = make(map[NodeID]bool)
		}
		adj[x][y] = true
	}
	for _, e := range und {
		link(e.a, e.b)
		link(e.b, e.a)
	}

	var out []triangle
	for a, neighborsOfA := range adj {
		for b := range neighborsOfA {
			if b <= a {
				continue
			}
			for c := range adj[b] {
				if c <= b {
					continue
				}
				if neighborsOfA[c] {
					out = append(out, triangle{a: a, b: b, c: c})
				}
			}
		}
	}
	return out
}

// gf2BoundaryRank is rank(∂₂): each triangle's boundary is the XOR (over
// GF(2)) of its three edges, one row per triangle, one column per
// undirected edge.
func gf2BoundaryRank(und []undirectedEdge, tri []triangle) int {
	edgeIndex := make(map[undirectedEdge]int, len(und))
	for i, e := range und {
		edgeIndex[e] = i
	}

	rows := make([]*big.Int, len(tri))
	for i, t := range tri {
		row := new(big.Int)
		row.SetBit(row, edgeIndex[canonicalPair(t.a, t.b)], 1)
		row.SetBit(row, edgeIndex[canonicalPair(t.b, t.c)], 1)
		row.SetBit(row, edgeIndex[canonicalPair(t.a, t.c)], 1)
		rows[i] = row
	}

	return gf2Rank(rows, len(und))
}

// gf2Rank is plain Gaussian elimination over GF(2) — XOR instead of
// subtraction — on bitmask rows, one column at a time. big.Int rather
// than a 64-bit machine word: a real workspace's edge count isn't
// bounded to 64, unlike the mockup's own small fixed example graph.
func gf2Rank(rows []*big.Int, numCols int) int {
	rs := make([]*big.Int, len(rows))
	for i, r := range rows {
		rs[i] = new(big.Int).Set(r)
	}

	rank := 0
	for col := 0; col < numCols && rank < len(rs); col++ {
		pivot := -1
		for i := rank; i < len(rs); i++ {
			if rs[i].Bit(col) == 1 {
				pivot = i
				break
			}
		}
		if pivot == -1 {
			continue
		}
		rs[rank], rs[pivot] = rs[pivot], rs[rank]
		for i := range rs {
			if i != rank && rs[i].Bit(col) == 1 {
				rs[i].Xor(rs[i], rs[rank])
			}
		}
		rank++
	}
	return rank
}
