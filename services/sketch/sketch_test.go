package sketch_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/sketch"
)

// ── HyperLogLog ────────────────────────────────────────────────────────────

// The property the whole structure exists for, and the one § 12 demonstrates
// by letting you paste a line twice: a repeat cannot move the estimate,
// because it hashes to the same register with the same rank.
func TestAddingADuplicateCannotChangeTheEstimate(t *testing.T) {
	h := sketch.NewHLL(8)
	for i := 0; i < 500; i++ {
		h.Add(fmt.Sprintf("actor-%d", i))
	}
	before := h.Estimate()
	for i := 0; i < 500; i++ {
		h.Add(fmt.Sprintf("actor-%d", i)) // every one already seen
	}
	assert.Equal(t, before, h.Estimate())
}

// The accuracy claim, checked against the structure's OWN promised bound
// rather than a number picked to pass. 1.04/√m is what HLL guarantees; a
// tolerance looser than that would be a test that cannot fail.
func TestEstimateStaysInsideTheStandardErrorBound(t *testing.T) {
	for _, n := range []int{100, 1000, 10000, 100000} {
		h := sketch.NewHLL(12)
		for i := 0; i < n; i++ {
			h.Add(fmt.Sprintf("k-%d", i))
		}
		err := math.Abs(h.Estimate()-float64(n)) / float64(n)
		// Three standard errors: the bound is a standard deviation, so a
		// single run outside 1σ is expected and outside 3σ is a bug.
		assert.Less(t, err, 3*h.StandardError(),
			"n=%d estimate=%.0f err=%.3f%% bound=%.3f%%", n, h.Estimate(), err*100, h.StandardError()*100)
	}
}

// Small cardinalities are where the raw harmonic estimator is badly biased and
// linear counting takes over. At a notebook's scale this IS the common case,
// so it gets its own test rather than riding on the large-n one.
func TestSmallCardinalitiesAreNearlyExact(t *testing.T) {
	h := sketch.NewHLL(10)
	for i := 0; i < 20; i++ {
		h.Add(fmt.Sprintf("a-%d", i))
	}
	assert.InDelta(t, 20, h.Estimate(), 2, "linear counting should be near-exact here")
}

// The property that makes HLL usable across machines at all: two sketches
// merge to exactly the union, with no coordination and no double counting of
// elements both saw. Nothing else in this package has it.
func TestMergeIsTheUnionIncludingTheOverlap(t *testing.T) {
	a, b, both := sketch.NewHLL(10), sketch.NewHLL(10), sketch.NewHLL(10)
	for i := 0; i < 800; i++ {
		k := fmt.Sprintf("k-%d", i)
		a.Add(k)
		both.Add(k)
	}
	for i := 400; i < 1200; i++ { // 400 shared, deliberately
		k := fmt.Sprintf("k-%d", i)
		b.Add(k)
		both.Add(k)
	}
	a.Merge(b)
	assert.InDelta(t, both.Estimate(), a.Estimate(), 1e-9,
		"a merged sketch must equal one that saw everything")
}

func TestAnEmptySketchEstimatesZero(t *testing.T) {
	assert.Equal(t, 0.0, sketch.NewHLL(8).Estimate())
}

// ── Count-Min ──────────────────────────────────────────────────────────────

// THE HARDEST AND MOST IMPORTANT TEST HERE. The structure's whole safety
// argument is one-sidedness: an overestimate puts a page on a top-k that does
// not belong, which you notice; an underestimate silently omits one that
// does, which you do not.
func TestCountMinNeverUnderestimates(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	// Deliberately undersized, so collisions are frequent and the property is
	// actually under pressure rather than trivially true.
	cm := sketch.NewCountMin(4, 16)
	exact := map[string]uint32{}
	for i := 0; i < 5000; i++ {
		k := fmt.Sprintf("page-%d", rng.Intn(300))
		cm.Add(k, 1)
		exact[k]++
	}
	for k, want := range exact {
		assert.GreaterOrEqual(t, cm.Estimate(k), want, "key %q underestimated", k)
	}
}

// With a table far wider than the key set, there are no collisions and the
// sketch is exact — which is what makes the error on § 12 attributable to
// collisions rather than to the algorithm.
func TestAWideEnoughTableIsExact(t *testing.T) {
	cm := sketch.NewCountMin(5, 4096)
	for i := 0; i < 40; i++ {
		for j := 0; j <= i; j++ {
			cm.Add(fmt.Sprintf("k-%d", i), 1)
		}
	}
	for i := 0; i < 40; i++ {
		assert.Equal(t, uint32(i+1), cm.Estimate(fmt.Sprintf("k-%d", i)))
	}
}

