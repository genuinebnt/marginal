package sketch

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// Event is one row of § 12's editable stream.
//
// Four fields and no more, because these are the only dimensions a person
// actually chose: who (hashed into a sketch and never stored), what they read,
// what it was about, and how long they stayed. Anything else would be a field
// that exists to be sliced by rather than one anybody set.
type Event struct {
	Actor string  `json:"actor"`
	Page  string  `json:"page"`
	Topic string  `json:"topic"`
	Ms    float64 `json:"ms"`
	// Tags the page carries. Space-separated in the buffer's fifth field.
	// A tag is in here for the same reason a topic is — somebody chose it.
	Tags []string `json:"tags"`
}

// ParseStream reads the editable buffer: one event per line,
// `actor, page, topic, ms, tag tag tag`. Trailing fields may be omitted.
//
// Malformed lines are SKIPPED and counted, never fatal. This is a text box a
// person is typing into — half a line is the normal state of it, and a parser
// that refused the whole stream mid-keystroke would make the screen unusable
// exactly while it is being used.
func ParseStream(src string) (events []Event, skipped int) {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			skipped++
			continue
		}
		e := Event{Actor: parts[0], Page: parts[1]}
		if len(parts) > 2 {
			e.Topic = parts[2]
		}
		if len(parts) > 3 && parts[3] != "" {
			if ms, err := strconv.ParseFloat(parts[3], 64); err == nil {
				e.Ms = ms
			} else {
				skipped++
				continue
			}
		}
		if len(parts) > 4 {
			e.Tags = strings.Fields(parts[4])
		}
		events = append(events, e)
	}
	return events, skipped
}

// Report is every panel on § 12, with each sketch's answer beside the exact
// one and the error between them.
//
// The exact answers are computed here, from the same events, using ordinary
// maps and a sort — which is the honest way to make the comparison: the
// screen is not claiming the sketch is right, it is showing you both numbers
// and letting you see the gap.
type Report struct {
	Events  int `json:"events"`
	Skipped int `json:"skipped"`

	// Distinct actors: the sketch, the truth, and the gap.
	HLLEstimate      float64 `json:"hll_estimate"`
	HLLExact         int     `json:"hll_exact"`
	HLLErrorPct      float64 `json:"hll_error_pct"`
	HLLStandardError float64 `json:"hll_standard_error"`
	// []int, not the []uint8 the structure holds: encoding/json renders a
	// []byte as a base64 STRING, which reaches the browser as something the
	// register chart cannot draw. Widened at the boundary, on purpose.
	HLLRegisters []int `json:"hll_registers"`
	HLLBytes     int   `json:"hll_bytes"`

	// Per-page counts. Estimate is the sketch, Exact the map.
	Heavy      []Counted `json:"heavy"`
	CMDepth    int       `json:"cm_depth"`
	CMWidth    int       `json:"cm_width"`
	CMBytes    int       `json:"cm_bytes"`
	CMOverEst  int       `json:"cm_over_estimates"`
	CMUnderEst int       `json:"cm_under_estimates"`

	// Session length quantiles.
	P50          float64    `json:"p50"`
	P95          float64    `json:"p95"`
	P99          float64    `json:"p99"`
	ExactP50     float64    `json:"exact_p50"`
	ExactP95     float64    `json:"exact_p95"`
	ExactP99     float64    `json:"exact_p99"`
	Centroids    []Centroid `json:"centroids"`
	TDigestBytes int        `json:"tdigest_bytes"`

	// Distinct readers per topic — one HLL each, which is what makes the
	// per-topic numbers cost 2 KB rather than a row per reader per topic.
	ByTopic []TopicReaders `json:"by_topic"`

	// Momentum per tag, strongest movement first.
	Momentum []TagMomentum `json:"momentum"`

	// TotalBytes is every sketch added up: the number § 12 puts in its top
	// bar, and the one that does not grow with the stream.
	TotalBytes int `json:"total_bytes"`
	// ExactBytes is what storing the raw stream would have cost, for
	// comparison. The ratio is the argument.
	ExactBytes int `json:"exact_bytes"`
}

