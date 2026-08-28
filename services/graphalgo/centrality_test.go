package graphalgo

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// path graph a—b—c: b is on the only path between a and c, so it carries all
// of the betweenness and the endpoints carry none.
func TestBetweennessFindsTheBridgeInAPath(t *testing.T) {
	g := Graph{
		Nodes: []NodeID{"a", "b", "c"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	bc := Betweenness(g)
	require.Greater(t, bc["b"], 0.0)
	require.Equal(t, 0.0, bc["a"])
	require.Equal(t, 0.0, bc["c"])
}

func TestBetweennessIsZeroOnACompleteGraph(t *testing.T) {
	// Everyone is adjacent to everyone, so no shortest path passes THROUGH
	// anyone. A measure that reported centrality here would be measuring
	// degree wearing a different name.
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: []Edge{
			{From: "a", To: "b"}, {From: "a", To: "c"}, {From: "a", To: "d"},
			{From: "b", To: "c"}, {From: "b", To: "d"}, {From: "c", To: "d"},
		},
	}
	for id, v := range Betweenness(g) {
		require.InDelta(t, 0.0, v, 1e-9, "node %s", id)
	}
}

func TestBetweennessSplitsCreditBetweenEqualShortestPaths(t *testing.T) {
	// Two disjoint 2-hop routes from a to d. Neither b nor c is essential, so
	// each gets half the credit — the σ bookkeeping is what this checks, and
	// a naive "count paths through v" would give each of them full credit.
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: []Edge{
			{From: "a", To: "b"}, {From: "b", To: "d"},
			{From: "a", To: "c"}, {From: "c", To: "d"},
		},
	}
	bc := Betweenness(g)
	require.InDelta(t, bc["b"], bc["c"], 1e-9)
	require.Greater(t, bc["b"], 0.0)
}

func TestBetweennessRewardsALowDegreeBridge(t *testing.T) {
	// Two triangles joined by one edge. The joining nodes have the SAME
	// degree as their triangle-mates but far higher betweenness — the case
	// the doc comment claims and the reason the measure earns its cost.
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "x", "y", "z"},
		Edges: []Edge{
			{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"},
			{From: "x", To: "y"}, {From: "y", To: "z"}, {From: "z", To: "x"},
			{From: "c", To: "x"},
		},
	}
	bc := Betweenness(g)
	require.Greater(t, bc["c"], bc["a"])
	require.Greater(t, bc["x"], bc["y"])
}

func TestBetweennessHandlesADisconnectedGraph(t *testing.T) {
	// Unreachable pairs contribute nothing and must not divide by a zero
	// path count.
	g := Graph{
		Nodes: []NodeID{"a", "b", "x", "y"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "x", To: "y"}},
	}
	bc := Betweenness(g)
	for id, v := range bc {
		require.False(t, math.IsNaN(v), "node %s went NaN", id)
		require.Equal(t, 0.0, v)
	}
}

func TestModularityIsHighWhenCommunitiesMatchTheWiring(t *testing.T) {
	// Two triangles, one thin bridge, partitioned the way they are actually
	// wired. Q should be solidly positive.
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "x", "y", "z"},
		Edges: []Edge{
			{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"},
			{From: "x", To: "y"}, {From: "y", To: "z"}, {From: "z", To: "x"},
			{From: "c", To: "x"},
		},
	}
	q := Modularity(g, map[NodeID]string{
		"a": "L", "b": "L", "c": "L", "x": "R", "y": "R", "z": "R",
	})
	require.Greater(t, q, 0.3)
}

func TestModularityIsNearZeroWhenEveryoneIsOneCommunity(t *testing.T) {
	// One community explains nothing the degree sequence does not: e/m is 1
	// and (d/2m)² is also 1, so Q collapses. This is the sanity check that
	// the null term is actually subtracted.
	g := Graph{
		Nodes: []NodeID{"a", "b", "c"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	q := Modularity(g, map[NodeID]string{"a": "all", "b": "all", "c": "all"})
	require.InDelta(t, 0.0, q, 1e-9)
}

func TestModularityIsNegativeWhenCommunitiesCutTheWiring(t *testing.T) {
	// A partition that puts every edge BETWEEN communities is worse than
	// random, and Q says so with a negative number rather than 0.
	g := Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "c", To: "d"}},
	}
	q := Modularity(g, map[NodeID]string{"a": "L", "b": "R", "c": "L", "d": "R"})
	require.Less(t, q, 0.0)
}

func TestModularityExcludesUngroupedNodes(t *testing.T) {
	// An untopiced page is not a community. Pooling the "" group would give
	// it credit for its internal edges and inflate Q with a group that means
	// "we do not know".
	g := Graph{
		Nodes: []NodeID{"a", "b", "u", "v"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "u", To: "v"}},
	}
	grouped := Modularity(g, map[NodeID]string{"a": "L", "b": "L", "u": "", "v": ""})
	pooled := Modularity(g, map[NodeID]string{"a": "L", "b": "L", "u": "N", "v": "N"})
	require.Less(t, grouped, pooled)
}

func TestTopBetweennessIsStableOnTies(t *testing.T) {
	// Every value equal: the order must come from the id, not from map
	// iteration, or the "top bridges" list reshuffles on every refresh.
	bc := map[NodeID]float64{"c": 1, "a": 1, "b": 1}
	require.Equal(t, []NodeID{"a", "b", "c"}, TopBetweenness(bc, 3))
	require.Equal(t, []NodeID{"a"}, TopBetweenness(bc, 1))
}