func TestAnUnseenKeyEstimatesZeroWhenThereIsRoom(t *testing.T) {
	cm := sketch.NewCountMin(5, 4096)
	cm.Add("seen", 3)
	assert.Equal(t, uint32(0), cm.Estimate("never seen"))
}

func TestTopKIsStableAcrossRuns(t *testing.T) {
	// Ties break on the key, or the "heavy pages" list reshuffles between two
	// renders of the same data.
	cm := sketch.NewCountMin(4, 512)
	for _, k := range []string{"a", "b", "c", "d"} {
		cm.Add(k, 5) // all equal, so ONLY the tie rule orders them
	}
	first := cm.TopK([]string{"d", "c", "b", "a"}, 4)
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, cm.TopK([]string{"b", "a", "d", "c"}, 4))
	}
}

// ── t-digest ───────────────────────────────────────────────────────────────

// T1. A p95 below the p50 is not "a bit inaccurate", it is a number nobody
// can act on — and it is exactly what a wrong interpolation produces.
func TestQuantilesAreMonotoneInQ(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	td := sketch.NewTDigest(100)
	for i := 0; i < 20000; i++ {
		td.Add(rng.ExpFloat64() * 1000)
	}
	last := math.Inf(-1)
	for q := 0.0; q <= 1.0; q += 0.01 {
		v := td.Quantile(q)
		assert.GreaterOrEqual(t, v, last, "q=%.2f went backwards", q)
		last = v
	}
}

// The tails are where the accuracy is supposed to be, so that is where the
// tolerance is tightest. A digest that was accurate in the middle and vague
// at p99 would be a histogram with extra steps.
func TestTailQuantilesAreAccurate(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	td := sketch.NewTDigest(100)
	var all []float64
	for i := 0; i < 50000; i++ {
		x := rng.ExpFloat64() * 1000
		td.Add(x)
		all = append(all, x)
	}
	sort.Float64s(all)
	for _, q := range []float64{0.5, 0.9, 0.95, 0.99} {
		want := all[int(q*float64(len(all)))-1]
		got := td.Quantile(q)
		rel := math.Abs(got-want) / want
		assert.Less(t, rel, 0.05, "q=%.2f want=%.1f got=%.1f", q, want, got)
	}
}

// The compression claim, stated as what it actually is: the centroid count is
// bounded INDEPENDENTLY of how much arrives. Asserting a fixed number would be
// a test tuned to one implementation's constant; asserting that ten times the
// input produces roughly the same number of centroids is the property.
//
// (It does grow a little: the extreme tails have a size limit below 1, so a
// handful of singleton centroids sit at each end, and how far "extreme" is
// depends on N. That is the structure keeping its promise about the tails,
// not a leak.)
func TestCentroidCountIsBoundedIndependentlyOfInputSize(t *testing.T) {
	count := func(n int) int {
		rng := rand.New(rand.NewSource(17))
		td := sketch.NewTDigest(100)
		for i := 0; i < n; i++ {
			td.Add(rng.Float64() * 1e6)
		}
		return len(td.Centroids())
	}
	small, large := count(20000), count(200000)
	assert.Greater(t, small, 10, "and it is not collapsing to nothing either")
	assert.Less(t, large, small*2,
		"10x the samples produced %d centroids against %d — that is not compression", large, small)
	assert.Less(t, large, 1000, "200k samples must not become 200k centroids")
}

func TestASingleSampleIsItsOwnEveryQuantile(t *testing.T) {
	td := sketch.NewTDigest(100)
	td.Add(42)
	assert.Equal(t, 42.0, td.Quantile(0.5))
	assert.Equal(t, 42.0, td.Quantile(0.99))
}

func TestAnEmptyDigestIsNaNNotZero(t *testing.T) {
	// Zero would be a legitimate measurement. NaN is the only honest answer
	// to "what is the median of nothing".
	assert.True(t, math.IsNaN(sketch.NewTDigest(100).Quantile(0.5)))
}

// ── the stream and the report ──────────────────────────────────────────────

func TestParseStreamSkipsHalfLinesRatherThanFailing(t *testing.T) {
	// This is a text box someone is typing into. Half a line is its normal
	// state, and refusing the whole stream mid-keystroke makes the screen
	// unusable exactly while it is being used.
	events, skipped := sketch.ParseStream(strings.Join([]string{
		"# a comment",
		"ada, Block model, storage, 900",
		"",
		"incomplete",
		"rina, Anchors, protocol",
		"bad, ms, protocol, not-a-number",
	}, "\n"))
	assert.Len(t, events, 2)
	assert.Equal(t, 2, skipped)
	assert.Equal(t, "ada", events[0].Actor)
	assert.Equal(t, 900.0, events[0].Ms)
	assert.Equal(t, 0.0, events[1].Ms, "a missing duration is zero, not an error")
}

