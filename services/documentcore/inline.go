package documentcore

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MarkTag is a Mark's variant discriminant. RFC-001 §1: bold | italic |
// strike | code | link(Url) | pagelink(PageId).
type MarkTag uint8

const (
	Bold MarkTag = iota
	Italic
	Strike
	Code
	Link
	PageLink
)

// MarkKind is the full identity of a mark, including its payload where one
// exists — two Link marks with different URLs are different kinds, and
// don't coalesce even when they cover the same range (RFC-001: the URL is
// part of the kind, not incidental data). A tagged struct rather than an
// interface: the variant set is small and fixed, and this needs to be a
// map key and support ==, which a fixed struct gets for free.
type MarkKind struct {
	Tag  MarkTag
	URL  string // Link only
	Page PageID // PageLink only
}

func NewBold() MarkKind             { return MarkKind{Tag: Bold} }
func NewItalic() MarkKind           { return MarkKind{Tag: Italic} }
func NewStrike() MarkKind           { return MarkKind{Tag: Strike} }
func NewCode() MarkKind             { return MarkKind{Tag: Code} }
func NewLink(url string) MarkKind   { return MarkKind{Tag: Link, URL: url} }
func NewPageLink(p PageID) MarkKind { return MarkKind{Tag: PageLink, Page: p} }

func (t MarkTag) String() string {
	switch t {
	case Bold:
		return "bold"
	case Italic:
		return "italic"
	case Strike:
		return "strike"
	case Code:
		return "code"
	case Link:
		return "link"
	case PageLink:
		return "pagelink"
	default:
		return fmt.Sprintf("MarkTag(%d)", uint8(t))
	}
}

// markKindJSON is the wire shape: {"tag":"bold"}, {"tag":"link","url":"..."},
// or {"tag":"pagelink","page":"<uuid>"} — unused fields omitted. Matches
// testdata/document-core/marks.json's vector schema exactly.
type markKindJSON struct {
	Tag  string `json:"tag"`
	URL  string `json:"url,omitempty"`
	Page string `json:"page,omitempty"`
}

func (k MarkKind) MarshalJSON() ([]byte, error) {
	raw := markKindJSON{Tag: k.Tag.String(), URL: k.URL}
	if k.Tag == PageLink {
		raw.Page = k.Page.String()
	}
	return json.Marshal(raw)
}

func (k *MarkKind) UnmarshalJSON(data []byte) error {
	var raw markKindJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch raw.Tag {
	case "bold":
		*k = NewBold()
	case "italic":
		*k = NewItalic()
	case "strike":
		*k = NewStrike()
	case "code":
		*k = NewCode()
	case "link":
		*k = NewLink(raw.URL)
	case "pagelink":
		id, err := uuid.Parse(raw.Page)
		if err != nil {
			return fmt.Errorf("documentcore: invalid pagelink page id %q: %w", raw.Page, err)
		}
		*k = NewPageLink(PageID(id))
	default:
		return fmt.Errorf("documentcore: unknown mark tag %q", raw.Tag)
	}
	return nil
}

// Mark is a MarkKind applied over a half-open byte range [Start, End) of a
// Content's Text. Byte offsets, not rune/UTF-16 indices — DATA_MODEL.md
// persists marks as byte offsets in JSONB, so this is the wire format, not
// a Go-idiom choice; docs/rust/README.md's future Rust port and any
// TypeScript consumer must use the same offsets to agree on the same text.
type Mark struct {
	Kind  MarkKind `json:"kind"`
	Start int      `json:"start"`
	End   int      `json:"end"`
}

// Content is a block's inline text plus its marks. Invariants (checked by
// AddMark/RemoveMark, restored by normalise after every mutation):
//
//  1. Marks are sorted by Start (ties broken by Tag, then URL, then Page).
//  2. Same-kind marks never overlap and never touch — adjacent or
//     overlapping runs of the same kind are always coalesced into one.
//  3. Different kinds are independent and may overlap freely.
//  4. A zero-width range ([n, n)) carries no mark and is silently dropped.
type Content struct {
	Text  string `json:"text"`
	Marks []Mark `json:"marks"`
}

// PlainContent is unformatted text with no marks. Marks starts as a
// non-nil empty slice, not nil — so JSON encoding always produces
// "marks":[] rather than "marks":null across the WASM/JS boundary.
func PlainContent(text string) Content { return Content{Text: text, Marks: []Mark{}} }

// Equal reports whether c and other hold the same text and the same
// marks in the same order — Page.Apply's SetBlockContent precondition
// check (RFC-002 §1) uses this instead of reflect.DeepEqual, which
// treats a nil Marks slice and an empty non-nil one as unequal. Both
// occur legitimately in this codebase (PlainContent always constructs a
// non-nil empty slice; a JSON-decoded Content with an absent/null
// "marks" field, or one built as a bare Content{Text: s} literal, gets
// nil) — reflect.DeepEqual made a real SetBlockContent op fail its
// precondition depending purely on which construction path produced the
// value being compared against, not on any actual difference in marks.
// Mark (and MarkKind inside it) is a plain comparable struct — no
// reflection needed, and no allocation either.
func (c Content) Equal(other Content) bool {
	if c.Text != other.Text {
		return false
	}
	if len(c.Marks) != len(other.Marks) {
		return false
	}
	for i := range c.Marks {
		if c.Marks[i] != other.Marks[i] {
			return false
		}
	}
	return true
}

// OutOfBoundsError reports an offset beyond the content's byte length.
type OutOfBoundsError struct {
	Offset int
	Len    int
}

func (e *OutOfBoundsError) Error() string {
	return fmt.Sprintf("offset %d out of bounds for content of length %d", e.Offset, e.Len)
}