// TopicReaders is one topic's distinct-reader estimate.
type TopicReaders struct {
	Topic    string  `json:"topic"`
	Estimate float64 `json:"estimate"`
	Exact    int     `json:"exact"`
	// DeltaPct is the second half of the buffer against the first, in
	// percent. § 12 labels this "7 d vs prior 7 d"; at a text box's scale
	// the two windows are the two halves of what you typed, which is the
	// same comparison with an honest window.
	DeltaPct float64 `json:"delta_pct"`
}

// TagMomentum is § 12's TAG MOMENTUM row: reads of pages carrying this tag in
// the buffer's second half against its first.
//
// Momentum is reads, not writes — a tag can climb while nobody edits it, which
// is exactly the week to check whether what it points at is still true.
type TagMomentum struct {
	Tag      string  `json:"tag"`
	Recent   int     `json:"recent"`
	Prior    int     `json:"prior"`
	DeltaPct float64 `json:"delta_pct"`
}

// Analyze runs all three sketches over the stream and computes the exact
// answers beside them.
//
// The parameters are small on purpose — 64 HLL registers and a 4×24 Count-Min
// table, which are § 12's own stated dimensions. At a notebook's scale that is
// enough to be accurate and small enough that the error is VISIBLE when you
// push it: paste a few hundred distinct actors into the buffer and the HLL
// starts to drift, which is the demonstration. A structure tuned so far past
// its input that it is always exact teaches nothing.
func Analyze(events []Event) Report {
	const (
		hllPrecision = 6 // 2^6 = 64 registers
		cmDepth      = 4
		cmWidth      = 24
		compression  = 60
	)

	hll := NewHLL(hllPrecision)
	cm := NewCountMin(cmDepth, cmWidth)
	td := NewTDigest(compression)

	exactActors := map[string]bool{}
	exactPages := map[string]uint32{}
	topicHLL := map[string]*HLL{}
	topicExact := map[string]map[string]bool{}
	var durations []float64

	// The two windows every "vs prior" figure on § 12 compares. Splitting the
	// buffer in half is the only window a text box actually has, and saying so
	// is better than quoting a seven-day figure the input cannot support.
	half := len(events) / 2
	topicWindow := map[string][2]int{}
	tagWindow := map[string][2]int{}

	for i, e := range events {
		window := 0
		if i >= half {
			window = 1
		}
		if e.Topic != "" {
			w := topicWindow[e.Topic]
			w[window]++
			topicWindow[e.Topic] = w
		}
		for _, tag := range e.Tags {
			w := tagWindow[tag]
			w[window]++
			tagWindow[tag] = w
		}
	}

	for _, e := range events {
		hll.Add(e.Actor)
		exactActors[e.Actor] = true

		cm.Add(e.Page, 1)
		exactPages[e.Page]++

		if e.Ms > 0 {
			td.Add(e.Ms)
			durations = append(durations, e.Ms)
		}

		if e.Topic != "" {
			if topicHLL[e.Topic] == nil {
				topicHLL[e.Topic] = NewHLL(hllPrecision)
				topicExact[e.Topic] = map[string]bool{}
			}
			topicHLL[e.Topic].Add(e.Actor)
			topicExact[e.Topic][e.Actor] = true
		}
	}

	r := Report{
		Events:           len(events),
		HLLEstimate:      hll.Estimate(),
		HLLExact:         len(exactActors),
		HLLStandardError: hll.StandardError() * 100,
		HLLRegisters:     widen(hll.Registers()),
		HLLBytes:         hll.Bytes(),
		CMDepth:          cmDepth,
		CMWidth:          cmWidth,
		CMBytes:          cm.Bytes(),
		Centroids:        td.Centroids(),
		TDigestBytes:     td.Bytes(),
	}
	if r.HLLExact > 0 {
		r.HLLErrorPct = math.Abs(r.HLLEstimate-float64(r.HLLExact)) / float64(r.HLLExact) * 100
	}

	// Heavy pages: the sketch's ranking, with the true counts beside it. The
	// candidate set is the distinct pages seen — a Count-Min cannot enumerate
	// its own keys, which is the same fact as the privacy property from the
	// query side.
	candidates := make([]string, 0, len(exactPages))
	for p := range exactPages {
		candidates = append(candidates, p)
	}
	sort.Strings(candidates)
	r.Heavy = cm.TopK(candidates, 8)
	for i := range r.Heavy {
		r.Heavy[i].Exact = exactPages[r.Heavy[i].Key]
		switch {
		case r.Heavy[i].Estimate > r.Heavy[i].Exact:
			r.CMOverEst++
		case r.Heavy[i].Estimate < r.Heavy[i].Exact:
			// Must never happen. Counted rather than asserted, so the screen
			// can show a zero that means something.
			r.CMUnderEst++
		}
	}

	if len(durations) > 0 {
		r.P50, r.P95, r.P99 = td.Quantile(0.5), td.Quantile(0.95), td.Quantile(0.99)
		sort.Float64s(durations)
		r.ExactP50 = exactQuantile(durations, 0.5)
		r.ExactP95 = exactQuantile(durations, 0.95)
		r.ExactP99 = exactQuantile(durations, 0.99)
	}

	for topic, h := range topicHLL {
		w := topicWindow[topic]
		r.ByTopic = append(r.ByTopic, TopicReaders{
			Topic: topic, Estimate: h.Estimate(), Exact: len(topicExact[topic]),
			DeltaPct: deltaPct(w[0], w[1]),
		})
	}
	sort.Slice(r.ByTopic, func(i, j int) bool {
		if r.ByTopic[i].Estimate != r.ByTopic[j].Estimate {
			return r.ByTopic[i].Estimate > r.ByTopic[j].Estimate
		}
		return r.ByTopic[i].Topic < r.ByTopic[j].Topic
	})

	for tag, w := range tagWindow {
		r.Momentum = append(r.Momentum, TagMomentum{
			Tag: tag, Prior: w[0], Recent: w[1], DeltaPct: deltaPct(w[0], w[1]),
		})
	}
	sort.Slice(r.Momentum, func(i, j int) bool {
		a, b := r.Momentum[i], r.Momentum[j]
		if math.Abs(a.DeltaPct) != math.Abs(b.DeltaPct) {
			return math.Abs(a.DeltaPct) > math.Abs(b.DeltaPct)
		}
		return a.Tag < b.Tag
	})
	if len(r.Momentum) > 6 {
		r.Momentum = r.Momentum[:6]
	}

	r.TotalBytes = r.HLLBytes + r.CMBytes + r.TDigestBytes
	for _, h := range topicHLL {
		r.TotalBytes += h.Bytes()
	}
	// What the raw stream would have cost: one row per event, actor and page
	// stored. The ratio is the argument for sketching at all.
	for _, e := range events {
		r.ExactBytes += len(e.Actor) + len(e.Page) + len(e.Topic) + 8
	}
	return r
}

// widen turns the register bytes into the ints the JSON boundary needs.
func widen(rs []uint8) []int {
	out := make([]int, len(rs))
	for i, r := range rs {
		out[i] = int(r)
	}
	return out
}

// deltaPct is the second window against the first. A tag that did not exist
// before is +100%, not an infinity — the screen has to print it.
func deltaPct(prior, recent int) float64 {
	switch {
	case prior == 0 && recent == 0:
		return 0
	case prior == 0:
		return 100
	default:
		return (float64(recent) - float64(prior)) / float64(prior) * 100
	}
}

// exactQuantile is the nearest-rank quantile over a sorted slice — the truth
// the sketch is measured against.
func exactQuantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
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
