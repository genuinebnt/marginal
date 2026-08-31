// Package opscript parses and replays § 13 TRACE's editable op script.
//
// The lab section's premise is that you type something and watch the
// machinery move. TRACE could only replay a page already stored on the
// server, so there was nothing to type into and its palimpsest was empty
// on every seeded page — nothing had ever typed a character into one.
//
// It lives here rather than in documentcore because it is a
// DEMO-ONLY concern: documentcore is the real editor core, shared with
// collaboration-service, and a script format nobody but one screen reads
// does not belong in it. It is Go rather than TypeScript for the reason
// netsim.ParseScenario and sketch.ParseStream are: parsing an input into
// the shape an algorithm consumes is part of the algorithm's contract.
package opscript

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"marginal/documentcore"
)

// scriptStep is one entry of the replay — deliberately the same shape
// collaboration-service's own trace endpoint returns, so the screen
// consumes a typed script and a real page's history through one code
// path rather than two.
type Step struct {
	// Kind and Detail are what the op stream prints.
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	// Op and Inverse are the real ops, as JSON, for the panels that
	// show them.
	Op      any `json:"op"`
	Inverse any `json:"inverse"`
	// LawHolds is RFC-002 §3 CHECKED, not asserted: the inverse is
	// actually applied to the page the op produced, and the result
	// compared field by field with the page from before it. A screen
	// that printed this without running it would be making the same
	// empty claim as an audit log with a green tick and no hash chain.
	LawHolds bool `json:"law_holds"`
	// After is the page as of this step.
	After documentcore.Page `json:"after"`
}

type Result struct {
	Steps []Step `json:"steps"`
	// Skipped counts lines that did not parse. Never fatal — half a
	// line is the normal state of a textarea being typed into.
	Skipped int `json:"skipped"`
	// Errors are lines that parsed but could not apply, with the
	// reason. An op the model REJECTS is a real answer about the
	// model, so it is reported rather than swallowed.
	Errors []string `json:"errors"`
	// AllHold is false the moment any step's law check fails.
	AllHold bool `json:"all_hold"`
}

// parseScript reads § 13's editable op script.
//
//	insert <kind> <text...>      append a block, e.g. `insert heading Anchors`
//	text <n> <text...>           set block n's content
//	kind <n> <kind>              change block n's kind
//	move <from> <to>             move a block
//	delete <n>                   delete block n
//
// Block indices are 1-based, matching what the screen prints beside each
// block — an index nobody can see is not usable in a script somebody
// types.
func Replay(src string) Result {
	page := documentcore.NewPage(documentcore.PageID(uuid.Must(uuid.NewV7())), "scratch")
	out := Result{Steps: []Step{}, Errors: []string{}, AllHold: true}

	for n, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		op, err := parseLine(line, page)
		if err != nil {
			out.Skipped++
			continue
		}

		before := clonePage(page)
		if err := page.Apply(op); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("line %d: %v", n+1, err))
			continue
		}

		// The law, run rather than claimed: undo the op on the page it
		// produced and see whether we are back where we started.
		inverse := op.Invert()
		check := clonePage(page)
		holds := check.Apply(inverse) == nil && samePage(check, before)
		if !holds {
			out.AllHold = false
		}

		kind, detail := describe(op)
		out.Steps = append(out.Steps, Step{
			Kind: kind, Detail: detail,
			Op: op, Inverse: inverse,
			LawHolds: holds, After: clonePage(page),
		})
	}
	return out
}

