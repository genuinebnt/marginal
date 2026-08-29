package graphalgo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/graphalgo"
)

func nearestFixture() graphalgo.Graph {
	// a - b - c - d, plus a - e. Undirected reachability, so distance from a:
	// b=1, e=1, c=2, d=3.
	return graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c", "d", "e", "z"},
		Edges: []graphalgo.Edge{
			{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "d"}, {From: "a", To: "e"},
		},
	}
}

func TestNearestNeighboursRanksByHopDistance(t *testing.T) {
	got := graphalgo.NearestNeighbours(nearestFixture(), "a", 3)
	assert.Equal(t, []graphalgo.Neighbour{
		{ID: "b", Hops: 1}, {ID: "e", Hops: 1}, {ID: "c", Hops: 2},
	}, got)
}

func TestNearestNeighboursExcludesTheSourceAndTheUnreachable(t *testing.T) {
	got := graphalgo.NearestNeighbours(nearestFixture(), "a", 0)
	for _, n := range got {
		assert.NotEqual(t, graphalgo.NodeID("a"), n.ID, "a page is not its own neighbour")
		assert.NotEqual(t, graphalgo.NodeID("z"), n.ID, "z is unreachable, not merely far")
	}
	assert.Len(t, got, 4)
}

func TestNearestNeighboursBreaksTiesStably(t *testing.T) {
	// b and e are both one hop away. BFS visits in queue order, which is
	// insertion order — so without the tie rule this list reshuffles and
	// stops being about distance at all.
	g := nearestFixture()
	for i := 0; i < 25; i++ {
		got := graphalgo.NearestNeighbours(g, "a", 2)
		require.Len(t, got, 2)
		assert.Equal(t, graphalgo.NodeID("b"), got[0].ID)
		assert.Equal(t, graphalgo.NodeID("e"), got[1].ID)
	}
}

func TestRingSizesCountsEachFrontier(t *testing.T) {
	// 1 at distance 0 (a itself), 2 at distance 1 (b, e), 1 each after.
	assert.Equal(t, []int{1, 2, 1, 1}, graphalgo.RingSizes(nearestFixture(), "a"))
}
