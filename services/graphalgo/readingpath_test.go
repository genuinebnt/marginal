package graphalgo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/graphalgo"
)

func ids(steps []graphalgo.PathStep) []graphalgo.NodeID {
	out := make([]graphalgo.NodeID, len(steps))
	for i, s := range steps {
		out[i] = s.ID
	}
	return out
}

func TestReadingPathFollowsTheAuthorsArrows(t *testing.T) {
	// a → b → p. Reading a, then b, then p follows the links forward, which
	// is the only ordering the graph actually asserts.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "p", "z"},
		Edges: []graphalgo.Edge{{From: "a", To: "b"}, {From: "b", To: "p"}},
	}
	got := graphalgo.ReadingPath(g, "p")
	assert.Equal(t, []graphalgo.NodeID{"a", "b", "p"}, ids(got))
	assert.True(t, got[len(got)-1].Destination, "the destination is always last")
}

func TestReadingPathIgnoresPagesThatCannotReachTheTarget(t *testing.T) {
	// z links FROM p, not to it. It is downstream, not a prerequisite —
	// the distinction a shortest-path or an undirected walk would lose.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "p", "z"},
		Edges: []graphalgo.Edge{{From: "a", To: "p"}, {From: "p", To: "z"}},
	}
	assert.Equal(t, []graphalgo.NodeID{"a", "p"}, ids(graphalgo.ReadingPath(g, "p")))
}

func TestReadingPathOfAnUnlinkedPageIsItself(t *testing.T) {
	// "Start here" is a real answer, and the common one. Returning nothing
	// would make every caller invent the same special case.
	g := graphalgo.Graph{Nodes: []graphalgo.NodeID{"p", "q"}}
	got := graphalgo.ReadingPath(g, "p")
	require.Len(t, got, 1)
	assert.Equal(t, graphalgo.NodeID("p"), got[0].ID)
	assert.True(t, got[0].Destination)
}

func TestReadingPathPutsIndependentPrerequisitesAtTheSameDepth(t *testing.T) {
	// a and b both link straight to p and neither depends on the other, so
	// they can be read in either order. Same depth is how the UI says that.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "p"},
		Edges: []graphalgo.Edge{{From: "a", To: "p"}, {From: "b", To: "p"}},
	}
	got := graphalgo.ReadingPath(g, "p")
	require.Len(t, got, 3)
	assert.Equal(t, got[0].Depth, got[1].Depth)
	assert.Greater(t, got[2].Depth, got[1].Depth)
}

func TestReadingPathKeepsPagesTangledInACycle(t *testing.T) {
	// b and c cite each other and both reach p. The hardest case: they cannot
	// be ordered against each other, and dropping them would silently shorten
	// the path exactly where the graph is most tangled.
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"b", "c", "p"},
		Edges: []graphalgo.Edge{
			{From: "b", To: "c"}, {From: "c", To: "b"}, {From: "c", To: "p"},
		},
	}
	got := ids(graphalgo.ReadingPath(g, "p"))
	assert.ElementsMatch(t, []graphalgo.NodeID{"b", "c", "p"}, got)
	assert.Equal(t, graphalgo.NodeID("p"), got[len(got)-1])
}

func TestReadingPathIsStableAcrossRuns(t *testing.T) {
	g := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"d", "b", "a", "c", "p"},
		Edges: []graphalgo.Edge{
			{From: "a", To: "p"}, {From: "b", To: "p"}, {From: "c", To: "p"}, {From: "d", To: "p"},
		},
	}
	first := ids(graphalgo.ReadingPath(g, "p"))
	for i := 0; i < 25; i++ {
		assert.Equal(t, first, ids(graphalgo.ReadingPath(g, "p")))
	}
}
