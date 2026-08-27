package graphalgo

import "math"

// LayoutNode is one site's mutable force-directed simulation state —
// position and velocity, updated one LayoutTick at a time.
type LayoutNode struct {
	ID NodeID  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	VX float64 `json:"vx"`
	VY float64 `json:"vy"`
}

// LayoutParams are graph.html's own tick() constants, ported unchanged:
// Repel (inverse-square repulsion strength, every pair of nodes pushes
// each other apart), SpringLength/SpringK (the rest length and
// stiffness of the spring an edge acts as), Center (a weak pull toward
// the layout's own center so the whole graph doesn't drift off), Damp
// (velocity retained per tick, <1 so the simulation can actually settle
// instead of oscillating forever).
type LayoutParams struct {
	Repel        float64 `json:"repel"`
	SpringLength float64 `json:"spring_length"`
	SpringK      float64 `json:"spring_k"`
	Center       float64 `json:"center"`
	Damp         float64 `json:"damp"`
}

// DefaultLayoutParams are graph.html's own tick() values.
func DefaultLayoutParams() LayoutParams {
	return LayoutParams{Repel: 900, SpringLength: 70, SpringK: 0.02, Center: 0.0025, Damp: 0.86}
}

// AlphaMin/AlphaDecay are graph.html's own "cools to a stop" constants —
// a caller ticks the simulation `for alpha > AlphaMin || dragging in
// progress`, exactly the mockup's own loop() condition ("a settled
// layout redrawing at 60fps is a bug you cannot see").
const (
	AlphaMin   = 0.004
	AlphaDecay = 0.988
)

// LayoutTick advances the simulation by one step and returns the new
// node states — pairwise repulsion (O(n²), the same trade this
// package's own Voronoi makes over a faster but subtler alternative),
// spring attraction along every edge toward SpringLength, a weak pull
// toward (centerX, centerY), damping, then an alpha-scaled position
// update. draggedID's own node (if any) has its velocity zeroed and its
// position left exactly where the caller already set it — the
// simulation never moves whatever the mouse is currently holding.
// Ported field-for-field from graph.html's own tick().
func LayoutTick(nodes []LayoutNode, edges []Edge, params LayoutParams, centerX, centerY, alpha float64, draggedID NodeID) []LayoutNode {
	out := make([]LayoutNode, len(nodes))
	copy(out, nodes)

	index := make(map[NodeID]int, len(out))
	for i, n := range out {
		index[n.ID] = i
	}

	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			dx := out[i].X - out[j].X
			dy := out[i].Y - out[j].Y
			d2 := dx*dx + dy*dy
			if d2 == 0 {
				d2 = 0.01 // two coincident nodes still repel — graph.html's own `|| .01` guard
			}
			d := math.Sqrt(d2)
			f := params.Repel / d2
			fx, fy := dx/d*f, dy/d*f
			out[i].VX += fx
			out[i].VY += fy
			out[j].VX -= fx
			out[j].VY -= fy
		}
	}

	for _, e := range edges {
		ai, aok := index[e.From]
		bi, bok := index[e.To]
		if !aok || !bok || ai == bi {
			continue
		}
		dx := out[bi].X - out[ai].X
		dy := out[bi].Y - out[ai].Y
		d := math.Hypot(dx, dy)
		if d == 0 {
			d = 0.01
		}
		f := (d - params.SpringLength) * params.SpringK
		fx, fy := dx/d*f, dy/d*f
		out[ai].VX += fx
		out[ai].VY += fy
		out[bi].VX -= fx
		out[bi].VY -= fy
	}

	for i := range out {
		out[i].VX += (centerX - out[i].X) * params.Center
		out[i].VY += (centerY - out[i].Y) * params.Center

		if out[i].ID == draggedID {
			out[i].VX, out[i].VY = 0, 0
			continue
		}
		out[i].VX *= params.Damp
		out[i].VY *= params.Damp
		out[i].X += out[i].VX * alpha
		out[i].Y += out[i].VY * alpha
	}

	return out
}

// NextAlpha decays alpha by graph.html's own ALPHA_DECAY — the
// simulation "cools to a stop" because each call shrinks alpha
// geometrically until it crosses AlphaMin.
func NextAlpha(alpha float64) float64 { return alpha * AlphaDecay }

// SeededRNG is graph.html's own linear congruential generator, ported
// unchanged: `seed = (seed*1103515245 + 12345) & 0x7fffffff`. Same seed,
// same sequence — a layout's initial scatter (before the simulation
// settles it) is reproducible run to run, which is the entire content of
// "seeded" in "a seeded force simulation."
type SeededRNG struct{ state int64 }

func NewSeededRNG(seed int64) *SeededRNG { return &SeededRNG{state: seed} }

// Float64 returns the generator's next value in [0, 1).
func (r *SeededRNG) Float64() float64 {
	r.state = (r.state*1103515245 + 12345) & 0x7fffffff
	return float64(r.state) / float64(0x7fffffff)
}

// SeedPositions scatters nodes in a jittered ring around
// (centerX, centerY) — the general form of graph.html's own "seed
// positions by cluster so it does not start as a blob": this repo's real
// pages carry no cluster metadata to seed by, so nodes are spread evenly
// around the circle by index instead, with just enough seeded jitter
// that the starting layout isn't perfectly, visually-deadly symmetric.
// The force simulation (LayoutTick), not this scatter, is what actually
// determines the settled layout.
func SeedPositions(nodes []NodeID, seed int64, centerX, centerY, spread float64) []LayoutNode {
	rng := NewSeededRNG(seed)
	n := len(nodes)
	out := make([]LayoutNode, n)
	for i, id := range nodes {
		base := (float64(i) / float64(n)) * 2 * math.Pi
		angle := base + (rng.Float64()-0.5)*0.9
		dist := spread * (0.4 + rng.Float64()*0.6)
		out[i] = LayoutNode{ID: id, X: centerX + math.Cos(angle)*dist, Y: centerY + math.Sin(angle)*dist}
	}
	return out
}
