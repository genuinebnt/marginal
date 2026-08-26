package documentcore

import "testing"

// Baselines recorded in docs/porting/BENCHMARKS.md for later comparison
// against the Rust port — run with: go test ./... -bench=. -benchmem
// -run=^$ (the -run=^$ skips the regular test suite so only benchmarks run).

// BenchmarkPageApplyInsertBlock measures one InsertBlock's cost against a
// page of realistic, CONSTANT size (200 blocks, matching
// BenchmarkPageApplySetBlockContent's own "realistic number of blocks"
// baseline) — every iteration inserts after the same fixed reference
// block, then immediately un-inserts it (via Invert, outside the timer)
// so the page never grows across b.N. An earlier version let every
// iteration insert and never delete, so the page grew without bound over
// the whole benchmark run; since InsertBlock/indexOf are a linear scan
// over Page.Blocks (a deliberate, simple choice at this repo's scale —
// see Apply's own switch), the reported ns/op was actually the amortized
// average of a linearly-growing-per-call operation across however many
// iterations -benchtime happened to run, not a stable per-op cost. Caught
// while re-running this exact benchmark for a Rust-port comparison
// baseline: -benchtime=100x and -benchtime=1s disagreed by ~100x because
// they grew the page to very different sizes, which is the tell.
func BenchmarkPageApplyInsertBlock(b *testing.B) {
	page := NewPage(newTestPageID(), "Title")
	var after *BlockID
	for i := 0; i < 200; i++ {
		id := newTestBlockID()
		if err := page.Apply(InsertBlock{ID: id, After: after, Kind: NewParagraph(), Content: PlainContent("x")}); err != nil {
			b.Fatal(err)
		}
		after = &id
	}
	target := page.Blocks[100].ID

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := newTestBlockID()
		op := InsertBlock{ID: id, After: &target, Kind: NewParagraph(), Content: PlainContent("x")}
		if err := page.Apply(op); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		if err := page.Apply(op.Invert()); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

// BenchmarkPageApplySetBlockContent measures the hot path of a live editing
// session: retyping the same block over and over, on a page that already
// has a realistic number of other blocks around it.
func BenchmarkPageApplySetBlockContent(b *testing.B) {
	page := NewPage(newTestPageID(), "Title")
	var after *BlockID
	for i := 0; i < 200; i++ {
		id := newTestBlockID()
		if err := page.Apply(InsertBlock{ID: id, After: after, Kind: NewParagraph(), Content: PlainContent("x")}); err != nil {
			b.Fatal(err)
		}
		after = &id
	}
	target := page.Blocks[100].ID
	prev := page.Blocks[100].Content

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next := PlainContent("updated content")
		if err := page.Apply(SetBlockContent{Block: target, Prev: prev, Content: next}); err != nil {
			b.Fatal(err)
		}
		prev = next
	}
}

func BenchmarkHistoryUndoRedo(b *testing.B) {
	page, id := pageWithOneBlock()
	h := NewHistory(1000)
	prev := page.Blocks[0].Content

	for i := 0; i < 500; i++ {
		next := PlainContent("content")
		if err := page.Apply(SetBlockContent{Block: id, Prev: prev, Content: next}); err != nil {
			b.Fatal(err)
		}
		h.Record(SetBlockContent{Block: id, Prev: prev, Content: next})
		prev = next
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := h.Undo(&page); err != nil {
			b.Fatal(err)
		}
		if err := h.Redo(&page); err != nil {
			b.Fatal(err)
		}
	}
}
