package graphalgo

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runToSettle(nodes []LayoutNode, edges []Edge, params LayoutParams, centerX, centerY float64, dragged NodeID, maxTicks int) []LayoutNode {
	alpha := 1.0
	for i := 0; i < maxTicks && (alpha > AlphaMin || dragged != ""); i++ {
		nodes = LayoutTick(nodes, edges, params, centerX, centerY, alpha, dragged)
		alpha = NextAlpha(alpha)
		if dragged != "" {
			break // a single tick is enough to prove the dragged node doesn't move
		}
	}
	return nodes
}

func dist(a, b LayoutNode) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

// TestLayoutTwoConnectedNodesSettleNearSpringLength is the simplest real
// check of the physics: two nodes joined by one edge, starting far
// apart, must end up roughly SpringLength apart once the simulation
// settles — not exact (repulsion always pushes them slightly further
// than the spring's own rest length), but close.
func TestLayoutTwoConnectedNodesSettleNearSpringLength(t *testing.T) {
	params := DefaultLayoutParams()
	nodes := []LayoutNode{
		{ID: "a", X: 0, Y: 0},
		{ID: "b", X: 500, Y: 0},
	}
	edges := []Edge{{From: "a", To: "b"}}

	settled := runToSettle(nodes, edges, params, 250, 0, "", 5000)
	got := dist(settled[0], settled[1])
	assert.InDelta(t, params.SpringLength, got, 15, "two connected nodes should settle near the spring's rest length, got %.2f", got)
}

// TestLayoutDraggedNodeIsNeverMovedBySimulation: the node currently held
// by the mouse must stay exactly where the caller put it — the whole
// point of "reheats on drag" is that dragging drives the layout, not the
// other way around.
func TestLayoutDraggedNodeIsNeverMovedBySimulation(t *testing.T) {
	params := DefaultLayoutParams()
	nodes := []LayoutNode{
		{ID: "dragged", X: 123, Y: 456},
		{ID: "other", X: 0, Y: 0},
	}
	edges := []Edge{{From: "dragged", To: "other"}}

	out := LayoutTick(nodes, edges, params, 250, 250, 1.0, "dragged")

	dragged := out[0]
	require.Equal(t, NodeID("dragged"), dragged.ID)
	assert.Equal(t, 123.0, dragged.X, "the dragged node's position must be untouched by the tick")
	assert.Equal(t, 456.0, dragged.Y)
	assert.Equal(t, 0.0, dragged.VX, "the dragged node's velocity must be zeroed, not accumulated")
	assert.Equal(t, 0.0, dragged.VY)
}

// TestLayoutRepulsionNeverProducesNaNForCoincidentNodes: two nodes
// starting at the exact same point must not divide by zero — graph.html's
// own `d2 || .01` guard exists for exactly this case. The direction of
// repulsion is genuinely undefined for zero separation (dx = dy = 0
// leaves the force magnitude huge but its x/y components both zero,
// same as the mockup's own math), so the real guarantee here is "no
// NaN," not "the pair visibly separates in one tick" — that only
// happens once something else (another node, a later non-zero jitter)
// breaks the tie.
func TestLayoutRepulsionNeverProducesNaNForCoincidentNodes(t *testing.T) {
	params := DefaultLayoutParams()
	nodes := []LayoutNode{
		{ID: "a", X: 50, Y: 50},
		{ID: "b", X: 50, Y: 50},
	}
	out := LayoutTick(nodes, nil, params, 50, 50, 1.0, "")
	assert.False(t, math.IsNaN(out[0].VX) || math.IsNaN(out[0].VY), "must never produce NaN from a zero-distance pair")
	assert.False(t, math.IsInf(out[0].VX, 0) || math.IsInf(out[0].VY, 0), "must never produce Inf from a zero-distance pair")
}

func TestNextAlphaDecaysMonotonicallyBelowMin(t *testing.T) {
	alpha := 1.0
	iterations := 0
	for alpha > AlphaMin && iterations < 10000 {
		next := NextAlpha(alpha)
		require.Less(t, next, alpha, "alpha must strictly decrease every tick")
		alpha = next
		iterations++
	}
	assert.Less(t, iterations, 10000, "the simulation must actually cool below AlphaMin in a bounded number of ticks")
}

func TestSeedPositionsIsDeterministicForTheSameSeed(t *testing.T) {
	nodes := []NodeID{"a", "b", "c", "d"}
	first := SeedPositions(nodes, 20260807, 250, 250, 150)
	second := SeedPositions(nodes, 20260807, 250, 250, 150)
	assert.Equal(t, first, second, "the same seed must produce the exact same scatter every time")
}

func TestSeedPositionsDiffersForDifferentSeeds(t *testing.T) {
	nodes := []NodeID{"a", "b", "c", "d"}
	first := SeedPositions(nodes, 1, 250, 250, 150)
	second := SeedPositions(nodes, 2, 250, 250, 150)
	assert.NotEqual(t, first, second)
}
