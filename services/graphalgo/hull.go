package graphalgo

import (
	"math"
	"sort"
)

// Territory hulls — the coloured background polygons § 07 GRAPH draws behind
// its nodes. Each is the convex hull of one topic's settled node positions,
// so "where a topic lives on the plane" is a computed region rather than a
// drawn decoration.
//
// A hull rather than the Voronoi territory already in this package: Voronoi
// partitions the WHOLE plane between sites, which is the right answer to
// "which page is nearest here" and the wrong one to "where is this topic",
// since it assigns empty space to whichever page happens to border it. A hull
// covers only where the topic's pages actually are, and topics may overlap —
// which is itself informative, because two topics whose hulls overlap are two
// topics whose pages are interleaved.

// HullPoint is one settled node position, tagged with the group it belongs to.
type HullPoint struct {
	Group string  `json:"group"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

// Hull is one group's boundary, in order, ready to stroke as a polygon.
type Hull struct {
	Group  string  `json:"group"`
	Points []Point `json:"points"`
}

// Point is voronoi.go's own — this package already has exactly this type,
// and a second one would silently split the geometry in half.

// cross is the 2-D cross product of OA and OB. Positive means counter-
// clockwise, which is the turn direction the scan keeps.
func cross(o, a, b Point) float64 {
	return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X)
}

// ConvexHull returns the convex hull of pts in counter-clockwise order,
// by Andrew's monotone chain: sort by x (then y), then sweep once for the
// lower boundary and once for the upper. O(n log n), dominated by the sort.
//
// Fewer than three points has no hull as such, and the input is returned
// sorted — a single page still needs SOMETHING to draw, and callers render
// one or two points as a dot or a segment rather than a polygon.
func ConvexHull(pts []Point) []Point {
	if len(pts) < 3 {
		out := append([]Point(nil), pts...)
		sort.Slice(out, func(i, j int) bool {
			if out[i].X != out[j].X {
				return out[i].X < out[j].X
			}
			return out[i].Y < out[j].Y
		})
		return out
	}

	p := append([]Point(nil), pts...)
	sort.Slice(p, func(i, j int) bool {
		if p[i].X != p[j].X {
			return p[i].X < p[j].X
		}
		return p[i].Y < p[j].Y
	})

	// Build lower then upper. A non-left turn means the middle point is
	// inside the hull being built, so it is popped — that pop is what keeps
	// the whole thing linear after the sort.
	build := func(src []Point) []Point {
		var out []Point
		for _, q := range src {
			for len(out) >= 2 && cross(out[len(out)-2], out[len(out)-1], q) <= 0 {
				out = out[:len(out)-1]
			}
			out = append(out, q)
		}
		return out
	}

	lower := build(p)
	rev := make([]Point, len(p))
	for i, q := range p {
		rev[len(p)-1-i] = q
	}
	upper := build(rev)

	// Drop each chain's last point: it is the other chain's first.
	return append(lower[:len(lower)-1], upper[:len(upper)-1]...)
}

// Expand pushes a hull outward from its centroid by pad, so the polygon
// contains its nodes rather than passing exactly through them — a boundary
// drawn through the node centres reads as a net, not a territory.
func Expand(h []Point, pad float64) []Point {
	if len(h) == 0 || pad == 0 {
		return h
	}
	var cx, cy float64
	for _, p := range h {
		cx += p.X
		cy += p.Y
	}
	cx /= float64(len(h))
	cy /= float64(len(h))

	out := make([]Point, len(h))
	for i, p := range h {
		dx, dy := p.X-cx, p.Y-cy
		d := dx*dx + dy*dy
		if d == 0 {
			out[i] = p
			continue
		}
		// Normalise without a sqrt call per point in the hot path.
		inv := pad / math.Sqrt(d)
		out[i] = Point{X: p.X + dx*inv, Y: p.Y + dy*inv}
	}
	return out
}

// Territories groups pts by Group and returns one padded hull per group,
// ordered by group name so the draw order is stable across ticks — an
// unstable order makes overlapping regions flicker as the layout settles.
func Territories(pts []HullPoint, pad float64) []Hull {
	byGroup := map[string][]Point{}
	for _, p := range pts {
		if p.Group == "" {
			continue // untopiced pages belong to no territory, by definition
		}
		byGroup[p.Group] = append(byGroup[p.Group], Point{X: p.X, Y: p.Y})
	}

	names := make([]string, 0, len(byGroup))
	for g := range byGroup {
		names = append(names, g)
	}
	sort.Strings(names)

	out := make([]Hull, 0, len(names))
	for _, g := range names {
		out = append(out, Hull{Group: g, Points: Expand(ConvexHull(byGroup[g]), pad)})
	}
	return out
}
