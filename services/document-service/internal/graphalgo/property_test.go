package graphalgo

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// TestBettiB1CliqueNeverNegative is a real, non-tautological algebraic
// fact, not just a sanity check: dim B1(∂₂'s image) can never exceed
// dim Z1 (the cycle space ∂1's kernel) because ∂₁∘∂₂ = 0 always in a
// chain complex — every triangle's own boundary is itself a cycle, so
// filling triangles can only ever remove loops from the cycle space,
// never push B1Clique below zero. Checked here across random graphs
// rather than trusted from the formula alone, since this package's
// input ultimately comes from user-authored page links (RFC-002-adjacent
// untrusted-input discipline, .agents/agents.md), not from a hand-picked
// fixture.
func TestBettiB1CliqueNeverNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := randomGraph(t)
		b := Betti(g)

		if b.B1Clique < 0 {
			t.Fatalf("B1Clique went negative (%d) for graph %+v", b.B1Clique, g)
		}
		if b.Rank2 > b.Triangles {
			t.Fatalf("rank(∂2) = %d exceeds the number of triangles (%d)", b.Rank2, b.Triangles)
		}
		if b.B0 < 1 && len(g.Nodes) > 0 {
			t.Fatalf("a non-empty graph must have at least one component")
		}
	})
}

// TestComponentsAndOrphansNeverPanicOnRandomGraphs exercises the whole
// graphalgo surface (not just Betti) against adversarial shapes — dense,
// sparse, disconnected, all self-loops — the same reason
// .agents/agents.md asks for native fuzzing/property tests on anything
// touching untrusted input: a page-link graph built entirely from
// user-typed [[titles]] can be shaped in ways a hand-written fixture
// would never think to try.
func TestComponentsAndOrphansNeverPanicOnRandomGraphs(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := randomGraph(t)
		comp := Components(g)
		_ = Orphans(comp, g.Nodes[:min(len(g.Nodes), 1)])
		_ = DetectCycle(g)
		_ = Diameter(g)
		if len(g.Nodes) > 0 {
			_, _ = BFS(g, g.Nodes[0])
			_ = ForwardReachable(g, g.Nodes[0])
		}
	})
}

// TestVoronoiCellsAlwaysPartitionBoundsExactly is the defining property
// of a Voronoi diagram, checked across random site counts and positions
// rather than only the hand-picked fixtures in voronoi_test.go: every
// cell's area must sum to exactly the bounding rectangle's own area, no
// matter how the sites are arranged — the mockup's own claim ("cell
// area measures the layout") only means something if this holds for any
// layout, not just a convenient one.
func TestVoronoiCellsAlwaysPartitionBoundsExactly(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		bounds := Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
		n := rapid.IntRange(1, 15).Draw(t, "n")
		sites := make([]Site, n)
		for i := range sites {
			sites[i] = Site{
				ID: NodeID(fmt.Sprintf("s%d", i)),
				Point: Point{
					X: rapid.Float64Range(0, 100).Draw(t, "x"),
					Y: rapid.Float64Range(0, 100).Draw(t, "y"),
				},
			}
		}

		cells := Voronoi(sites, bounds)
		var total float64
		for _, c := range cells {
			total += PolygonArea(c.Poly)
		}
		want := (bounds.MaxX - bounds.MinX) * (bounds.MaxY - bounds.MinY)
		if diff := total - want; diff > 1e-3 || diff < -1e-3 {
			t.Fatalf("cells summed to %.6f, want %.6f (sites=%+v)", total, want, sites)
		}
	})
}

func randomGraph(t *rapid.T) Graph {
	n := rapid.IntRange(0, 12).Draw(t, "n")
	nodes := make([]NodeID, n)
	for i := range nodes {
		nodes[i] = NodeID(fmt.Sprintf("n%d", i))
	}
	if n == 0 {
		return Graph{}
	}

	numEdges := rapid.IntRange(0, n*n).Draw(t, "numEdges")
	edges := make([]Edge, numEdges)
	for i := range edges {
		from := nodes[rapid.IntRange(0, n-1).Draw(t, "from")]
		to := nodes[rapid.IntRange(0, n-1).Draw(t, "to")]
		edges[i] = Edge{From: from, To: to}
	}
	return Graph{Nodes: nodes, Edges: edges}
}
