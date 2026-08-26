package documentcore

import (
	"encoding/json"
	"fmt"
)

// BlockTag is a block's variant discriminant. RFC-001 §1's grammar has more
// variants (List, Toggle, Image) than this package implements — those need
// a real parent/child tree (adjacency list + LTREE path per DATA_MODEL.md),
// which this first slice doesn't have yet. Added when a block kind that
// needs nesting actually lands, not before (agents.md §3: feature depth,
// not surface area).
type BlockTag uint8

const (
	Paragraph BlockTag = iota
	Heading
	Quote
	CodeBlock
	Divider
)

func (t BlockTag) String() string {
	switch t {
	case Paragraph:
		return "paragraph"
	case Heading:
		return "heading"
	case Quote:
		return "quote"
	case CodeBlock:
		return "code_block"
	case Divider:
		return "divider"
	default:
		return fmt.Sprintf("BlockTag(%d)", uint8(t))
	}
}

// BlockKind is validated at construction, never a bare tag — this resolves
// docs/porting/OPEN_QUESTIONS.md's inherited note that the deleted Rust
// draft's Heading{level} accepted 0 and 255 unchecked.
//
// Level is meaningful only when Tag == Heading; Language only when
// Tag == CodeBlock. A tagged struct (rather than one type per tag behind an
// interface) is the idiomatic Go shape here: the field set is small, fixed,
// and comparable, so callers can use == without a type switch.
type BlockKind struct {
	Tag      BlockTag
	Level    uint8  // 1..=3, Heading only
	Language string // CodeBlock only; block-level attribute (which grammar to
	// syntax-highlight), not a per-character mark — see Content for the
	// inline-formatting axis this is deliberately not part of.
}

// InvalidHeadingLevelError reports a Heading level outside RFC-001's
// documented range.
type InvalidHeadingLevelError struct{ Level uint8 }

func (e *InvalidHeadingLevelError) Error() string {
	return fmt.Sprintf("invalid heading level %d: must be 1..=3", e.Level)
}

func NewParagraph() BlockKind { return BlockKind{Tag: Paragraph} }
func NewQuote() BlockKind     { return BlockKind{Tag: Quote} }
func NewDivider() BlockKind   { return BlockKind{Tag: Divider} }

func NewCodeBlock(language string) BlockKind {
	return BlockKind{Tag: CodeBlock, Language: language}
}

func NewHeading(level uint8) (BlockKind, error) {
	if level < 1 || level > 3 {
		return BlockKind{}, &InvalidHeadingLevelError{Level: level}
	}
	return BlockKind{Tag: Heading, Level: level}, nil
}

// blockKindJSON is the wire shape: {"tag":"heading","level":2},
// {"tag":"code_block","language":"go"}, or just {"tag":"paragraph"} —
// unused fields omitted. Same shape testdata/document-core vectors use, so
// the WASM boundary, the future DATA_MODEL.md JSONB column, and the test
// fixtures all agree on one encoding.
type blockKindJSON struct {
	Tag      string `json:"tag"`
	Level    uint8  `json:"level,omitempty"`
	Language string `json:"language,omitempty"`
}

func (k BlockKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(blockKindJSON{Tag: k.Tag.String(), Level: k.Level, Language: k.Language})
}

func (k *BlockKind) UnmarshalJSON(data []byte) error {
	var raw blockKindJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch raw.Tag {
	case "paragraph":
		*k = NewParagraph()
	case "quote":
		*k = NewQuote()
	case "divider":
		*k = NewDivider()
	case "code_block":
		*k = NewCodeBlock(raw.Language)
	case "heading":
		hk, err := NewHeading(raw.Level)
		if err != nil {
			return err
		}
		*k = hk
	default:
		return fmt.Errorf("documentcore: unknown block tag %q", raw.Tag)
	}
	return nil
}

// Block is a page's unit of structure: an id, a kind, and its inline
// content. Exported fields, no getters — documentcore never mutates a Block
// in place (RFC-002 §1: every change is an Op); callers replace blocks by
// value instead.
type Block struct {
	ID      BlockID   `json:"id"`
	Kind    BlockKind `json:"kind"`
	Content Content   `json:"content"`
}
