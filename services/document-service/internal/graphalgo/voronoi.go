package graphalgo

import "math"

// Point is a 2D coordinate. The force-directed layout is the intended
// source of these (a site's position in the drawn graph), but Voronoi/
// Delaunay are pure geometry over any point set — this package doesn't
// know or care where a coordinate came from.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Site is one page's position in the layout — Voronoi's own input.
// Point is embedded so its JSON flattens into {id, x, y}, not a nested
// object — the wasm bridge's own wire shape (cmd/graphwasm).
type Site struct {
	ID NodeID `json:"id"`
	Point
}

// Rect is the finite viewport a Voronoi diagram is clipped against. An
// unbounded diagram has no finite polygon for an outermost site, so
// "exact Voronoi diagram" here means exact within this rectangle — the
// same practical choice graph.html's own implementation makes, not a
// mathematical compromise.
type Rect struct {
	MinX float64 `json:"min_x"`
	MinY float64 `json:"min_y"`
	MaxX float64 `json:"max_x"`
	MaxY float64 `json:"max_y"`
}

func (r Rect) corners() []Point {
	return []Point{{r.MinX, r.MinY}, {r.MaxX, r.MinY}, {r.MaxX, r.MaxY}, {r.MinX, r.MaxY}}
}

// VoronoiCell is one site's exact territory: every point within bounds
// closer to Site than to any other site, as a convex polygon.
type VoronoiCell struct {
	Site Site    `json:"site"`
	Poly []Point `json:"poly"`
}

// bisector is the perpendicular bisector between si and sj, as a
// half-plane inequality a*x + b*y <= c that holds exactly where a point
// is at least as close to si as to sj: |p−si| ≤ |p−sj| ⟺
// 2(sj−si)·p ≤ |sj|² − |si|².
func bisector(si, sj Point) (a, b, c float64) {
	a = 2 * (sj.X - si.X)
	b = 2 * (sj.Y - si.Y)
	c = sj.X*sj.X + sj.Y*sj.Y - si.X*si.X - si.Y*si.Y
	return a, b, c
}

// clipHalfPlane is Sutherland–Hodgman polygon clipping against one
// half-plane (a*x + b*y <= c is kept, bisector's own convention): walk
// the polygon's edges, keep every vertex already on the kept side, and
// insert the exact crossing point wherever an edge straddles the
// boundary.
func clipHalfPlane(poly []Point, a, b, c float64) []Point {
	f := func(p Point) float64 { return a*p.X + b*p.Y - c }
	var out []Point
	n := len(poly)
	for i := 0; i < n; i++ {
		p, q := poly[i], poly[(i+1)%n]
		fp, fq := f(p), f(q)
		if fp <= 0 {
			out = append(out, p)
		}
		if (fp < 0 && fq > 0) || (fp > 0 && fq < 0) {
			t := fp / (fp - fq)
			out = append(out, Point{X: p.X + (q.X-p.X)*t, Y: p.Y + (q.Y-p.Y)*t})
		}
	}
	return out
}

// Voronoi computes the exact Voronoi diagram of sites, clipped to
// bounds: each site's cell is bounds, successively clipped against its
// own perpendicular bisector with every other site — O(n²) clips total,
// the same trade this repo's force-directed layout makes for its own
// repulsion pass over Fortune's sweep: exact and simple over
// asymptotically faster and subtle, at this repo's demo scale
// (graph.html's own doc comment: "the same kind of answer as
// Barnes–Hut").
func Voronoi(sites []Site, bounds Rect) []VoronoiCell {
	cells := make([]VoronoiCell, len(sites))
	for i, si := range sites {
		poly := bounds.corners()
		for j, sj := range sites {
			if j == i {
				continue
			}
			a, b, c := bisector(si.Point, sj.Point)
			poly = clipHalfPlane(poly, a, b, c)
			if len(poly) < 3 {
				break
			}
		}
		cells[i] = VoronoiCell{Site: si, Poly: poly}
	}
	return cells
}

// PolygonArea is the shoelace formula. A cell's area measures the
// LAYOUT — where the force simulation put a site — never an orphan
// signal on its own: graph.html's own explicit caution is "a hint about
// isolation, never the orphan test — that is connected components"
// (graphalgo.Orphans).
func PolygonArea(poly []Point) float64 {
	var sum float64
	n := len(poly)
	for i := 0; i < n; i++ {
		p, q := poly[i], poly[(i+1)%n]
		sum += p.X*q.Y - q.X*p.Y
	}
	return math.Abs(sum) / 2
}

// DelaunayPair is one edge of the Delaunay dual — two sites whose
// Voronoi cells share a boundary segment.
type DelaunayPair struct {
	A NodeID `json:"a"`
	B NodeID `json:"b"`
}

// Delaunay reads the dual triangulation directly off cells — no second
// algorithm, since the triangulation is already implicit in which
// bisector clipped which edge (graph.html's own delaunay(), ported
// field-for-field including its numeric tolerances: 0.6 units off the
// shared bisector line, 1.5 units of minimum shared-edge length, telling
// a real shared boundary apart from a clipped sliver or a
// floating-point near-miss).
func Delaunay(cells []VoronoiCell) []DelaunayPair {
	const (
		onBisectorTolerance = 0.6
		minSharedEdgeLength = 1.5
	)
	var pairs []DelaunayPair
	for i := 0; i < len(cells); i++ {
		for j := i + 1; j < len(cells); j++ {
			a, b, c := bisector(cells[i].Site.Point, cells[j].Site.Point)
			norm := math.Hypot(a, b)
			if norm == 0 {
				norm = 1
			}
			poly := cells[i].Poly
			shared := false
			for k := 0; k < len(poly) && !shared; k++ {
				p, q := poly[k], poly[(k+1)%len(poly)]
				dp := math.Abs(a*p.X+b*p.Y-c) / norm
				dq := math.Abs(a*q.X+b*q.Y-c) / norm
				if dp < onBisectorTolerance && dq < onBisectorTolerance &&
					math.Hypot(q.X-p.X, q.Y-p.Y) > minSharedEdgeLength {
					shared = true
				}
			}
			if shared {
				pairs = append(pairs, DelaunayPair{A: cells[i].Site.ID, B: cells[j].Site.ID})
			}
		}
	}
	return pairs
}
