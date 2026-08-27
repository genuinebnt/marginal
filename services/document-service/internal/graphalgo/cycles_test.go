package graphalgo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectCycleAcyclicGraphReturnsNil(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	assert.Nil(t, DetectCycle(g))
}

func TestDetectCycleSelfLoop(t *testing.T) {
	g := Graph{Nodes: []NodeID{"a"}, Edges: []Edge{{From: "a", To: "a"}}}
	cycle := DetectCycle(g)
	require.Equal(t, []NodeID{"a", "a"}, cycle)
}

func TestDetectCycleSimpleTriangle(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"}},
	}
	cycle := DetectCycle(g)
	require.NotNil(t, cycle)
	assert.Equal(t, cycle[0], cycle[len(cycle)-1], "a reported cycle must be closed")
	assert.GreaterOrEqual(t, len(cycle), 4, "a, b, c, a")
}

// TestDetectCycleDiamondIsNotACycle is the whole reason three-colour DFS
// exists instead of a plain visited set — graph-algorithms.html's own
// argument: "a visited set answers seen before, not on the current
// path." a -> b, a -> c, b -> d, c -> d visits d twice (once via each
// branch) with no cycle anywhere in the graph.
func TestDetectCycleDiamondIsNotACycle(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: []Edge{
			{From: "a", To: "b"}, {From: "a", To: "c"},
			{From: "b", To: "d"}, {From: "c", To: "d"},
		},
	}
	assert.Nil(t, DetectCycle(g), "a diamond shares a descendant on two branches — that is not a cycle")
}

func TestDetectCycleDisjointAcyclicPlusCyclicComponent(t *testing.T) {
	// x -> y is acyclic and visited first (Nodes order); a -> b -> a is a
	// separate cyclic component. The acyclic component must not produce a
	// false positive, and the real cycle in the other component must
	// still be found.
	g := Graph{
		Nodes: []NodeID{"x", "y", "a", "b"},
		Edges: []Edge{{From: "x", To: "y"}, {From: "a", To: "b"}, {From: "b", To: "a"}},
	}
	cycle := DetectCycle(g)
	require.NotNil(t, cycle)
	assert.Equal(t, cycle[0], cycle[len(cycle)-1])
}
