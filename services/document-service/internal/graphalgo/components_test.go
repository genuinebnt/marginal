package graphalgo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentsSingleNodeNoEdges(t *testing.T) {
	g := Graph{Nodes: []NodeID{"a"}}
	comp := Components(g)
	assert.Equal(t, map[NodeID]int{"a": 0}, comp)
}

func TestComponentsMergesAcrossUndirectedChain(t *testing.T) {
	// a -> b -> c is one component even though the edges only point
	// forward — components ignore direction (undirected flood fill).
	g := Graph{
		Nodes: []NodeID{"a", "b", "c"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	comp := Components(g)
	assert.Equal(t, comp["a"], comp["b"])
	assert.Equal(t, comp["b"], comp["c"])
}

func TestComponentsTwoDisjointIslands(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "c", To: "d"}},
	}
	comp := Components(g)
	assert.Equal(t, comp["a"], comp["b"])
	assert.Equal(t, comp["c"], comp["d"])
	assert.NotEqual(t, comp["a"], comp["c"])
}

// TestOrphansMutuallyLinkedPairStillOrphaned pins graph-algorithms.html's
// own stated argument directly: "OrphanPage is a connected components
// problem, not backlinks == 0 — a mutually-linked pair with nothing
// pointing in is still orphaned." a and b link to each other (each has a
// nonzero backlink count) but neither is a root, and nothing reachable
// from root points into the pair, so the whole {a, b} component is an
// orphan.
func TestOrphansMutuallyLinkedPairStillOrphaned(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"root", "a", "b"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "a"}},
	}
	comp := Components(g)
	orphans := Orphans(comp, []NodeID{"root"})
	require.Len(t, orphans, 1)
	assert.Equal(t, comp["a"], orphans[0])
	assert.NotContains(t, orphans, comp["root"])
}

func TestOrphansComponentContainingARootIsNotOrphaned(t *testing.T) {
	// root <-> a, b <-> c: the {root, a} component contains a root, so
	// it's not orphaned; {b, c} contains no root, so it is.
	g := Graph{
		Nodes: []NodeID{"root", "a", "b", "c"},
		Edges: []Edge{{From: "root", To: "a"}, {From: "a", To: "root"}, {From: "b", To: "c"}, {From: "c", To: "b"}},
	}
	comp := Components(g)
	orphans := Orphans(comp, []NodeID{"root"})
	require.Len(t, orphans, 1)
	assert.Equal(t, comp["b"], orphans[0])
	assert.NotContains(t, orphans, comp["root"])
}

func TestOrphansIsolatedSingleNodeIsOrphaned(t *testing.T) {
	g := Graph{Nodes: []NodeID{"root", "lonely"}}
	comp := Components(g)
	orphans := Orphans(comp, []NodeID{"root"})
	assert.Equal(t, []int{comp["lonely"]}, orphans)
}

func TestOrphansNoRootsMeansEveryComponentIsOrphaned(t *testing.T) {
	g := Graph{Nodes: []NodeID{"a", "b"}}
	comp := Components(g)
	orphans := Orphans(comp, nil)
	assert.Len(t, orphans, 2, "with no roots at all, nothing is reachable from one")
}