func TestAnalyzeReportsBothAnswersAndNeverUnderestimatesAPage(t *testing.T) {
	rng := rand.New(rand.NewSource(23))
	var lines []string
	for i := 0; i < 4000; i++ {
		lines = append(lines, fmt.Sprintf("actor-%d, page-%d, topic-%d, %d",
			rng.Intn(400), rng.Intn(60), rng.Intn(5), 100+rng.Intn(9000)))
	}
	events, _ := sketch.ParseStream(strings.Join(lines, "\n"))
	r := sketch.Analyze(events)

	assert.Equal(t, 4000, r.Events)
	assert.Greater(t, r.HLLExact, 0)
	assert.Equal(t, 0, r.CMUnderEst, "the one-sided guarantee, over a real stream")
	assert.LessOrEqual(t, r.P50, r.P95)
	assert.LessOrEqual(t, r.P95, r.P99)
	assert.NotEmpty(t, r.ByTopic)

	// The argument for sketching at all: fixed memory against a stream that
	// is not.
	assert.Less(t, r.TotalBytes, r.ExactBytes/10,
		"sketches=%d bytes, raw stream=%d bytes", r.TotalBytes, r.ExactBytes)
}

func TestAnalyzeOfAnEmptyStreamIsAnEmptyReportNotACrash(t *testing.T) {
	r := sketch.Analyze(nil)
	assert.Equal(t, 0, r.Events)
	assert.Equal(t, 0.0, r.HLLEstimate)
	assert.Empty(t, r.Heavy)
	assert.Empty(t, r.ByTopic)
}

func TestAnalyzeIsDeterministic(t *testing.T) {
	events, _ := sketch.ParseStream("a, p, t, 1\nb, p, t, 2\na, q, u, 3")
	assert.Equal(t, sketch.Analyze(events), sketch.Analyze(events))
}

// § 12's TAG MOMENTUM and the topic deltas both claim a "vs prior window"
// comparison. The window is the buffer's two halves, and a tag only in the
// second half has to print as a number rather than an infinity.
func TestMomentumComparesTheTwoHalvesOfTheBuffer(t *testing.T) {
	var stream strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&stream, "a%d, old-page, protocol, 100, rope\n", i)
	}
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&stream, "b%d, new-page, storage, 100, crdt rope\n", i)
	}
	events, skipped := sketch.ParseStream(stream.String())
	require.Zero(t, skipped)
	report := sketch.Analyze(events)

	byTag := map[string]sketch.TagMomentum{}
	for _, m := range report.Momentum {
		byTag[m.Tag] = m
	}

	crdt, ok := byTag["crdt"]
	require.True(t, ok, "a tag that appears only in the second half must still be reported")
	assert.Equal(t, 0, crdt.Prior)
	assert.Equal(t, 10, crdt.Recent)
	assert.Equal(t, 100.0, crdt.DeltaPct, "no prior reads is +100%, not a division by zero")

	rope := byTag["rope"]
	assert.Equal(t, 10, rope.Prior)
	assert.Equal(t, 10, rope.Recent)
	assert.Zero(t, rope.DeltaPct, "a tag read evenly across both halves has not moved")
}

// The stream is typed by hand: a line may end after any field. Tags are the
// last one, so their absence must not cost the rest of the line.
func TestTagsAreOptionalAndDoNotBreakTheLine(t *testing.T) {
	events, skipped := sketch.ParseStream("ana, page-one\nbo, page-two, protocol\ncy, page-three, protocol, 500\ndi, page-four, protocol, 500, rope crdt\n")
	require.Zero(t, skipped)
	require.Len(t, events, 4)
	assert.Empty(t, events[0].Tags)
	assert.Equal(t, []string{"rope", "crdt"}, events[3].Tags)
}

// The Report crosses a JSON boundary to reach § 12, and two things about it
// are only wrong on the far side: a []byte marshals as a base64 STRING, and a
// grouped field declaration gives every name the same tag, which makes
// encoding/json drop all of them. Both shipped once. Tested at the boundary
// because neither is visible from Go.
func TestTheReportSurvivesJSON(t *testing.T) {
	events, _ := sketch.ParseStream("ana, one, protocol, 1000, rope\nbo, two, storage, 5000, crdt\ncy, one, protocol, 90000, rope\n")
	data, err := json.Marshal(sketch.Analyze(events))
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))

	regs, ok := wire["hll_registers"].([]any)
	require.True(t, ok, "registers must be an array of numbers, not a base64 string: %v", wire["hll_registers"])
	assert.Len(t, regs, 64)

	for _, key := range []string{"p50", "p95", "p99", "exact_p50", "exact_p95", "exact_p99"} {
		v, present := wire[key]
		require.True(t, present, "%s never reached the wire", key)
		assert.Positive(t, v, "%s arrived as %v", key, v)
	}
}
