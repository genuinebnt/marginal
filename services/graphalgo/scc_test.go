package graphalgo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/graphalgo"
)

func TestStronglyConnectedSplitsALoopFromWhatOnlyTouchesIt(t *testing.T) {
	// The case that separates SCC from Components, and the hardest one here:
	// a -> b -> c -> a is a real loop, and d is reachable FROM it but cannot
	// reach back. Undirected flood fill puts all four in one component;
	// strong connectivity must not.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c", "d"},
		Edges: []graphalgo.Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"}, {From: "c", To: "d"}},
	}
	scc := graphalgo.StronglyConnected(g)
	assert.Equal(t, scc["a"], scc["b"])
	assert.Equal(t, scc["a"], scc["c"])
	assert.NotEqual(t, scc["a"], scc["d"], "d is downstream of the loop, not inside it")
	assert.Equal(t, []int{3, 1}, graphalgo.SCCSizes(scc))

	// And the same graph, ignoring direction, is one component — the
	// contrast is the reason both functions exist.
	assert.Len(t, graphalgo.SCCSizes(graphalgo.Components(g)), 1)
}

func TestStronglyConnectedGivesEverySingletonItsOwnComponent(t *testing.T) {
	// The healthy default for a notebook: links point forward, nothing loops.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c"},
		Edges: []graphalgo.Edge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	scc := graphalgo.StronglyConnected(g)
	assert.Equal(t, []int{1, 1, 1}, graphalgo.SCCSizes(scc))
}

func TestStronglyConnectedDoesNotMergeComponentsThatMerelyTouch(t *testing.T) {
	// Two separate loops joined by one edge. The classic Tarjan bug — using
	// a neighbour's lowlink instead of its index on the back edge — collapses
	// these into one component.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c", "d"},
		Edges: []graphalgo.Edge{
			{From: "a", To: "b"}, {From: "b", To: "a"},
			{From: "c", To: "d"}, {From: "d", To: "c"},
			{From: "b", To: "c"},
		},
	}
	scc := graphalgo.StronglyConnected(g)
	assert.Equal(t, scc["a"], scc["b"])
	assert.Equal(t, scc["c"], scc["d"])
	assert.NotEqual(t, scc["a"], scc["c"])
}

func TestStronglyConnectedNumbersComponentsDeterministically(t *testing.T) {
	// Component 0 must hold the smallest node id, every run — Tarjan's own
	// discovery order depends on where the outer loop starts, and an index
	// that moves is an index nothing can be coloured by.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"z", "m", "a"},
		Edges: []graphalgo.Edge{{From: "z", To: "m"}},
	}
	for i := 0; i < 25; i++ {
		scc := graphalgo.StronglyConnected(g)
		assert.Equal(t, 0, scc["a"])
		assert.Equal(t, 1, scc["m"])
		assert.Equal(t, 2, scc["z"])
	}
}

func TestStronglyConnectedHandlesAnEmptyGraph(t *testing.T) {
	require.Empty(t, graphalgo.StronglyConnected(graphalgo.Graph{}))
}
