package bench_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/bench"
)

// Every workload must actually run. A benchmark screen whose
// fourth option panics is worse than one with three.
func TestEveryWorkloadRuns(t *testing.T) {
	for _, w := range bench.Workloads() {
		t.Run(w.Name, func(t *testing.T) {
			r := bench.Run(w, 50)
			assert.Equal(t, w.Name, r.Workload)
			assert.Positive(t, r.Ran)
			assert.Positive(t, r.TotalNs, "the whole run took zero time")
			assert.NotEmpty(t, r.Frames, "no spans were recorded")
			assert.NotEmpty(t, r.Note, "a workload that does not say what it did")
		})
	}
}

func TestPercentilesAreOrdered(t *testing.T) {
	r := bench.Run(bench.ByName("applyOp"), 500)
	assert.LessOrEqual(t, r.Min, r.P50)
	assert.LessOrEqual(t, r.P50, r.P95)
	assert.LessOrEqual(t, r.P95, r.P99)
	assert.LessOrEqual(t, r.P99, r.P999)
	assert.LessOrEqual(t, r.P999, r.Max)
}

// Nearest-rank, not interpolated: an interpolated p99 reports a
// duration that never happened, which on a latency histogram is
// exactly the number people go on to quote.
func TestQuantilesAreRealObservations(t *testing.T) {
	r := bench.Run(bench.ByName("applyOp"), 300)
	// Every reported percentile must sit inside the observed range.
	for _, v := range []float64{r.P50, r.P95, r.P99, r.P999} {
		assert.GreaterOrEqual(t, v, r.Min)
		assert.LessOrEqual(t, v, r.Max)
	}
}

// The histogram must account for every sample. A bucket set that
// silently drops the tail is how a p99.9 disappears.
func TestEverySampleLandsInABucket(t *testing.T) {
	r := bench.Run(bench.ByName("compilePaste"), 400)
	total := 0
	for _, b := range r.Buckets {
		total += b.Count
		assert.LessOrEqual(t, b.LoNs, b.HiNs)
	}
	assert.Equal(t, r.Ran, total, "%d samples ran, %d were bucketed", r.Ran, total)
}

// A frame's self time is its own, never its children's. Getting
// this backwards makes every flame graph a picture of the root.
func TestSelfTimeExcludesChildren(t *testing.T) {
	r := bench.Run(bench.ByName("applyOp"), 100)
	var root *bench.Frame
	for i := range r.Frames {
		if r.Frames[i].Depth == 0 {
			root = &r.Frames[i]
		}
	}
	require.NotNil(t, root)
	assert.LessOrEqual(t, root.SelfNs, root.TotalNs)
	var childTotal int64
	for _, f := range r.Frames {
		if f.Depth == 1 {
			childTotal += f.TotalNs
		}
	}
	assert.LessOrEqual(t, childTotal, root.TotalNs+root.TotalNs/10,
		"children claim more time than the call that contains them")
}

// A workload whose one iteration is milliseconds must not be
// asked for 50 000 of them because a chip on screen says so.
func TestExpensiveWorkloadsClampTheSampleCount(t *testing.T) {
	r := bench.Run(bench.ByName("simulate"), 1_000_000)
	assert.Less(t, r.Samples, 1_000_000)
	assert.Equal(t, r.Samples, r.Ran, "it must report what it actually ran")
}

// The same nil-slice rule § 14 learned the hard way.
func TestNoSliceReachesTheWireAsNull(t *testing.T) {
	data, err := json.Marshal(bench.Run(bench.ByName("applyOp"), 10))
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))
	for _, key := range []string{"buckets", "frames"} {
		assert.NotNil(t, wire[key], "%s arrived as null — the screen maps over it", key)
	}
}

func TestDurationNeverPrintsZero(t *testing.T) {
	for _, ns := range []float64{0, 0.4, 1, 999, 1000, 1_500_000, 2_000_000_000} {
		assert.NotEmpty(t, bench.Duration(ns))
	}
	assert.Equal(t, "1.5 ms", bench.Duration(1_500_000))
	assert.Equal(t, "1.0 µs", bench.Duration(1000))
	assert.Equal(t, "0.40 ns", bench.Duration(0.4),
		"sub-nanosecond is a measurement; rounding it to 0 reads as no measurement")
	assert.Equal(t, "0.00 ns", bench.Duration(0))
}

