package documentcore

import "testing"

// Baselines recorded in docs/porting/BENCHMARKS.md for later comparison
// against the Rust port — run with: go test ./... -bench=. -benchmem
// -run=^$ (the -run=^$ skips the regular test suite so only benchmarks run).

func BenchmarkPageApplyInsertBlock(b *testing.B) {
	page := NewPage(newTestPageID(), "Title")
	var after *BlockID

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		id := newTestBlockID()
		if err := page.Apply(InsertBlock{ID: id, After: after, Kind: NewParagraph(), Content: PlainContent("x")}); err != nil {
			b.Fatal(err)
		}
		after = &id
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
