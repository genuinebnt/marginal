// Package bench is § 16 PERF's engine: a real benchmark of real
// code paths from this repo, run wherever it is compiled.
//
// The screen says "measured on your machine" and this is what
// makes that true. The workloads are not synthetic loops — they
// are `documentcore.Page.Apply`, `mdc.Compile` and
// `netsim.Run`, the same functions the editor, the paste
// handler and § 14 call. A benchmark of code nothing else runs
// measures the benchmark.
//
// Timing is `time.Since` per iteration. Under GOOS=js that is
// backed by the host's clock and quantised — usually to a
// microsecond, sometimes coarser, and never to zero, so
// sub-microsecond iterations land in one bucket rather than
// producing a fake spread. The screen states the resolution it
// actually observed rather than implying nanoseconds.
package bench

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Bucket is one log-spaced latency bucket.
type Bucket struct {
	// LoNs and HiNs bound the bucket, inclusive-exclusive.
	LoNs  int64  `json:"lo_ns"`
	HiNs  int64  `json:"hi_ns"`
	Count int    `json:"count"`
	Label string `json:"label"`
}

// Frame is one row of the flame graph.
//
// Walked from a real call tree, not sampled: each workload
// declares its own phases and the harness times them. A sampling
// profiler is not available in wasm, and drawing one anyway from
// invented stacks would be the exact dishonesty this screen is
// against.
type Frame struct {
	Name  string `json:"name"`
	Depth int    `json:"depth"`
	// SelfNs excludes children; TotalNs includes them.
	SelfNs  int64 `json:"self_ns"`
	TotalNs int64 `json:"total_ns"`
	// Fraction of the root's total, for the bar width.
	Fraction float64 `json:"fraction"`
	Calls    int     `json:"calls"`
}

// Result is § 16's whole screen, from one run.
type Result struct {
	Workload string `json:"workload"`
	Samples  int    `json:"samples"`
	// Iterations actually timed — a workload may be clamped below
	// the requested sample count when one iteration is expensive,
	// and saying so is better than quietly running fewer.
	Ran int `json:"ran"`
	// Budgeted is true when the run stopped on the clock
	// rather than on the sample count — the screen says which,
	// because "10 000 samples" and "as many as fit in a
	// second" are different claims.
	Budgeted bool     `json:"budgeted"`
	Buckets  []Bucket `json:"buckets"`

	P50  float64 `json:"p50_ns"`
	P95  float64 `json:"p95_ns"`
	P99  float64 `json:"p99_ns"`
	P999 float64 `json:"p999_ns"`
	Min  float64 `json:"min_ns"`
	Max  float64 `json:"max_ns"`
	Mean float64 `json:"mean_ns"`

	// ClockResolutionNs is the smallest non-zero gap the clock
	// actually produced. Stated because it is what forces the
	// batching below, and because every number here is
	// quantised by it.
	ClockResolutionNs int64 `json:"clock_resolution_ns"`
	// Supported is the highest quantile the sample count
	// can actually support: a p99.9 over 29 samples is
	// just the maximum wearing a percentile's name. The
	// screen greys out anything above this rather than
	// printing a number that is not one.
	Supported float64 `json:"supported_quantile"`
	// BatchSize is how many iterations each timed sample
	// covers. 1 means every sample is one iteration.
	//
	// It is greater than 1 whenever one iteration is faster
	// than the host clock can see — which in a browser is the
	// normal case, since `performance.now()` is deliberately
	// coarsened against timing attacks. Timing single
	// iterations there produces a histogram of zeroes and a
	// p50 of "0 ns", which is not a fast result, it is no
	// result. Each sample is then the MEAN of BatchSize
	// iterations, and the screen says so — a distribution of
	// batch means is narrower than the real one, and calling
	// its p99.9 a tail latency without that caveat would be a
	// lie of exactly the kind this screen exists against.
	BatchSize int `json:"batch_size"`
	// TotalNs is the whole run, so the screen can say what it cost
	// you to look at it.
	TotalNs int64 `json:"total_ns"`

	Frames []Frame `json:"frames"`
	// Note is what this particular workload actually did, in a
	// sentence the screen prints verbatim.
	Note string `json:"note"`
}

