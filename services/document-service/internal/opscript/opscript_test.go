package opscript

import (
	"testing"

	"github.com/google/uuid"

	"marginal/documentcore"
)

// The law is the screen's whole claim, so it is tested by running a
// script and asserting every step's inverse actually restores the
// page — not by trusting that Invert() is correct.
func TestEveryStepOfAScriptInverts(t *testing.T) {
	r := Replay(`
insert heading Anchors
insert paragraph A rope is the wrong primitive
text 2 A tree of addressable nodes
kind 2 quote
move 2 1
delete 1
`)
	if len(r.Steps) != 6 {
		t.Fatalf("expected 6 steps, got %d (skipped %d, errors %v)", len(r.Steps), r.Skipped, r.Errors)
	}
	if !r.AllHold {
		for i, s := range r.Steps {
			if !s.LawHolds {
				t.Errorf("step %d (%s) does not invert", i+1, s.Kind)
			}
		}
	}
}

// Half a line is the normal state of a textarea being typed into.
func TestMalformedLinesAreSkippedNotFatal(t *testing.T) {
	r := Replay("insert heading Ok\nnonsense\ninsert\ntext 99 nowhere\ninsert paragraph Fine")
	if len(r.Steps) != 2 {
		t.Fatalf("expected 2 usable steps, got %d", len(r.Steps))
	}
	if r.Skipped != 3 {
		t.Errorf("expected 3 skipped, got %d", r.Skipped)
	}
}

// Two guarantees, and the second is stronger than I expected.
//
// The law check re-runs each inverse and compares, so it is verifying
// Invert() rather than trusting it. But an op that LIES about what it
// overwrote never reaches that check: Page.Apply rejects it outright
// with "current content does not match op's expected prior value".
//
// So the model does not merely record enough to undo an edit — it
// refuses an edit whose recorded prior value is wrong, at apply time.
// That is RFC-002 §3 enforced rather than documented, and it is why
// LawHolds is true for every op that applied: the ops that would break
// it cannot get in.
func TestApplyRefusesAnOpThatLiesAboutWhatItOverwrote(t *testing.T) {
	page := newScratch()
	id := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	if err := page.Apply(documentcore.InsertBlock{
		ID: id, Kind: documentcore.BlockKind{Tag: documentcore.Paragraph},
		Content: documentcore.PlainContent("hello"),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := page.Apply(setContentWithWrongPrev(page.Blocks[0], "goodbye"))
	if err == nil {
		t.Fatal("Apply accepted an op whose Prev did not match the block's content")
	}

	// And the block is untouched — a rejected op is not a partial one.
	if page.Blocks[0].Content.Text != "hello" {
		t.Errorf("a rejected op still changed the page: %q", page.Blocks[0].Content.Text)
	}
}

// Every op kind the script can emit must invert. This is the check the
// screen prints, run over one of each.
func TestEveryOpKindInverts(t *testing.T) {
	r := Replay(`
insert heading Anchors
insert paragraph A rope is the wrong primitive
text 2 A tree of addressable nodes
kind 2 quote
move 2 1
delete 1
`)
	if !r.AllHold {
		for i, s := range r.Steps {
			if !s.LawHolds {
				t.Errorf("step %d (%s) does not invert", i+1, s.Kind)
			}
		}
	}
	kinds := map[string]bool{}
	for _, s := range r.Steps {
		kinds[s.Kind] = true
	}
	for _, want := range []string{"InsertBlock", "SetBlockContent", "SetBlockKind", "MoveBlock", "DeleteBlock"} {
		if !kinds[want] {
			t.Errorf("script never produced a %s", want)
		}
	}
}
