package graphalgo

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBoundsSize = 100.0

func testBounds() Rect { return Rect{MinX: 0, MinY: 0, MaxX: testBoundsSize, MaxY: testBoundsSize} }

func TestVoronoiSingleSiteOwnsTheWholeBounds(t *testing.T) {
	sites := []Site{{ID: "a", Point: Point{X: 50, Y: 50}}}
	cells := Voronoi(sites, testBounds())
	require.Len(t, cells, 1)
	assert.InDelta(t, testBoundsSize*testBoundsSize, PolygonArea(cells[0].Poly), 1e-9)
}

// TestVoronoiTwoSitesEachOwnHalfTheBounds pins the exact geometry for
// the simplest real case: two sites symmetric about the bounds' vertical
// midline split it into two equal halves, and the shared boundary is
// exactly that midline.
func TestVoronoiTwoSitesEachOwnHalfTheBounds(t *testing.T) {
	sites := []Site{
		{ID: "left", Point: Point{X: 25, Y: 50}},
		{ID: "right", Point: Point{X: 75, Y: 50}},
	}
	cells := Voronoi(sites, testBounds())
	require.Len(t, cells, 2)

	total := PolygonArea(cells[0].Poly) + PolygonArea(cells[1].Poly)
	assert.InDelta(t, testBoundsSize*testBoundsSize, total, 1e-6, "the two cells must exactly partition the bounds, no gap or overlap")
	assert.InDelta(t, PolygonArea(cells[0].Poly), PolygonArea(cells[1].Poly), 1e-6, "symmetric sites get equal territory")

	for _, p := range cells[0].Poly {
		assert.LessOrEqual(t, p.X, 50.0+1e-9, "the left site's cell must never cross the shared midline")
	}
}

// TestVoronoiCellsPartitionBoundsWithNoGapOrOverlap is the general-case
// version of the two-site test above: for any number of sites, every
// cell's area must sum to exactly the bounds' own area — the defining
// property of a Voronoi diagram (every point in bounds belongs to
// exactly one cell).
func TestVoronoiCellsPartitionBoundsWithNoGapOrOverlap(t *testing.T) {
	rng := rand.New(rand.NewSource(20260827))
	sites := make([]Site, 12)
	for i := range sites {
		sites[i] = Site{ID: NodeID(string(rune('a' + i))), Point: Point{
			X: rng.Float64() * testBoundsSize,
			Y: rng.Float64() * testBoundsSize,
		}}
	}
	cells := Voronoi(sites, testBounds())

	var total float64
	for _, c := range cells {
		total += PolygonArea(c.Poly)
	}
	assert.InDelta(t, testBoundsSize*testBoundsSize, total, 1e-6)
}

func TestVoronoiEveryPointClosestToItsOwnSite(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	sites := make([]Site, 8)
	for i := range sites {
		sites[i] = Site{ID: NodeID(string(rune('a' + i))), Point: Point{
			X: rng.Float64() * testBoundsSize,
			Y: rng.Float64() * testBoundsSize,
		}}
	}
	cells := Voronoi(sites, testBounds())

	dist := func(p Point, s Site) float64 { return math.Hypot(p.X-s.X, p.Y-s.Y) }

	for i, cell := range cells {
		for _, vertex := range cell.Poly {
			closest := 0
			for j, other := range sites {
				if dist(vertex, other) < dist(vertex, sites[closest])-1e-9 {
					closest = j
				}
			}
			// A vertex sits on a boundary between (at least) two equally
			// close sites — verify sites[i] is one of the nearest, not
			// necessarily the unique argmin.
			assert.InDelta(t, dist(vertex, sites[closest]), dist(vertex, sites[i]), 1e-6,
				"every vertex of site %d's cell must be at least as close to it as to the nearest site", i)
		}
	}
}

// TestDelaunayTriangleAllThreePairsAreNeighbours: three sites forming a
// genuine triangle (no degenerate collinearity) must all be mutually
// Delaunay-adjacent — the classic base case.
func TestDelaunayTriangleAllThreePairsAreNeighbours(t *testing.T) {
	sites := []Site{
		{ID: "a", Point: Point{X: 20, Y: 20}},
		{ID: "b", Point: Point{X: 80, Y: 30}},
		{ID: "c", Point: Point{X: 50, Y: 80}},
	}
	cells := Voronoi(sites, testBounds())
	pairs := Delaunay(cells)
	assert.Len(t, pairs, 3, "three sites forming a triangle must all be pairwise Delaunay neighbours")
}

// TestDelaunayFarApartSiteIsNotANeighbourOfAllOthers: a fourth site far
// outside a tight cluster of three should not be Delaunay-adjacent to
// every one of them — its cell only borders whichever cluster members
// face it.
func TestDelaunayFarApartSiteIsNotANeighbourOfAllOthers(t *testing.T) {
	sites := []Site{
		{ID: "a", Point: Point{X: 10, Y: 10}},
		{ID: "b", Point: Point{X: 15, Y: 10}},
		{ID: "c", Point: Point{X: 10, Y: 15}},
		{ID: "far", Point: Point{X: 95, Y: 95}},
	}
	cells := Voronoi(sites, testBounds())
	pairs := Delaunay(cells)

	farNeighbours := 0
	for _, p := range pairs {
		if p.A == "far" || p.B == "far" {
			farNeighbours++
		}
	}
	assert.Less(t, farNeighbours, 3, "the far site cannot be a Delaunay neighbour of every tightly clustered site")
}

func TestPolygonAreaOfAUnitSquare(t *testing.T) {
	square := []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	assert.InDelta(t, 1.0, PolygonArea(square), 1e-9)
}
