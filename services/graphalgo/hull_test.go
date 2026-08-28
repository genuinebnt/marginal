package graphalgo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvexHullDropsInteriorPoints(t *testing.T) {
	// A square with a point in the middle. The interior point is the whole
	// test: a hull that keeps it is not a hull.
	h := ConvexHull([]Point{
		{0, 0}, {10, 0}, {10, 10}, {0, 10}, {5, 5},
	})
	require.Len(t, h, 4)
	require.NotContains(t, h, Point{5, 5})
}

func TestConvexHullDropsCollinearPoints(t *testing.T) {
	// A point exactly on an edge adds no area. Keeping it would produce a
	// polygon with a redundant vertex — harmless to draw, but it makes hull
	// length useless as a shape signal.
	h := ConvexHull([]Point{{0, 0}, {5, 0}, {10, 0}, {10, 10}, {0, 10}})
	require.Len(t, h, 4)
	require.NotContains(t, h, Point{5, 0})
}

func TestConvexHullHandlesTooFewPointsToBound(t *testing.T) {
	// One and two points have no polygon. They must not panic and must not
	// come back empty — a single page still needs something to draw.
	require.Len(t, ConvexHull([]Point{{1, 1}}), 1)
	require.Len(t, ConvexHull([]Point{{1, 1}, {2, 2}}), 2)
	require.Empty(t, ConvexHull(nil))
}

func TestConvexHullIsCounterClockwise(t *testing.T) {
	h := ConvexHull([]Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}})
	// Shoelace: positive area means counter-clockwise, which the fill rule
	// and any later offsetting both depend on.
	var area float64
	for i := range h {
		j := (i + 1) % len(h)
		area += h[i].X*h[j].Y - h[j].X*h[i].Y
	}
	require.Greater(t, area, 0.0)
}

func TestExpandPushesEveryVertexOutward(t *testing.T) {
	h := ConvexHull([]Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}})
	e := Expand(h, 4)
	require.Len(t, e, len(h))
	// Every expanded vertex is further from the centroid (5,5) than before.
	for i := range h {
		before := (h[i].X-5)*(h[i].X-5) + (h[i].Y-5)*(h[i].Y-5)
		after := (e[i].X-5)*(e[i].X-5) + (e[i].Y-5)*(e[i].Y-5)
		require.Greater(t, after, before)
	}
}

func TestExpandLeavesADegenerateHullAlone(t *testing.T) {
	// Every point at the centroid: there is no outward direction, and the
	// naive implementation divides by zero here.
	e := Expand([]Point{{5, 5}, {5, 5}}, 4)
	require.Equal(t, []Point{{5, 5}, {5, 5}}, e)
}

func TestTerritoriesSkipsUngroupedPoints(t *testing.T) {
	// An untopiced page belongs to no territory by definition — it must not
	// pull some other topic's hull toward it, and must not form a "" group.
	out := Territories([]HullPoint{
		{Group: "protocol", X: 0, Y: 0},
		{Group: "protocol", X: 10, Y: 0},
		{Group: "protocol", X: 5, Y: 9},
		{Group: "", X: 500, Y: 500},
	}, 0)
	require.Len(t, out, 1)
	require.Equal(t, "protocol", out[0].Group)
	for _, p := range out[0].Points {
		require.Less(t, p.X, 100.0)
	}
}

func TestTerritoriesAreOrderedByGroupName(t *testing.T) {
	// Draw order must be stable across ticks, or overlapping regions flicker
	// as the layout settles.
	out := Territories([]HullPoint{
		{Group: "storage", X: 0, Y: 0}, {Group: "storage", X: 1, Y: 1},
		{Group: "protocol", X: 5, Y: 5}, {Group: "protocol", X: 6, Y: 6},
		{Group: "interface", X: 9, Y: 9}, {Group: "interface", X: 8, Y: 8},
	}, 0)
	require.Equal(t, []string{"interface", "protocol", "storage"},
		[]string{out[0].Group, out[1].Group, out[2].Group})
}
