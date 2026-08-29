package graphalgo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/graphalgo"
)

func TestTopologicalSortPlacesEveryPrerequisiteFirst(t *testing.T) {
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c", "d"},
		Edges: []graphalgo.Edge{
			{From: "a", To: "b"}, {From: "a", To: "c"},
			{From: "b", To: "d"}, {From: "c", To: "d"},
		},
	}
	order, ok := graphalgo.TopologicalSort(g)
	require.True(t, ok)
	require.Len(t, order, 4)

	// The law, checked rather than asserted as a literal: for every edge,
	// From must appear before To. A hard-coded expected slice would pass on
	// a wrong order that happened to match.
	at := map[graphalgo.NodeID]int{}
	for i, n := range order {
		at[n] = i
	}
	for _, e := range g.Edges {
		assert.Less(t, at[e.From], at[e.To], "%s must precede %s", e.From, e.To)
	}
}

func TestTopologicalSortReturnsThePlaceablePartOfACyclicGraph(t *testing.T) {
	// The hardest case: failure has to be USEFUL. a is orderable; b/c are a
	// loop and d hangs off it, so three nodes are unplaced — "these are
	// tangled" is actionable where a bare error is not.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c", "d"},
		Edges: []graphalgo.Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"}, {From: "c", To: "b"},
			{From: "c", To: "d"},
		},
	}
	order, ok := graphalgo.TopologicalSort(g)
	assert.False(t, ok)
	assert.Equal(t, []graphalgo.NodeID{"a"}, order)
	assert.Equal(t, []graphalgo.NodeID{"b", "c", "d"}, graphalgo.Unplaced(g, order))
}

func TestTopologicalSortIsStableAcrossRuns(t *testing.T) {
	// Four independent nodes: every permutation is a valid topological
	// order, so only the tie rule makes the answer reproducible.
	g := graphalgo.Graph{Nodes: []graphalgo.NodeID{"d", "b", "a", "c"}}
	for i := 0; i < 25; i++ {
		order, ok := graphalgo.TopologicalSort(g)
		require.True(t, ok)
		assert.Equal(t, []graphalgo.NodeID{"a", "b", "c", "d"}, order)
	}
}

func TestLayersGroupsWhatCanBeReadInParallel(t *testing.T) {
	// b and c both depend only on a, so they are one level: either order,
	// or both at once. The level count is the longest dependency chain.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c", "d"},
		Edges: []graphalgo.Edge{
			{From: "a", To: "b"}, {From: "a", To: "c"}, {From: "b", To: "d"},
		},
	}
	order, ok := graphalgo.TopologicalSort(g)
	require.True(t, ok)
	layers := graphalgo.Layers(g, order)
	require.Len(t, layers, 3)
	assert.Equal(t, []graphalgo.NodeID{"a"}, layers[0])
	assert.ElementsMatch(t, []graphalgo.NodeID{"b", "c"}, layers[1])
	assert.Equal(t, []graphalgo.NodeID{"d"}, layers[2])
}

func TestLayersUsesTheLONGESTPathNotTheFirstOneFound(t *testing.T) {
	// d depends on a (one hop) AND on c (three hops). It belongs at level 3;
	// taking the first prerequisite seen would put it at level 1 and draw a
	// dependency arrow pointing backwards.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c", "d"},
		Edges: []graphalgo.Edge{
			{From: "a", To: "d"},
			{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "d"},
		},
	}
	order, ok := graphalgo.TopologicalSort(g)
	require.True(t, ok)
	layers := graphalgo.Layers(g, order)
	require.Len(t, layers, 4)
	assert.Equal(t, []graphalgo.NodeID{"d"}, layers[3])
}
