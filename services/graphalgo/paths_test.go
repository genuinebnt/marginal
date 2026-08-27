package graphalgo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chainGraph() Graph {
	// a -> b -> c -> d, a straight line, all edges forward.
	return Graph{
		Nodes: []NodeID{"a", "b", "c", "d"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "d"}},
	}
}

func TestBFSDistancesAlongAChain(t *testing.T) {
	dist, _ := BFS(chainGraph(), "a")
	assert.Equal(t, map[NodeID]int{"a": 0, "b": 1, "c": 2, "d": 3}, dist)
}

func TestBFSIsUndirectedLinkDistanceIgnoresEdgeDirection(t *testing.T) {
	// b -> a is the only edge (backwards relative to a "chain from a"
	// reading) — BFS from a must still reach b at distance 1, because
	// link distance only cares that a link exists, not which way it
	// points.
	g := Graph{Nodes: []NodeID{"a", "b"}, Edges: []Edge{{From: "b", To: "a"}}}
	dist, _ := BFS(g, "a")
	assert.Equal(t, 1, dist["b"])
}

func TestBFSUnreachableNodeIsAbsentFromDist(t *testing.T) {
	g := Graph{Nodes: []NodeID{"a", "island"}}
	dist, _ := BFS(g, "a")
	_, ok := dist["island"]
	assert.False(t, ok)
}

func TestShortestPathReconstructsFullChain(t *testing.T) {
	dist, prev := BFS(chainGraph(), "a")
	path, ok := ShortestPath(dist, prev, "a", "d")
	require.True(t, ok)
	assert.Equal(t, []NodeID{"a", "b", "c", "d"}, path)
}

func TestShortestPathSourceEqualsTarget(t *testing.T) {
	dist, prev := BFS(chainGraph(), "a")
	path, ok := ShortestPath(dist, prev, "a", "a")
	require.True(t, ok)
	assert.Equal(t, []NodeID{"a"}, path)
}

func TestShortestPathUnreachableReturnsFalse(t *testing.T) {
	g := Graph{Nodes: []NodeID{"a", "island"}}
	dist, prev := BFS(g, "a")
	_, ok := ShortestPath(dist, prev, "a", "island")
	assert.False(t, ok)
}

// TestForwardReachableIsDirectedUnlikeBFS is blast radius's whole point:
// a page linking INTO source is not reachable FROM source, even though
// plain (undirected) BFS would say it's one hop away.
func TestForwardReachableIsDirectedUnlikeBFS(t *testing.T) {
	g := Graph{Nodes: []NodeID{"source", "linksIntoSource", "linkedFromSource"},
		Edges: []Edge{{From: "linksIntoSource", To: "source"}, {From: "source", To: "linkedFromSource"}}}

	reachable := ForwardReachable(g, "source")
	assert.Equal(t, 0, reachable["source"])
	assert.Equal(t, 1, reachable["linkedFromSource"])
	_, blastHitsBackward := reachable["linksIntoSource"]
	assert.False(t, blastHitsBackward, "a page linking INTO source must not be part of source's own blast radius")
}

func TestForwardReachableFollowsMultipleHops(t *testing.T) {
	reachable := ForwardReachable(chainGraph(), "a")
	assert.Equal(t, map[NodeID]int{"a": 0, "b": 1, "c": 2, "d": 3}, reachable)
}

func TestDiameterOfAChain(t *testing.T) {
	assert.Equal(t, 3, Diameter(chainGraph()))
}

func TestDiameterSkipsDisconnectedPairsUsesLargestComponent(t *testing.T) {
	// Chain a-b-c-d (diameter 3) plus a disjoint pair x-y (diameter 1) —
	// the graph's diameter is the max across components, not "infinite"
	// because x/y can't reach a/b/c/d.
	g := chainGraph()
	g.Nodes = append(g.Nodes, "x", "y")
	g.Edges = append(g.Edges, Edge{From: "x", To: "y"})
	assert.Equal(t, 3, Diameter(g))
}

func TestDiameterSingleNodeIsZero(t *testing.T) {
	assert.Equal(t, 0, Diameter(Graph{Nodes: []NodeID{"a"}}))
}
