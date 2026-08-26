// Package doctext combines a rope (the text content, RFC-001 §2's CRDT
// working format) with an anchor.Log (the per-character identity needed
// to resolve Anchors) into the one thing a live editing session actually
// mutates: insert/delete by rune offset, with every inserted character
// getting a permanent identity that a mark, comment, or another actor's
// concurrent op can refer back to regardless of what gets edited around
// it afterward.
//
// Offsets here are RUNE offsets, not byte offsets — a live cursor
// position is conceptually "the Nth character," and an Anchor should
// never be able to point into the middle of one. The rope itself is
// byte-indexed (matching document-service's stored spans), so Text
// converts at the boundary via rope.Rope.ByteOffsetForRune, an O(log n)
// tree walk — see byteOffsetForRuneOffset's own doc comment.
package doctext

import (
	"errors"
	"fmt"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/rope"
)

var (
	ErrOutOfBounds   = errors.New("doctext: offset out of bounds")
	ErrInvertedRange = errors.New("doctext: start > end")
)

type Text struct {
	rope rope.Rope
	log  *anchor.Log
	gen  *anchor.IDGenerator
}

// New is an empty Text whose inserted characters will be attributed to
// actor — this doc-actor process's own identity for Lamport ids, not a
// user id (RFC-001 §9; ItemID is about ordering insert operations, not
// authorship).
func New(actor string) *Text {
	return &Text{log: anchor.NewLog(), gen: anchor.NewIDGenerator(actor)}
}

func (t *Text) String() string { return t.rope.String() }

// RuneLen is the current live character count — what InsertAt/DeleteRange
// offsets are measured in.
func (t *Text) RuneLen() int { return t.log.LiveLen() }

// InsertAt inserts s at rune offset pos, assigning each inserted rune a
// fresh ItemID, and returns them — the caller needs these to build the
// Anchor a subsequent op (or a mark's AnchorRange) should refer to.
func (t *Text) InsertAt(pos int, s string) ([]anchor.ItemID, error) {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil, nil
	}
	if pos < 0 || pos > t.RuneLen() {
		return nil, ErrOutOfBounds
	}

	byteOffset, err := t.byteOffsetForRuneOffset(pos)
	if err != nil {
		return nil, err
	}
	newRope, err := t.rope.Insert(byteOffset, s)
	if err != nil {
		// Wrapped, not returned bare: doctext declares its own
		// ErrOutOfBounds/ErrInvertedRange sentinels, but an unwrapped
		// rope error here would leak rope.ErrOutOfBounds/
		// rope.ErrNotCharBoundary instead — errors.Is(err,
		// doctext.ErrOutOfBounds) would then silently return false for
		// a condition doctext's own doc comment implies it covers. %w
		// keeps rope's own sentinel discoverable via errors.Is too, so
		// nothing is lost, just no longer silently substituted.
		return nil, fmt.Errorf("doctext: insert: %w", err)
	}

	ids := make([]anchor.ItemID, len(runes))
	for i := range ids {
		ids[i] = t.gen.Next()
	}
	if err := t.log.InsertAt(pos, ids); err != nil {
		return nil, err
	}

	t.rope = newRope
	return ids, nil
}

// DeleteRange removes the runes in [start, end).
func (t *Text) DeleteRange(start, end int) error {
	if start > end {
		return ErrInvertedRange
	}
	if start < 0 || end > t.RuneLen() {
		return ErrOutOfBounds
	}
	if start == end {
		return nil
	}

	startByte, err := t.byteOffsetForRuneOffset(start)
	if err != nil {
		return err
	}
	endByte, err := t.byteOffsetForRuneOffset(end)
	if err != nil {
		return err
	}

	newRope, err := t.rope.Delete(startByte, endByte)
	if err != nil {
		return fmt.Errorf("doctext: delete: %w", err) // see InsertAt's identical wrap for why
	}
	if err := t.log.Tombstone(start, end); err != nil {
		return err
	}

	t.rope = newRope
	return nil
}

// Resolve finds a's current rune offset — At if the character it names
// is still live, Detached (to the nearest live position) if it was
// deleted, Unknown if this Text never saw that ItemID at all.
func (t *Text) Resolve(a anchor.Anchor) anchor.Resolved {
	return t.log.Resolve(a)
}

// Boundaries names the whole live document as one AnchorRange — nil if
// empty. A caller that only knows rune offsets (a plain-text client with
// no anchor of its own, e.g. internal/wsapi's document-boundary field —
// see docs/api/collaboration.md) uses this to build a "delete the whole
// document" op without needing every individual ItemID in between, the
// same way DeleteText's own Range only ever names its first and last
// item (internal/ops's own doc comment on DeleteText).
func (t *Text) Boundaries() *anchor.AnchorRange {
	n := t.RuneLen()
	if n == 0 {
		return nil
	}
	first, err := t.log.ItemAt(0)
	if err != nil {
		return nil // can't happen for n > 0, but never panic over a display hint
	}
	last, err := t.log.ItemAt(n - 1)
	if err != nil {
		return nil
	}
	return &anchor.AnchorRange{
		Start: anchor.Anchor{Item: first, Bias: anchor.Before},
		End:   anchor.Anchor{Item: last, Bias: anchor.After},
	}
}

// Slice reads the runes in [start, end) without editing anything —
// internal/ops uses this to capture a DeleteText op's deleted content,
// which its own inverse (an InsertText) needs to carry.
func (t *Text) Slice(start, end int) (string, error) {
	if start > end {
		return "", ErrInvertedRange
	}
	if start < 0 || end > t.RuneLen() {
		return "", ErrOutOfBounds
	}
	startByte, err := t.byteOffsetForRuneOffset(start)
	if err != nil {
		return "", err
	}
	endByte, err := t.byteOffsetForRuneOffset(end)
	if err != nil {
		return "", err
	}
	s, err := t.rope.Slice(startByte, endByte)
	if err != nil {
		return "", fmt.Errorf("doctext: slice: %w", err) // see InsertAt's identical wrap for why
	}
	return s, nil
}

// byteOffsetForRuneOffset converts a rune count into the byte offset the
// rope actually indexes by — an O(log n) tree walk (rope.Rope.ByteOffsetForRune),
// not a decode of the whole document. An earlier version called
// t.rope.String() on every InsertAt/DeleteRange/Slice — twice, for the
// latter two — materialising the entire live text just to find one byte
// offset, which defeated the actual point of using a rope over a plain
// string for exactly the hot path (one character op per keystroke) the
// rope exists to make cheap.
func (t *Text) byteOffsetForRuneOffset(runeOffset int) (int, error) {
	i, err := t.rope.ByteOffsetForRune(runeOffset)
	if err != nil {
		return 0, ErrOutOfBounds
	}
	return i, nil
}
