// Package ops is the character-granular tier of RFC-002 §2's ISA —
// InsertText/DeleteText, applied against a doctext.Text. (SetMark isn't
// built yet — it needs mark storage on live text, which doctext doesn't
// have; see docs/porting/PROGRESS.md.)
//
// Unlike document-core's block-granular Op, this package doesn't expose
// a standalone Invert() method — Apply returns the inverse as a result
// instead. That's a real structural difference, not an inconsistency:
// InsertText's inverse is a DeleteText whose AnchorRange names the
// ItemIDs Apply itself assigns during insertion, which don't exist until
// Apply runs. document-core could compute Op.Invert() from an op's own
// fields alone because its ids were already known before applying;
// character-granular ops can't be. RFC-002 §3's invertibility law still
// holds — apply(inverse, apply(op, text)) == text — only *when* the
// inverse becomes computable differs.
package ops

import (
	"errors"
	"fmt"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/doctext"
)

var ErrUnknownAnchor = errors.New("ops: anchor refers to an item this text never saw")

type Op interface {
	isOp()
}

// InsertText inserts Text at At — nil At means "the start of the
// document," the one position that can't be named by an existing item's
// Anchor (there's nothing before it to anchor to when the document, or
// this insert's position, is empty).
type InsertText struct {
	At   *anchor.Anchor `json:"at"`
	Text string         `json:"text"`
}

func (InsertText) isOp() {}

// DeleteText removes the text in Range. Range's Start anchors to the
// first deleted item (Bias Before) and End to the last deleted item
// (Bias After) — deliberately anchoring to the items being removed, not
// their surviving neighbors: after the delete, resolving Start again
// naturally comes back Detached, at exactly the gap the deletion left —
// which is exactly where DeleteText's own inverse (an InsertText) needs
// to land. Text carries the deleted content, needed to invert.
type DeleteText struct {
	Range anchor.AnchorRange `json:"range"`
	Text  string             `json:"text"`
}

func (DeleteText) isOp() {}

// NoOp is InsertText's inverse when Text was empty — nothing was
// inserted, so nothing needs undoing, but Apply must still return some Op
// (a nil Op interface value would be a wrong-shaped sentinel to check
// against everywhere else).
type NoOp struct{}

func (NoOp) isOp() {}

// Apply mutates text according to op and returns the inverse.
func Apply(text *doctext.Text, op Op) (Op, error) {
	switch op := op.(type) {
	case InsertText:
		return applyInsertText(text, op)
	case DeleteText:
		return applyDeleteText(text, op)
	case NoOp:
		return NoOp{}, nil
	default:
		return nil, fmt.Errorf("ops: unknown op type %T", op)
	}
}

func applyInsertText(text *doctext.Text, op InsertText) (Op, error) {
	pos, err := resolveInsertPosition(text, op.At)
	if err != nil {
		return nil, err
	}

	ids, err := text.InsertAt(pos, op.Text)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return NoOp{}, nil
	}

	return DeleteText{
		Range: anchor.AnchorRange{
			Start: anchor.Anchor{Item: ids[0], Bias: anchor.Before},
			End:   anchor.Anchor{Item: ids[len(ids)-1], Bias: anchor.After},
		},
		Text: op.Text,
	}, nil
}

func applyDeleteText(text *doctext.Text, op DeleteText) (Op, error) {
	start, end, err := resolveRange(text, op.Range)
	if err != nil {
		return nil, err
	}
	if start == end {
		return NoOp{}, nil
	}

	deleted, err := text.Slice(start, end)
	if err != nil {
		return nil, err
	}
	if err := text.DeleteRange(start, end); err != nil {
		return nil, err
	}

	// Re-resolving Range.Start after the delete: its target item is now
	// tombstoned (this delete just did that), so it comes back Detached
	// at exactly the gap left behind — the correct reinsertion point.
	reinsertAt := text.Resolve(op.Range.Start)
	if err := validateReinsertPosition(reinsertAt); err != nil {
		return nil, err
	}
	return InsertText{At: &op.Range.Start, Text: deleted}, nil
}

// validateReinsertPosition is a defensive check, not a real branch in
// normal operation: Range.Start must resolve to something after its own
// delete, or DeleteText's own construction (or a concurrent edit this
// package doesn't yet reconcile against) was inconsistent.
func validateReinsertPosition(r anchor.Resolved) error {
	if r.Kind == anchor.Unknown {
		return ErrUnknownAnchor
	}
	return nil
}

func resolveInsertPosition(text *doctext.Text, at *anchor.Anchor) (int, error) {
	if at == nil {
		return 0, nil
	}
	resolved := text.Resolve(*at)
	if resolved.Kind == anchor.Unknown {
		return 0, ErrUnknownAnchor
	}
	return resolved.Offset, nil
}

func resolveRange(text *doctext.Text, r anchor.AnchorRange) (start, end int, err error) {
	startResolved := text.Resolve(r.Start)
	if startResolved.Kind == anchor.Unknown {
		return 0, 0, ErrUnknownAnchor
	}
	endResolved := text.Resolve(r.End)
	if endResolved.Kind == anchor.Unknown {
		return 0, 0, ErrUnknownAnchor
	}
	start, end = startResolved.Offset, endResolved.Offset
	if start > end {
		start, end = end, start
	}
	return start, end, nil
}
