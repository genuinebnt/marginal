package documentcore

import (
	"reflect"
	"testing"

	"pgregory.net/rapid"
)

// exampleOps is one concrete instance of every Op variant — enough to check
// invert-is-an-involution without needing a generator for a heterogeneous
// interface type. RFC-002 §3 treats invertibility as an algebraic law, so
// this is checked as one, not just spot-tested per variant.
func exampleOps() []Op {
	id, other := newTestBlockID(), newTestBlockID()
	heading, err := NewHeading(2)
	if err != nil {
		panic(err)
	}
	content := PlainContent("hello")
	if err := content.AddMark(NewBold(), 0, 5); err != nil {
		panic(err)
	}

	return []Op{
		InsertBlock{ID: id, After: &other, Kind: NewParagraph(), Content: PlainContent("x")},
		DeleteBlock{Tombstone: Block{ID: id, Kind: NewParagraph(), Content: PlainContent("x")}, After: &other},
		SetBlockKind{ID: id, From: NewParagraph(), To: heading},
		SetBlockContent{Block: id, Prev: PlainContent(""), Content: content},
		SetTitle{Page: newTestPageID(), From: "Old", To: "New"},
		MoveBlock{ID: id, From: nil, To: &other},
	}
}

func TestInvertIsAnInvolution(t *testing.T) {
	for _, op := range exampleOps() {
		got := op.Invert().Invert()
		if !opsEqual(op, got) {
			t.Errorf("invert is not an involution for %#v: got %#v", op, got)
		}
	}
}

// opsEqual compares two Ops' concrete type and field values, following
// pointer fields (After/From/To *BlockID) to the values they point to.
// reflect.DeepEqual, not a %#v string comparison — %#v prints a pointer's
// address, so two ops holding equal ids behind different pointers (e.g.
// one decoded from JSON, one built by hand) would otherwise look unequal.
func opsEqual(a, b Op) bool {
	return reflect.DeepEqual(a, b)
}

// PropertyTestPageApplyInvertRoundTrip walks a small sequence of
// InsertBlock ops built from rapid's random choices, checking RFC-002 §3's
// invertibility law after every step: apply(invert(op), apply(op, page))
// must equal page. Stateful rather than fully arbitrary — the generator
// only ever proposes an After that names a block that actually exists,
// since testing invertibility isn't the same project as testing rejection
// of bad input (page_test.go covers that deterministically).
func TestPropertyPageApplyInvertRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		page := NewPage(newTestPageID(), "Title")
		steps := rapid.IntRange(1, 8).Draw(t, "steps")

		for i := 0; i < steps; i++ {
			var after *BlockID
			if len(page.Blocks) > 0 && rapid.Bool().Draw(t, "pickAfter") {
				idx := rapid.IntRange(0, len(page.Blocks)-1).Draw(t, "afterIndex")
				id := page.Blocks[idx].ID
				after = &id
			}

			op := InsertBlock{
				ID:      newTestBlockID(),
				After:   after,
				Kind:    NewParagraph(),
				Content: PlainContent(rapid.StringN(0, 8, -1).Draw(t, "text")),
			}

			before := snapshotBlocks(page.Blocks)

			if err := page.Apply(op); err != nil {
				t.Fatalf("apply failed: %v", err)
			}
			if err := page.Apply(op.Invert()); err != nil {
				t.Fatalf("invert failed to apply: %v", err)
			}

			after2 := snapshotBlocks(page.Blocks)
			if !blocksEqual(before, after2) {
				t.Fatalf("round trip did not restore blocks:\nbefore: %+v\nafter:  %+v", before, after2)
			}

			// Re-apply so the page keeps growing across steps, instead of
			// resetting to empty every time.
			if err := page.Apply(op); err != nil {
				t.Fatalf("re-apply failed: %v", err)
			}
		}
	})
}

func snapshotBlocks(blocks []Block) []Block {
	out := make([]Block, len(blocks))
	copy(out, blocks)
	return out
}

func blocksEqual(a, b []Block) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Kind != b[i].Kind || a[i].Content.Text != b[i].Content.Text {
			return false
		}
	}
	return true
}
