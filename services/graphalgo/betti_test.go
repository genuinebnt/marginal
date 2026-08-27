package graphalgo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func undirected(pairs ...[2]NodeID) []Edge {
	edges := make([]Edge, len(pairs))
	for i, p := range pairs {
		edges[i] = Edge{From: p[0], To: p[1]}
	}
	return edges
}

func TestBettiEmptyGraphIsAllZero(t *testing.T) {
	b := Betti(Graph{})
	assert.Equal(t, BettiNumbers{}, b)
}

func TestBettiSingleNodeNoEdges(t *testing.T) {
	b := Betti(Graph{Nodes: []NodeID{"a"}})
	assert.Equal(t, 1, b.B0)
	assert.Equal(t, 0, b.B1)
	assert.Equal(t, 0, b.Triangles)
}

// TestBettiTreeHasNoLoops: a tree (V-1 edges, connected, acyclic) must
// score zero independent loops — the textbook base case for cycle rank.
func TestBettiTreeHasNoLoops(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: undirected([2]NodeID{"a", "b"}, [2]NodeID{"b", "c"}, [2]NodeID{"b", "d"}),
	}
	b := Betti(g)
	assert.Equal(t, 1, b.B0)
	assert.Equal(t, 0, b.B1, "a tree has no independent loops")
	assert.Equal(t, 0, b.Triangles)
	assert.Equal(t, 0, b.B1Clique)
	assert.Equal(t, 0, b.B2)
}

// TestBettiSquareCycleHasOneLoopAndNoTriangle: a 4-cycle with no diagonal
// has exactly one independent loop, and nothing to fill (no 3-cliques),
// so B1Clique must equal B1 unchanged.
func TestBettiSquareCycleHasOneLoopAndNoTriangle(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: undirected([2]NodeID{"a", "b"}, [2]NodeID{"b", "c"}, [2]NodeID{"c", "d"}, [2]NodeID{"d", "a"}),
	}
	b := Betti(g)
	assert.Equal(t, 1, b.B1)
	assert.Equal(t, 0, b.Triangles)
	assert.Equal(t, 0, b.Rank2)
	assert.Equal(t, 1, b.B1Clique, "no triangle exists to fill, so the loop survives unchanged")
}

// TestBettiSingleTriangleFillsItsOwnLoop: a 3-cycle has one independent
// loop in the plain graph, but filling its own single triangle kills it
// — B1Clique must drop to zero.
func TestBettiSingleTriangleFillsItsOwnLoop(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c"},
		Edges: undirected([2]NodeID{"a", "b"}, [2]NodeID{"b", "c"}, [2]NodeID{"a", "c"}),
	}
	b := Betti(g)
	assert.Equal(t, 1, b.B1)
	assert.Equal(t, 1, b.Triangles)
	assert.Equal(t, 1, b.Rank2)
	assert.Equal(t, 0, b.B1Clique, "filling the triangle kills the one loop it bounds")
	assert.Equal(t, 0, b.B2)
}

// TestBettiHollowTetrahedronHasAVoid is graph-algorithms.html's own
// stated reason β₂ is worth printing at all: K4 (four pages, every pair
// linked) with all four triangular faces filled but the solid interior
// not — classically a hollow tetrahedron, homotopy-equivalent to a
// 2-sphere: β₀=1, β₁=0 (every loop bounds a filled face), β₂=1 (one
// enclosed void). This is the one graph in this whole file where β₂ is
// actually nonzero.
func TestBettiHollowTetrahedronHasAVoid(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: undirected(
			[2]NodeID{"a", "b"}, [2]NodeID{"a", "c"}, [2]NodeID{"a", "d"},
			[2]NodeID{"b", "c"}, [2]NodeID{"b", "d"}, [2]NodeID{"c", "d"},
		),
	}
	b := Betti(g)
	assert.Equal(t, 1, b.B0)
	assert.Equal(t, 4, b.Triangles, "every 3-subset of K4 is a triangle")
	assert.Equal(t, 3, b.B1, "cycle rank of K4: E - V + B0 = 6 - 4 + 1")
	assert.Equal(t, 3, b.Rank2)
	assert.Equal(t, 0, b.B1Clique, "every loop in K4 bounds some filled face")
	assert.Equal(t, 2, b.Chi, "V - E + F = 4 - 6 + 4")
	assert.Equal(t, 1, b.B2, "the hollow tetrahedron's one enclosed void")
}

func TestBettiB0CountsDisconnectedComponents(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: undirected([2]NodeID{"a", "b"}, [2]NodeID{"c", "d"}),
	}
	b := Betti(g)
	assert.Equal(t, 2, b.B0)
	assert.Equal(t, 0, b.B1)
}

// TestBettiIgnoresSelfLoopsAndDuplicateDirectedEdges: a self-loop
// contributes no independent cycle, and both directions of the same link
// (or the same direction submitted twice) must collapse to one
// undirected edge, not two — otherwise B1 would overcount.
func TestBettiIgnoresSelfLoopsAndDuplicateDirectedEdges(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b"},
		Edges: []Edge{
			{From: "a", To: "a"},
			{From: "a", To: "b"},
			{From: "b", To: "a"},
		},
	}
	b := Betti(g)
	assert.Equal(t, 1, b.B0)
	assert.Equal(t, 0, b.B1, "one real undirected edge between two nodes is a tree, not a loop")
}