// Setup runs once, not per iteration. This is the whole
// difference between "one op costs 6 µs" and "one op costs
// 400 µs" — the second number is sixty InsertBlocks and
// looks entirely plausible.
func TestSetupIsNotTimed(t *testing.T) {
	setups, runs := 0, 0
	w := bench.Workload{
		Name:  "counted",
		Note:  "counts its own calls",
		Setup: func() { setups++ },
		Run:   func(_ *bench.Tracer, _ int) { runs++ },
	}
	bench.Run(w, 200)
	assert.Equal(t, 1, setups, "setup ran %d times", setups)
	// 200 timed + 1 warm-up + up to 64 traced.
	assert.Greater(t, runs, 200)
}

// A browser's clock is deliberately coarsened, and most of
// these workloads are faster than it. Timing single
// iterations there yields a histogram of zeroes and a p50 of
// "0 ns" — which is not a fast result, it is no result. The
// harness must batch until the clock can see the work.
func TestFastWorkloadsAreBatchedUntilTheClockCanSeeThem(t *testing.T) {
	fast := bench.Workload{
		Name: "trivial",
		Note: "one integer add",
		Run:  func(_ *bench.Tracer, i int) { _ = i * 3 },
	}
	r := bench.Run(fast, 200)
	assert.Greater(t, r.BatchSize, 1,
		"a one-instruction workload was timed one iteration at a time")
	assert.Positive(t, r.P50, "p50 came back as zero — the clock saw nothing")
	assert.Positive(t, r.ClockResolutionNs)
}

// A slow workload needs no batching, and inflating it would
// make each sample a mean over work that was already
// individually measurable.
func TestSlowWorkloadsAreNotBatched(t *testing.T) {
	r := bench.Run(bench.ByName("simulate"), 30)
	assert.Equal(t, 1, r.BatchSize, "batched work the clock could already resolve")
}

// wasm holds the page's own thread, so a benchmark that
// overruns does not take longer — it freezes the tab it
// is drawing into. The bound is part of the screen
// working.
func TestARunIsBoundedByWallClock(t *testing.T) {
	slow := bench.Workload{
		Name: "slow",
		Note: "sleeps",
		Run:  func(_ *bench.Tracer, _ int) { time.Sleep(2 * time.Millisecond) },
	}
	start := time.Now()
	r := bench.Run(slow, 100_000)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 8*time.Second, "the run did not stop on its budget")
	assert.True(t, r.Budgeted, "it stopped early and did not say so")
	assert.Less(t, r.Ran, 100_000)
	assert.Positive(t, r.Ran, "it must still report what it did measure")
}

// The batch cap is the other half of the same bound: a
// coarse host clock asks for a batch big enough to make
// one sample take minutes.
func TestBatchSizeIsCapped(t *testing.T) {
	trivial := bench.Workload{
		Name: "trivial",
		Note: "one add",
		Run:  func(_ *bench.Tracer, i int) { _ = i * 3 },
	}
	r := bench.Run(trivial, 500)
	assert.LessOrEqual(t, r.BatchSize, 1<<14)
}

// Calibration must not believe a single read from a coarse
// clock. One `Page.Apply` on a real browser measured 2 ms —
// the clock jumped mid-call — which returned a batch of 1,
// samples quantised to 0, and a p50 of "0.00 ns". The
// minimum of several probes is the least contaminated:
// noise only ever pushes a measurement up.
func TestCalibrationIgnoresASingleWildRead(t *testing.T) {
	call := 0
	spiky := bench.Workload{
		Name: "spiky",
		Note: "fast, with one 5 ms outlier early",
		Run: func(_ *bench.Tracer, _ int) {
			call++
			if call == 2 {
				time.Sleep(5 * time.Millisecond)
			}
		},
	}
	r := bench.Run(spiky, 200)
	assert.Greater(t, r.BatchSize, 1,
		"one slow call convinced it a single iteration was measurable")
}

// A p99.9 over 29 samples is the maximum wearing a
// percentile's name. The result says which quantiles its
// sample count can actually support, so the screen can
// stop pretending about the rest.
func TestItSaysWhichQuantilesTheSampleCountSupports(t *testing.T) {
	few := bench.Run(bench.ByName("simulate"), 20)
	assert.LessOrEqual(t, few.Supported, 0.95,
		"20 samples cannot support a p99, let alone a p99.9")

	many := bench.Run(bench.Workload{
		Name: "cheap", Note: "an add",
		Run: func(_ *bench.Tracer, i int) { _ = i * 3 },
	}, 5000)
	assert.GreaterOrEqual(t, many.Supported, 0.99)
}
