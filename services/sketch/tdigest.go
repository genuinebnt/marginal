package sketch

import (
	"math"
	"sort"
)

// Centroid is a cluster of samples: a mean and how many it stands for.
type Centroid struct {
	Mean   float64 `json:"mean"`
	Weight float64 `json:"weight"`
}

// TDigest estimates QUANTILES — "what is p50, p95, p99" — from a stream,
// without keeping the stream.
//
// THE IDEA, and why it is not a histogram. A histogram fixes its buckets in
// advance, so it is accurate where you guessed the data would be and useless
// where you guessed wrong. A t-digest clusters the samples themselves, and
// bounds each cluster's size by a function of WHERE it sits in the
// distribution: clusters near the median may be large, clusters out in the
// tails must be small. So the tails — which is where p99 lives, and p99 is the
// number anyone actually asks for — stay accurate, and the middle, where
// nobody cares about a millisecond, is where the compression happens.
//
// That asymmetry is the whole structure. An implementation that bounded every
// cluster by the same size would be a histogram with extra steps.
type TDigest struct {
	compression float64
	centroids   []Centroid
	buffer      []float64
	count       float64
	min, max    float64
}

// NewTDigest builds a digest. `compression` trades memory for tail accuracy;
// 100 is the usual default and keeps the centroid count in the low hundreds
// regardless of how many samples arrive.
func NewTDigest(compression float64) *TDigest {
	if compression < 20 {
		compression = 20
	}
	return &TDigest{
		compression: compression,
		min:         math.Inf(1),
		max:         math.Inf(-1),
	}
}

// Add buffers one sample. Buffering rather than merging per sample is not an
// optimisation detail: merging one value at a time makes the result depend on
// arrival order far more strongly, and a digest whose answer changes when you
// shuffle the input is a digest nobody can compare against an exact quantile.
func (t *TDigest) Add(x float64) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return
	}
	t.buffer = append(t.buffer, x)
	t.count++
	if x < t.min {
		t.min = x
	}
	if x > t.max {
		t.max = x
	}
	if len(t.buffer) >= 1000 {
		t.flush()
	}
}

// flush merges the buffer into the centroids and re-clusters.
//
// The scale function is the simple one: a cluster may absorb weight while its
// running quantile share stays under 4·q·(1−q)/compression. That expression is
// near zero at q=0 and q=1 — the tails — and largest at q=0.5, which is
// exactly the "small clusters in the tails, large in the middle" rule stated
// above, written as arithmetic.
func (t *TDigest) flush() {
	if len(t.buffer) == 0 {
		return
	}
	all := make([]Centroid, 0, len(t.centroids)+len(t.buffer))
	all = append(all, t.centroids...)
	for _, x := range t.buffer {
		all = append(all, Centroid{Mean: x, Weight: 1})
	}
	t.buffer = t.buffer[:0]

	sort.Slice(all, func(i, j int) bool { return all[i].Mean < all[j].Mean })

	total := 0.0
	for _, c := range all {
		total += c.Weight
	}

	merged := make([]Centroid, 0, len(all))
	cur := all[0]
	soFar := 0.0
	for _, c := range all[1:] {
		q := (soFar + cur.Weight/2) / total
		limit := 4 * total * q * (1 - q) / t.compression
		if cur.Weight+c.Weight <= math.Max(limit, 1) {
			// Weighted mean, not the midpoint: a 900-sample cluster absorbing
			// one sample must barely move, and a midpoint would drag it half
			// way across.
			w := cur.Weight + c.Weight
			cur.Mean = (cur.Mean*cur.Weight + c.Mean*c.Weight) / w
			cur.Weight = w
			continue
		}
		merged = append(merged, cur)
		soFar += cur.Weight
		cur = c
	}
	merged = append(merged, cur)
	t.centroids = merged
}

// Quantile is the value at q ∈ [0,1], interpolated between centroid means.
//
// INVARIANT T1 — Quantile is monotone non-decreasing in q. A p95 below the p50
// is not "a bit inaccurate", it is a number nobody can act on, and it is what
// an interpolation that walks the centroids in the wrong order produces.
func (t *TDigest) Quantile(q float64) float64 {
	t.flush()
	if len(t.centroids) == 0 {
		return math.NaN()
	}
	if q <= 0 {
		return t.min
	}
	if q >= 1 {
		return t.max
	}

	target := q * t.count
	soFar := 0.0
	for i, c := range t.centroids {
		// The centroid's own span of ranks is [soFar, soFar+weight); the mean
		// sits at its centre.
		mid := soFar + c.Weight/2
		if target <= mid {
			if i == 0 {
				return interpolate(target, 0, t.min, mid, c.Mean)
			}
			prev := t.centroids[i-1]
			prevMid := soFar - prev.Weight/2
			return interpolate(target, prevMid, prev.Mean, mid, c.Mean)
		}
		soFar += c.Weight
	}
	last := t.centroids[len(t.centroids)-1]
	return interpolate(target, t.count-last.Weight/2, last.Mean, t.count, t.max)
}

func interpolate(x, x0, y0, x1, y1 float64) float64 {
	if x1 == x0 {
		return y0
	}
	return y0 + (y1-y0)*(x-x0)/(x1-x0)
}

// Centroids exposes the clusters — § 12 draws them, and the shape is the
// argument: dense in the middle, sparse and small in the tails.
func (t *TDigest) Centroids() []Centroid {
	t.flush()
	return t.centroids
}

// Count is how many samples went in.
func (t *TDigest) Count() float64 { return t.count }

// Bytes is the digest's own size: two float64s per centroid.
func (t *TDigest) Bytes() int {
	t.flush()
	return len(t.centroids) * 16
}