// Workload is one thing worth timing.
type Workload struct {
	Name string
	Note string
	// MaxSamples clamps the requested count for workloads where one
	// iteration is milliseconds rather than microseconds.
	MaxSamples int
	// Setup runs ONCE, untimed, before the loop.
	//
	// Separating it is the difference between a benchmark and a
	// number: timing `seedPage(60)` inside every iteration of
	// "apply one op" measures building a page sixty times and
	// reports it as the cost of one keystroke, which is off by
	// two orders of magnitude and looks entirely plausible.
	Setup func()
	// Run does one iteration, recording its own phases into t.
	Run func(t *Tracer, i int)
}

// Tracer collects the call tree one iteration walks.
//
// Manual spans rather than sampling: there is no profiler in
// wasm, and this way every frame on the screen corresponds to a
// named function somebody chose to time, which is the honest
// version of a flame graph at this scale.
type Tracer struct {
	stack  []int
	frames []*frameAcc
	index  map[string]int
	on     bool
}

type frameAcc struct {
	name  string
	depth int
	total int64
	self  int64
	calls int
}

// Span times a phase. Nested calls nest in the graph.
func (t *Tracer) Span(name string, fn func()) {
	if t == nil || !t.on {
		fn()
		return
	}
	depth := len(t.stack)
	key := fmt.Sprintf("%d/%s", depth, name)
	idx, ok := t.index[key]
	if !ok {
		idx = len(t.frames)
		t.index[key] = idx
		t.frames = append(t.frames, &frameAcc{name: name, depth: depth})
	}
	t.stack = append(t.stack, idx)
	start := time.Now()
	fn()
	elapsed := time.Since(start).Nanoseconds()
	t.stack = t.stack[:len(t.stack)-1]

	f := t.frames[idx]
	f.total += elapsed
	f.self += elapsed
	f.calls++
	// Time spent here is not self time for whoever called us.
	if len(t.stack) > 0 {
		t.frames[t.stack[len(t.stack)-1]].self -= elapsed
	}
}

func newTracer(on bool) *Tracer {
	return &Tracer{index: map[string]int{}, on: on}
}