func parseLine(line string, page documentcore.Page) (documentcore.Op, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return nil, fmt.Errorf("too few fields")
	}
	blockAt := func(s string) (documentcore.Block, error) {
		i, err := strconv.Atoi(s)
		if err != nil || i < 1 || i > len(page.Blocks) {
			return documentcore.Block{}, fmt.Errorf("no block %s", s)
		}
		return page.Blocks[i-1], nil
	}

	switch strings.ToLower(fields[0]) {
	case "insert":
		kind, err := parseKind(fields[1])
		if err != nil {
			return nil, err
		}
		var after *documentcore.BlockID
		if len(page.Blocks) > 0 {
			last := page.Blocks[len(page.Blocks)-1].ID
			after = &last
		}
		return documentcore.InsertBlock{
			ID:      documentcore.BlockID(uuid.Must(uuid.NewV7())),
			After:   after,
			Kind:    kind,
			Content: documentcore.PlainContent(strings.Join(fields[2:], " ")),
		}, nil

	case "text":
		b, err := blockAt(fields[1])
		if err != nil {
			return nil, err
		}
		return documentcore.SetBlockContent{
			Block: b.ID, Prev: b.Content,
			Content: documentcore.PlainContent(strings.Join(fields[2:], " ")),
		}, nil

	case "kind":
		b, err := blockAt(fields[1])
		if err != nil {
			return nil, err
		}
		if len(fields) < 3 {
			return nil, fmt.Errorf("kind needs a value")
		}
		k, err := parseKind(fields[2])
		if err != nil {
			return nil, err
		}
		return documentcore.SetBlockKind{ID: b.ID, From: b.Kind, To: k}, nil

	case "delete":
		b, err := blockAt(fields[1])
		if err != nil {
			return nil, err
		}
		var after *documentcore.BlockID
		for i, blk := range page.Blocks {
			if blk.ID == b.ID && i > 0 {
				prev := page.Blocks[i-1].ID
				after = &prev
			}
		}
		return documentcore.DeleteBlock{Tombstone: b, After: after}, nil

	case "move":
		if len(fields) < 3 {
			return nil, fmt.Errorf("move needs from and to")
		}
		b, err := blockAt(fields[1])
		if err != nil {
			return nil, err
		}
		to, err := blockAt(fields[2])
		if err != nil {
			return nil, err
		}
		var from *documentcore.BlockID
		for i, blk := range page.Blocks {
			if blk.ID == b.ID && i > 0 {
				prev := page.Blocks[i-1].ID
				from = &prev
			}
		}
		toID := to.ID
		return documentcore.MoveBlock{ID: b.ID, From: from, To: &toID}, nil
	}
	return nil, fmt.Errorf("unknown verb %q", fields[0])
}

func parseKind(s string) (documentcore.BlockKind, error) {
	switch strings.ToLower(s) {
	case "paragraph", "p":
		return documentcore.BlockKind{Tag: documentcore.Paragraph}, nil
	case "heading", "h":
		return documentcore.BlockKind{Tag: documentcore.Heading, Level: 2}, nil
	case "quote":
		return documentcore.BlockKind{Tag: documentcore.Quote}, nil
	case "code":
		return documentcore.BlockKind{Tag: documentcore.CodeBlock}, nil
	case "divider":
		return documentcore.BlockKind{Tag: documentcore.Divider}, nil
	}
	return documentcore.BlockKind{}, fmt.Errorf("unknown block kind %q", s)
}

func describe(op documentcore.Op) (string, string) {
	switch o := op.(type) {
	case documentcore.InsertBlock:
		return "InsertBlock", fmt.Sprintf("%s %q", o.Kind.Tag, truncate(o.Content.Text))
	case documentcore.SetBlockContent:
		return "SetBlockContent", fmt.Sprintf("%q", truncate(o.Content.Text))
	case documentcore.SetBlockKind:
		return "SetBlockKind", fmt.Sprintf("%s to %s", o.From.Tag, o.To.Tag)
	case documentcore.DeleteBlock:
		return "DeleteBlock", fmt.Sprintf("%q", truncate(o.Tombstone.Content.Text))
	case documentcore.MoveBlock:
		return "MoveBlock", "reordered"
	}
	return "Op", ""
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}

// clonePage is a deep-enough copy for replay: the block slice is the
// only thing Apply mutates in place.
func clonePage(p documentcore.Page) documentcore.Page {
	out := p
	out.Blocks = append([]documentcore.Block(nil), p.Blocks...)
	return out
}

// samePage compares what the law is about: the block sequence, each
// one's kind and text. Ids are included because a reinserted block
// keeping its identity is exactly what makes an inverse an inverse.
func samePage(a, b documentcore.Page) bool {
	if len(a.Blocks) != len(b.Blocks) {
		return false
	}
	for i := range a.Blocks {
		x, y := a.Blocks[i], b.Blocks[i]
		if x.ID != y.ID || x.Kind != y.Kind || x.Content.Text != y.Content.Text {
			return false
		}
	}
	return true
}

// Test helpers live here rather than in the test file because they
// construct documentcore values the package already knows how to build,
// and a test that reimplements construction is testing its own copy.
func newScratch() documentcore.Page {
	return documentcore.NewPage(documentcore.PageID(uuid.Must(uuid.NewV7())), "scratch")
}

func setContentWithWrongPrev(b documentcore.Block, text string) documentcore.SetBlockContent {
	return documentcore.SetBlockContent{
		Block:   b.ID,
		Prev:    documentcore.PlainContent("something else entirely"),
		Content: documentcore.PlainContent(text),
	}
}
