package bench

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"marginal/documentcore"
	"marginal/mdc"
	"marginal/netsim"
	"marginal/semantic"
)

// Workloads are the paths § 16 offers to measure.
//
// Every one is a function this repo actually runs in anger — the
// editor's op apply, the paste handler's compile, § 14's
// simulation, the search index's vector build. A benchmark of code
// nothing else calls measures the benchmark.
func Workloads() []Workload {
	return []Workload{applyOp(), compilePaste(), simulate(), embedIndex()}
}

// ByName finds a workload, falling back to the first rather than
// failing: the name arrives from a URL-ish control, and a
// benchmark screen that 500s on a typo is worse than one that
// runs its default.
func ByName(name string) Workload {
	all := Workloads()
	for _, w := range all {
		if w.Name == name {
			return w
		}
	}
	return all[0]
}

// applyOp is the editor's own hot path: one op against a real
// page tree, through documentcore.Page.Apply.
func applyOp() Workload {
	// Seeded once, in Setup — building the page is not what a
	// keystroke costs, and timing it here would say it is.
	var page documentcore.Page
	return Workload{
		Name:  "applyOp",
		Note:  "documentcore.Page.Apply — the editor's own path: one op against a 60-block page, the same call a keystroke makes. The page is built once, in setup.",
		Setup: func() { page = seedPage(60) },
		Run: func(t *Tracer, i int) {
			t.Span("applyOp", func() {
				mid := page.Blocks[len(page.Blocks)/2]
				t.Span("setBlockContent", func() {
					_ = page.Apply(documentcore.SetBlockContent{
						Block:   mid.ID,
						Prev:    mid.Content,
						Content: documentcore.PlainContent(fmt.Sprintf("edit %d", i)),
					})
				})
				t.Span("invert", func() {
					// Invertibility is not free and § 16 should say what it
					// costs — RFC-002 §3 makes every op carry what it
					// overwrites, and this is the price of that.
					_ = documentcore.SetBlockContent{Block: mid.ID}.Invert()
				})
			})
		},
	}
}

// compilePaste is the paste handler: markdown in, ops out, with
// the projection re-checked (§ 11's own pipeline).
func compilePaste() Workload {
	src := strings.Repeat("## Heading\n\nSome prose with **marks** and a [[Page Link]].\n\n- one\n- two\n\n```go\nfunc main() {}\n```\n\n", 6)
	return Workload{
		Name:       "compilePaste",
		Note:       "mdc.Compile — lex, parse, lower, emit, then replay the emitted ops and compare field by field. § 11's whole pipeline, per paste.",
		MaxSamples: 20000,
		Run: func(t *Tracer, _ int) {
			var r mdc.Result
			t.Span("compile", func() { r = mdc.Compile(src) })
			t.Span("checkProjection", func() {
				if !r.Holds {
					panic("mdc: projection failed inside a benchmark: " + r.Mismatch)
				}
			})
		},
	}
}

// simulate is § 14's own engine — the most expensive of the four,
// and the one whose cost is a design fact rather than a detail.
func simulate() Workload {
	edits, _ := netsim.ParseScenario(
		"0, you, insert, 10, quite \n40, ada, insert, 20, addressable \n120, you, delete, 2, 5")
	return Workload{
		Name:       "simulate",
		Note:       "netsim.Run — § 14's two-replica simulation over a 180 ms wire, including the transform-on reference run its intent ledger needs.",
		MaxSamples: 4000,
		Run: func(t *Tracer, i int) {
			t.Span("run", func() {
				_ = netsim.Run(netsim.Scenario{
					Wire:      netsim.Wire{RTTMs: 180, LossPct: 4, JitterMs: 40, Seed: int64(i)},
					Transform: true,
					Initial:   "A rope is the wrong primitive here.",
					Edits:     edits,
				})
			})
		},
	}
}

// embedIndex is the search side: one document's vector, the same
// call § 09's index build makes per page.
func embedIndex() Workload {
	text := strings.Repeat("anchors survive a split where integers do not, and the rope is the wrong primitive for a document made of addressable nodes. ", 8)
	corpus := semantic.NewCorpus([]semantic.Document{
		{ID: "a", Terms: semantic.Tokenize(text)},
		{ID: "b", Terms: semantic.Tokenize("a rope of fragments, and the anchors that outlive it")},
	})
	return Workload{
		Name: "embedIndex",
		Note: "semantic.Tokenize + Corpus.Embed — one page's hashed IDF-weighted TF vector, the call § 09's index build makes per page.",
		Run: func(t *Tracer, _ int) {
			var terms []string
			t.Span("tokenize", func() { terms = semantic.Tokenize(text) })
			t.Span("embed", func() { _ = corpus.Embed(terms) })
		},
	}
}

// seedPage builds a page of n paragraph blocks — a realistic
// tree to apply an op against, not an empty one.
func seedPage(n int) documentcore.Page {
	page := documentcore.NewPage(documentcore.PageID(uuid.Must(uuid.NewV7())), "bench")
	var after *documentcore.BlockID
	for i := 0; i < n; i++ {
		id := documentcore.BlockID(uuid.Must(uuid.NewV7()))
		if err := page.Apply(documentcore.InsertBlock{
			ID:      id,
			After:   after,
			Kind:    documentcore.BlockKind{Tag: documentcore.Paragraph},
			Content: documentcore.PlainContent(fmt.Sprintf("block %d", i)),
		}); err != nil {
			panic("bench: seedPage: " + err.Error())
		}
		next := id
		after = &next
	}
	return page
}