// Run times one workload and returns everything § 16 draws.
func Run(w Workload, samples int) Result {
	if samples < 1 {
		samples = 1
	}
	if w.MaxSamples > 0 && samples > w.MaxSamples {
		samples = w.MaxSamples
	}

	if w.Setup != nil {
		w.Setup()
	}
	// One warm-up iteration, untimed. The first call through any
	// path pays for lazily-initialised state, and letting that
	// land in the histogram would put a single outlier in p99.9
	// and invite a story about GC that was really about warm-up.
	w.Run(newTracer(false), -1)

	// Find the clock's granularity, then batch against it.
	clock := clockResolution()
	batch := calibrate(w, clock)

	// A wall-clock budget, and the count actually reached is
	// reported rather than the count asked for.
	//
	// wasm runs on the page's own thread: a benchmark that
	// overruns does not "take longer" — it holds the thread it
	// is drawing into. Bounding the run is therefore part of
	// the screen working, and saying which bound was hit —
	// samples or seconds — is part of the numbers meaning
	// anything.
	times := make([]float64, 0, samples)
	runStart := time.Now()
	budgeted := false
	for i := 0; i < samples; i++ {
		start := time.Now()
		for j := 0; j < batch; j++ {
			w.Run(nil, i*batch+j)
		}
		times = append(times, float64(time.Since(start).Nanoseconds())/float64(batch))
		if time.Since(runStart) > budget {
			budgeted = true
			break
		}
	}
	total := time.Since(runStart).Nanoseconds()

	// The call tree is walked on its own iterations: timing every
	// span on every iteration would measure the tracer.
	tr := newTracer(true)
	traceRuns := min(samples, 64)
	for i := 0; i < traceRuns; i++ {
		tr.Span(w.Name, func() { w.Run(tr, i) })
	}

	res := Result{
		Workload: w.Name, Samples: samples, Ran: len(times), Budgeted: budgeted,
		TotalNs: total, Note: w.Note, BatchSize: batch,
		ClockResolutionNs: clock,
		Frames:            framesOf(tr),
	}
	if len(times) == 0 {
		res.Buckets = []Bucket{}
		return res
	}

	sorted := append([]float64(nil), times...)
	sort.Float64s(sorted)
	res.Min, res.Max = sorted[0], sorted[len(sorted)-1]
	res.P50 = quantile(sorted, 0.50)
	res.P95 = quantile(sorted, 0.95)
	res.P99 = quantile(sorted, 0.99)
	res.P999 = quantile(sorted, 0.999)
	var sum float64
	for _, v := range sorted {
		sum += v
	}
	res.Mean = sum / float64(len(sorted))
	res.Buckets = bucketise(sorted)
	// A quantile is supported when at least one observation
	// sits above it — otherwise it is the maximum by
	// another name.
	res.Supported = 0
	for _, q := range []float64{0.5, 0.95, 0.99, 0.999} {
		if float64(len(sorted))*(1-q) >= 1 {
			res.Supported = q
		}
	}
	return res
}

// quantile is nearest-rank over a sorted slice. Nearest-rank
// rather than interpolated: an interpolated p99 reports a
// duration that never happened, which on a latency histogram is
// exactly the number people quote.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(q*float64(len(sorted)))) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

// clockResolution is the smallest non-zero interval this host's
// clock will actually report, found by reading it until it moves.
//
// Measured rather than assumed: it is nanoseconds natively,
// ~100 ns to 5 µs in most browsers, and coarser still where
// cross-origin isolation is off. Everything else here depends
// on it, so it is the first thing established.
func clockResolution() int64 {
	best := int64(0)
	for i := 0; i < 32; i++ {
		start := time.Now()
		var d time.Duration
		for d == 0 {
			d = time.Since(start)
		}
		if ns := d.Nanoseconds(); ns > 0 && (best == 0 || ns < best) {
			best = ns
		}
	}
	if best <= 0 {
		best = 1
	}
	return best
}

// calibrate picks a batch size whose duration the clock can
// actually resolve — the same thing testing.B does when it
// raises b.N, and for the same reason.
//
// The target is 100× the resolution, CAPPED at maxBatchNs.
// Uncapped it is a trap: this host reported 99.8 µs
// granularity, which asks for a 10 ms batch, which for a
// 1.6 µs workload is 16 384 iterations per sample — and at
// 10 000 samples that is 164 million calls and four
// minutes of a frozen tab. The cap costs a little
// quantisation noise at the far tail and keeps the screen
// usable, which is the right trade for a screen.
func calibrate(w Workload, clock int64) int {
	// 20 ticks or 1 ms, whichever is SMALLER — and the
	// smaller one is the point.
	//
	// A coarse clock forces a real trade. A longer batch
	// is more accurate per sample and buys FEWER samples
	// inside the run's budget; at 100 ticks this screen
	// gathered 16 of them, and a p99.9 over 16 samples is
	// the maximum wearing a percentile's name. Capping the
	// batch costs quantisation (~10% of one sample at a
	// 100 µs clock) and buys back two orders of magnitude
	// more samples. The error is stated on screen, and
	// `Supported` says which quantiles the count can
	// actually carry — both halves of the trade, visible.
	target := min(clock*20, maxBatchNs)
	batch := 1
	for batch < maxBatch {
		// The MINIMUM of three probes, not one.
		//
		// A single read from a coarse clock is not evidence:
		// on this host one `Page.Apply` measured 2 ms —
		// not because it took 2 ms, but because a ~100 µs
		// clock jumped mid-call. Believing it returned a
		// batch of 1, whose samples were then quantised to 0
		// or 99.8 µs, and a p50 of "0.00 ns". The minimum of
		// several probes is the least contaminated one:
		// quantisation and scheduling noise only ever push a
		// measurement UP.
		best := int64(math.MaxInt64)
		for probe := 0; probe < 3; probe++ {
			start := time.Now()
			for j := 0; j < batch; j++ {
				w.Run(nil, j)
			}
			if ns := time.Since(start).Nanoseconds(); ns < best {
				best = ns
			}
		}
		if best >= target {
			return batch
		}
		batch *= 4
	}
	return maxBatch
}