// InvertedRangeError reports Start > End.
type InvertedRangeError struct{ Start, End int }

func (e *InvertedRangeError) Error() string {
	return fmt.Sprintf("inverted range: start %d > end %d", e.Start, e.End)
}

// NotCharBoundaryError reports an offset that falls inside a multi-byte
// UTF-8 sequence rather than at its start.
type NotCharBoundaryError struct{ Offset int }

func (e *NotCharBoundaryError) Error() string {
	return fmt.Sprintf("offset %d is not a char boundary", e.Offset)
}

// validateRange checks [start, end) against c.Text and reports whether it's
// a zero-width no-op. Order matters and is part of the contract tested
// against: bounds, then zero-width, then inverted, then boundary(start),
// then boundary(end) — see inline_test.go for the case that pins each one.
func (c *Content) validateRange(start, end int) (zeroWidth bool, err error) {
	n := len(c.Text)
	if start > n {
		return false, &OutOfBoundsError{Offset: start, Len: n}
	}
	if end > n {
		return false, &OutOfBoundsError{Offset: end, Len: n}
	}
	if start == end {
		return true, nil
	}
	if start > end {
		return false, &InvertedRangeError{Start: start, End: end}
	}
	if !utf8.RuneStart(c.Text[start]) {
		return false, &NotCharBoundaryError{Offset: start}
	}
	if end < n && !utf8.RuneStart(c.Text[end]) {
		return false, &NotCharBoundaryError{Offset: end}
	}
	return false, nil
}

// AddMark applies kind over [start, end). A zero-width range is a no-op,
// not an error. Same-kind marks that overlap or touch the new one coalesce
// (invariant 2) — normalise does the merge, so add order never affects the
// result.
func (c *Content) AddMark(kind MarkKind, start, end int) error {
	zeroWidth, err := c.validateRange(start, end)
	if err != nil {
		return err
	}
	if zeroWidth {
		return nil
	}
	c.Marks = append(c.Marks, Mark{Kind: kind, Start: start, End: end})
	c.normalise()
	return nil
}

// RemoveMark subtracts [start, end) from every mark of kind — trimming an
// edge, splitting a mark that fully covers the range into two, dropping a
// mark fully covered by it, or leaving it untouched if disjoint. Marks of
// other kinds are never considered.
func (c *Content) RemoveMark(kind MarkKind, start, end int) error {
	zeroWidth, err := c.validateRange(start, end)
	if err != nil {
		return err
	}
	if zeroWidth {
		return nil
	}

	kept := make([]Mark, 0, len(c.Marks))
	for _, m := range c.Marks {
		if m.Kind != kind {
			kept = append(kept, m)
			continue
		}
		switch {
		case m.End <= start || m.Start >= end:
			kept = append(kept, m) // disjoint
		case m.Start >= start && m.End <= end:
			// fully covered by the removal — dropped
		case m.Start < start && m.End > end:
			// removal is a hole inside this mark — split around it
			kept = append(kept, Mark{Kind: kind, Start: m.Start, End: start})
			kept = append(kept, Mark{Kind: kind, Start: end, End: m.End})
		case m.Start < start:
			kept = append(kept, Mark{Kind: kind, Start: m.Start, End: start}) // right edge trimmed
		default:
			kept = append(kept, Mark{Kind: kind, Start: end, End: m.End}) // left edge trimmed
		}
	}
	c.Marks = kept
	c.normalise()
	return nil
}

// MarksAt returns every MarkKind covering offset, in canonical (sorted)
// order. Ranges are half-open: a mark's End offset is not covered — a mark
// over [0,3) does not apply at offset 3.
func (c *Content) MarksAt(offset int) []MarkKind {
	var out []MarkKind
	for _, m := range c.Marks {
		if m.Start <= offset && offset < m.End {
			out = append(out, m.Kind)
		}
	}
	return out
}

// normalise restores Content's invariants after any mutation: same-kind
// marks that overlap or touch are merged into one, and the result is
// sorted by Start. Idempotent — calling it twice changes nothing, which is
// what "adding a mark fully covered by an existing one changes nothing"
// relies on.
func (c *Content) normalise() {
	if len(c.Marks) == 0 {
		return
	}

	byKind := make(map[MarkKind][]Mark, len(c.Marks))
	for _, m := range c.Marks {
		byKind[m.Kind] = append(byKind[m.Kind], m)
	}

	merged := make([]Mark, 0, len(c.Marks))
	for _, ms := range byKind {
		slices.SortFunc(ms, func(a, b Mark) int { return cmp.Compare(a.Start, b.Start) })
		run := ms[0]
		for _, m := range ms[1:] {
			if m.Start <= run.End { // overlaps or touches the current run
				if m.End > run.End {
					run.End = m.End
				}
				continue
			}
			merged = append(merged, run)
			run = m
		}
		merged = append(merged, run)
	}

	slices.SortFunc(merged, func(a, b Mark) int {
		if a.Start != b.Start {
			return cmp.Compare(a.Start, b.Start)
		}
		if a.Kind.Tag != b.Kind.Tag {
			return cmp.Compare(a.Kind.Tag, b.Kind.Tag)
		}
		if a.Kind.URL != b.Kind.URL {
			return cmp.Compare(a.Kind.URL, b.Kind.URL)
		}
		// bytes.Compare on the raw [16]byte UUID, not a and b.Kind.Page.String()
		// — PageID.String() formats a UUID (two allocations) on every
		// comparison, run O(n log n) times per mark mutation; the two
		// arrays compare just as correctly, and totally-ordered, without
		// allocating anything.
		ap, bp := uuid.UUID(a.Kind.Page), uuid.UUID(b.Kind.Page)
		return bytes.Compare(ap[:], bp[:])
	})
	c.Marks = merged
}
