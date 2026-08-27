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
		if a[i].ID != b[i].ID || a[i].Kind != b[i].Kind || a[i].Content.Text != b[i].Content.Text ||
			!blockIDPtrEqual(a[i].Parent, b[i].Parent) {
			return false
		}
	}
	return true
}

// containerCandidates returns every Quote/Toggle/Callout/Aside block in
// blocks — a random legal Parent for the nested property test below to
// choose from. Deliberately excludes List/ListItem: their extra
// ListChild ::= List | Paragraph restriction (InvalidListChildError) is
// already covered deterministically
// (TestInsertBlockUnderListItemRestrictsChildKind,
// TestSetBlockKindUnderListItemRestrictsChildKind); a generator that has
// to also replicate that rule to keep proposing legal ops would risk its
// own mirror logic drifting from Page.Apply's, and this property is about
// invertibility under nesting, not exhaustively fuzzing every containment
// rule a second time. Quote/Toggle/Callout/Aside all accept any child
// kind, so they can share one pool.
func containerCandidates(blocks []Block) []BlockID {
	var out []BlockID
	for _, b := range blocks {
		switch b.Kind.Tag {
		case Quote, Toggle, Callout, Aside:
			out = append(out, b.ID)
		}
	}
	return out
}

// TestPropertyNestedPageApplyInvertRoundTrip is
// TestPropertyPageApplyInvertRoundTrip's nested counterpart — RFC-002 §3's
// invertibility law is the harder property to trust once a container can
// carry a whole subtree along on a single MoveBlock (page.go's own doc
// comment on that case). The generator only ever proposes an InsertBlock
// under an existing container or a MoveBlock reparenting under one
// (never a cycle — ToParent is drawn from containers that aren't the
// moved block or, per its own doc comment, don't need an extra check
// here: containerCandidates never includes ListItem/List, and the moved
// block itself is excluded from the pool by construction below), for the
// same reason the flat version only proposes a legal After: testing
// invertibility isn't the same project as testing rejection.
func TestPropertyNestedPageApplyInvertRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		page := NewPage(newTestPageID(), "Title")
		steps := rapid.IntRange(1, 8).Draw(t, "steps")

		newBlockKind := func() BlockKind {
			if rapid.Bool().Draw(t, "isContainer") {
				switch rapid.IntRange(0, 3).Draw(t, "containerKind") {
				case 0:
					return NewQuote()
				case 1:
					return NewToggle()
				case 2:
					return NewCallout(Warn, "")
				default:
					return NewAside("🐢")
				}
			}
			return NewParagraph()
		}

		for i := 0; i < steps; i++ {
			containers := containerCandidates(page.Blocks)

			var op Op
			if len(page.Blocks) > 0 && len(containers) > 0 && rapid.Bool().Draw(t, "moveInsteadOfInsert") {
				// Reparent a random existing block under a random
				// container that isn't itself.
				movedIdx := rapid.IntRange(0, len(page.Blocks)-1).Draw(t, "movedIndex")
				moved := page.Blocks[movedIdx]

				var toParent *BlockID
				candidates := make([]BlockID, 0, len(containers))
				for _, c := range containers {
					if c != moved.ID {
						candidates = append(candidates, c)
					}
				}
				if len(candidates) > 0 && rapid.Bool().Draw(t, "reparent") {
					id := candidates[rapid.IntRange(0, len(candidates)-1).Draw(t, "toParentIndex")]
					// Reject if id is moved's own descendant — the only
					// case containerCandidates' own exclusions don't
					// already rule out (moved's subtree can itself
					// contain Quote/Toggle blocks).
					byID := blockIndex(page.Blocks)
					if !isDescendant(page.Blocks, byID, id, moved.ID) {
						toParent = &id
					}
				}
				op = MoveBlock{ID: moved.ID, FromParent: moved.Parent, From: predecessorOf(page.Blocks, movedIdx), ToParent: toParent, To: nil}
			} else {
				var parent *BlockID
				if len(containers) > 0 && rapid.Bool().Draw(t, "pickParent") {
					id := containers[rapid.IntRange(0, len(containers)-1).Draw(t, "parentIndex")]
					parent = &id
				}
				var after *BlockID
				if len(page.Blocks) > 0 && rapid.Bool().Draw(t, "pickAfter") {
					idx := rapid.IntRange(0, len(page.Blocks)-1).Draw(t, "afterIndex")
					id := page.Blocks[idx].ID
					after = &id
				}
				op = InsertBlock{
					ID:      newTestBlockID(),
					Parent:  parent,
					After:   after,
					Kind:    newBlockKind(),
					Content: PlainContent(rapid.StringN(0, 8, -1).Draw(t, "text")),
				}
			}

			before := snapshotBlocks(page.Blocks)

			if err := page.Apply(op); err != nil {
				// InsertBlock's own After may legitimately be a
				// now-invalid choice relative to the randomly-picked
				// Parent (e.g. after names a block under a different
				// parent) — retry this step rather than fail the whole
				// property on a generator miss, matching the flat
				// version's own bias toward "only ever propose a legal
				// op" without needing a fully independent legality
				// oracle here.
				continue
			}
			if err := page.Apply(op.Invert()); err != nil {
				t.Fatalf("invert failed to apply for %#v: %v", op, err)
			}

			after2 := snapshotBlocks(page.Blocks)
			if !blocksEqual(before, after2) {
				t.Fatalf("round trip did not restore blocks for %#v:\nbefore: %+v\nafter:  %+v", op, before, after2)
			}

			if err := page.Apply(op); err != nil {
				t.Fatalf("re-apply failed for %#v: %v", op, err)
			}
		}
	})
}