const (
	bucketCount = 12
	// maxBatchNs caps how long one timed sample may take. See
	// calibrate.
	maxBatchNs = int64(1 * time.Millisecond)
	maxBatch   = 1 << 14
	// budget bounds the whole measured run. wasm holds the
	// page's thread, so this is a responsiveness limit, not a
	// nicety.
	budget = 2 * time.Second
)

// bucketise puts the samples in log-spaced buckets spanning the
// observed range.
//
// Log-spaced because latency is: the interesting structure lives
// between 1 µs and 100 ms, and linear buckets over that range
// put every sample in the first one. The span is the observed
// min..max rather than a fixed 0.5 ms..256 ms, so a fast machine
// does not get eleven empty buckets and one full one.
func bucketise(sorted []float64) []Bucket {
	lo, hi := sorted[0], sorted[len(sorted)-1]
	if lo < 1 {
		lo = 1
	}
	if hi <= lo {
		hi = lo * 2
	}
	logLo, logHi := math.Log(lo), math.Log(hi)
	step := (logHi - logLo) / float64(bucketCount)

	out := make([]Bucket, bucketCount)
	for i := range out {
		bLo := int64(math.Exp(logLo + step*float64(i)))
		bHi := int64(math.Exp(logLo + step*float64(i+1)))
		out[i] = Bucket{LoNs: bLo, HiNs: bHi, Label: Duration(float64(bLo))}
	}
	for _, v := range sorted {
		i := bucketCount - 1
		if v < hi {
			i = int((math.Log(math.Max(v, 1)) - logLo) / step)
		}
		if i < 0 {
			i = 0
		}
		if i >= bucketCount {
			i = bucketCount - 1
		}
		out[i].Count++
	}
	return out
}

func framesOf(t *Tracer) []Frame {
	out := make([]Frame, 0, len(t.frames))
	var rootTotal int64
	for _, f := range t.frames {
		if f.depth == 0 {
			rootTotal += f.total
		}
	}
	for _, f := range t.frames {
		frac := 0.0
		if rootTotal > 0 {
			frac = float64(f.total) / float64(rootTotal)
		}
		out = append(out, Frame{
			Name: f.name, Depth: f.depth, SelfNs: max(f.self, 0),
			TotalNs: f.total, Fraction: frac, Calls: f.calls,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].TotalNs > out[j].TotalNs
	})
	return out
}

// Duration prints a nanosecond count the way § 16 does: a
// unit and a couple of significant figures, never "0s".
//
// Sub-nanosecond values keep a decimal rather than
// rounding to "0 ns" — after dividing a batch total by its
// size, 0.4 ns is a real measurement and 0 is not.
func Duration(ns float64) string {
	switch {
	case ns >= 1e9:
		return fmt.Sprintf("%.2f s", ns/1e9)
	case ns >= 1e6:
		return fmt.Sprintf("%.1f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.1f µs", ns/1e3)
	case ns >= 10:
		return fmt.Sprintf("%.0f ns", ns)
	default:
		return fmt.Sprintf("%.2f ns", ns)
	}
}
