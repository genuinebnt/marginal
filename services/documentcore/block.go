package documentcore

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// BlockTag is a block's variant discriminant — RFC-001 §1's full grammar:
// Paragraph, Heading, List, Quote, Code, Toggle, Image, Divider. Callout
// and Aside are RFC-001 §10's first implemented slice of its larger
// target grammar — the "zero-new-mechanism containers" §10.4 names: the
// exact same shape Quote/Toggle already have, nothing more.
type BlockTag uint8

const (
	Paragraph BlockTag = iota
	Heading
	Quote
	CodeBlock
	Divider
	List
	ListItem
	Toggle
	Image
	Callout
	Aside
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
	case List:
		return "list"
	case ListItem:
		return "list_item"
	case Toggle:
		return "toggle"
	case Image:
		return "image"
	case Callout:
		return "callout"
	case Aside:
		return "aside"
	default:
		return fmt.Sprintf("BlockTag(%d)", uint8(t))
	}
}

// IsContainer reports whether a block of this tag can hold children —
// Quote, Toggle, List, ListItem, Callout, and Aside (RFC-001 §1, §10);
// every other tag is a leaf. Page.Apply is the one place that enforces
// this (NotAContainerError) — callers are never trusted to check it
// themselves. Callout/Aside accept any child kind, the same as
// Quote/Toggle — only a ListItem parent restricts its children's kind
// (validateParent in page.go).
func (t BlockTag) IsContainer() bool {
	switch t {
	case Quote, Toggle, List, ListItem, Callout, Aside:
		return true
	default:
		return false
	}
}

// ListKind is List's own discriminant — RFC-001 §1: bulleted | numbered |
// todo.
type ListKind uint8

const (
	Bulleted ListKind = iota
	Numbered
	Todo
)

func (k ListKind) String() string {
	switch k {
	case Bulleted:
		return "bulleted"
	case Numbered:
		return "numbered"
	case Todo:
		return "todo"
	default:
		return fmt.Sprintf("ListKind(%d)", uint8(k))
	}
}

// InvalidListKindError reports a list_kind string ListKind doesn't
// recognise.
type InvalidListKindError struct{ Value string }

func (e *InvalidListKindError) Error() string {
	return fmt.Sprintf("invalid list kind %q", e.Value)
}

func parseListKind(s string) (ListKind, error) {
	switch s {
	case "bulleted":
		return Bulleted, nil
	case "numbered":
		return Numbered, nil
	case "todo":
		return Todo, nil
	default:
		return 0, &InvalidListKindError{Value: s}
	}
}

// CalloutTone is Callout's own discriminant — RFC-001 §10.2's six
// semantic tones, adopted from genuine-folio's own CALLOUT_VARIANTS
// (frontend/lib/directives.ts there). Not the Notion-palette superset
// §10.2's own comment describes and declines to adopt: nothing in
// Marginal imports or exports Notion content, so those extra variants
// would have no reader — add them if that ever changes, not on spec.
type CalloutTone uint8

const (
	Warn CalloutTone = iota // the historical default — genuine-folio's own "no keyword given" case
	Note
	Info
	Tip
	Danger
	Success
)

func (t CalloutTone) String() string {
	switch t {
	case Warn:
		return "warn"
	case Note:
		return "note"
	case Info:
		return "info"
	case Tip:
		return "tip"
	case Danger:
		return "danger"
	case Success:
		return "success"
	default:
		return fmt.Sprintf("CalloutTone(%d)", uint8(t))
	}
}

// InvalidCalloutToneError reports a tone string CalloutTone doesn't
// recognise.
type InvalidCalloutToneError struct{ Value string }

func (e *InvalidCalloutToneError) Error() string {
	return fmt.Sprintf("invalid callout tone %q", e.Value)
}

// parseCalloutTone treats "" the same as "warn" — an omitted tone is a
// legitimate, common case (genuine-folio's own default), not malformed
// input; an actually-unrecognised string still errors.
func parseCalloutTone(s string) (CalloutTone, error) {
	switch s {
	case "", "warn":
		return Warn, nil
	case "note":
		return Note, nil
	case "info":
		return Info, nil
	case "tip":
		return Tip, nil
	case "danger":
		return Danger, nil
	case "success":
		return Success, nil
	default:
		return 0, &InvalidCalloutToneError{Value: s}
	}
}

// BlockKind is validated at construction, never a bare tag — this resolves
// docs/porting/OPEN_QUESTIONS.md's inherited note that the deleted Rust
// draft's Heading{level} accepted 0 and 255 unchecked.
//
// Level is meaningful only when Tag == Heading; Language only when
// Tag == CodeBlock; ListKindOf only when Tag == List; Checked only when
// Tag == ListItem and its parent List's ListKindOf is Todo; FileID only
// when Tag == Image; Tone and Icon only when Tag == Callout; Emoji only
// when Tag == Aside. Checked is a plain bool, not *bool — RFC-001 §1's
// "Checked?" optionality has no real third state to distinguish from
// "unchecked" at this repo's scope, and a pointer field would silently
// break BlockKind's own == comparability (Page.Apply's SetBlockKind
// precondition depends on it): two logically-equal Checked values behind
// different pointers compare unequal under ==, which a plain bool can't
// do. A tagged struct (rather than one type per tag behind an interface)
// is the idiomatic Go shape here: the field set is small, fixed, and
// comparable, so callers can use == without a type switch.
type BlockKind struct {
	Tag        BlockTag
	Level      uint8    // 1..=3, Heading only
	Language   string   // CodeBlock only; block-level attribute (which grammar to
	// syntax-highlight), not a per-character mark — see Content for the
	// inline-formatting axis this is deliberately not part of.
	ListKindOf ListKind    // List only
	Checked    bool        // ListItem only, meaningful only under a Todo list
	FileID     FileID      // Image only; no upload/asset pipeline backs this yet (RFC-001 §1)
	Tone       CalloutTone // Callout only
	Icon       string      // Callout only; an emoji glyph, "" = no icon. RFC-001 §10.2's
	// Icon ::= Emoji | FileId only implements the Emoji half — a
	// file-backed icon has the same "no asset pipeline yet" gap Image's
	// own FileID already has, and nothing needs it yet.
	Emoji string // Aside only
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
func NewToggle() BlockKind    { return BlockKind{Tag: Toggle} }

func NewCodeBlock(language string) BlockKind {
	return BlockKind{Tag: CodeBlock, Language: language}
}

func NewHeading(level uint8) (BlockKind, error) {
	if level < 1 || level > 3 {
		return BlockKind{}, &InvalidHeadingLevelError{Level: level}
	}
	return BlockKind{Tag: Heading, Level: level}, nil
}

func NewList(kind ListKind) BlockKind { return BlockKind{Tag: List, ListKindOf: kind} }

// NewListItem. checked is only meaningful once inserted under a Todo
// list — Page.Apply doesn't reject a checked ListItem under a non-todo
// list (that would need to know the parent at construction time, before
// the block has one), it's simply ignored by every reader.
func NewListItem(checked bool) BlockKind { return BlockKind{Tag: ListItem, Checked: checked} }

func NewImage(fileID FileID) BlockKind { return BlockKind{Tag: Image, FileID: fileID} }

func NewCallout(tone CalloutTone, icon string) BlockKind {
	return BlockKind{Tag: Callout, Tone: tone, Icon: icon}
}

func NewAside(emoji string) BlockKind { return BlockKind{Tag: Aside, Emoji: emoji} }

// blockKindJSON is the wire shape: {"tag":"heading","level":2},
// {"tag":"code_block","language":"go"}, {"tag":"list","list_kind":"todo"},
// {"tag":"list_item","checked":true}, {"tag":"image","file_id":"<uuid>"},
// or just {"tag":"paragraph"} — unused fields omitted. Same shape
// testdata/document-core vectors use, so the WASM boundary, DATA_MODEL.md's
// JSONB column, and the test fixtures all agree on one encoding. FileID is
// wired as a string (like MarkKind.Page in inline.go) rather than FileID's
// own array-backed type directly, since encoding/json's omitempty never
// treats a fixed-size-array-backed type as empty regardless of its
// contents — a plain string correctly omits when unset.
type blockKindJSON struct {
	Tag      string `json:"tag"`
	Level    uint8  `json:"level,omitempty"`
	Language string `json:"language,omitempty"`
	ListKind string `json:"list_kind,omitempty"`
	Checked  bool   `json:"checked,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	Tone     string `json:"tone,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Emoji    string `json:"emoji,omitempty"`
}

func (k BlockKind) MarshalJSON() ([]byte, error) {
	raw := blockKindJSON{
		Tag:      k.Tag.String(),
		Level:    k.Level,
		Language: k.Language,
		Checked:  k.Checked,
		Icon:     k.Icon,
		Emoji:    k.Emoji,
	}
	if k.Tag == List {
		raw.ListKind = k.ListKindOf.String()
	}
	if k.Tag == Image {
		raw.FileID = k.FileID.String()
	}
	if k.Tag == Callout {
		raw.Tone = k.Tone.String()
	}
	return json.Marshal(raw)
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
	case "toggle":
		*k = NewToggle()
	case "code_block":
		*k = NewCodeBlock(raw.Language)
	case "heading":
		hk, err := NewHeading(raw.Level)
		if err != nil {
			return err
		}
		*k = hk
	case "list":
		lk, err := parseListKind(raw.ListKind)
		if err != nil {
			return err
		}
		*k = NewList(lk)
	case "list_item":
		*k = NewListItem(raw.Checked)
	case "image":
		id, err := uuid.Parse(raw.FileID)
		if err != nil {
			return fmt.Errorf("documentcore: invalid image file_id %q: %w", raw.FileID, err)
		}
		*k = NewImage(FileID(id))
	case "callout":
		tone, err := parseCalloutTone(raw.Tone)
		if err != nil {
			return err
		}
		*k = NewCallout(tone, raw.Icon)
	case "aside":
		*k = NewAside(raw.Emoji)
	default:
		return fmt.Errorf("documentcore: unknown block tag %q", raw.Tag)
	}
	return nil
}

// Block is a page's unit of structure: an id, its parent (nil = top-level
// — RFC-001 §1's containment), a kind, and its inline content. Exported
// fields, no getters — documentcore never mutates a Block in place
// (RFC-002 §1: every change is an Op); callers replace blocks by value
// instead.
type Block struct {
	ID      BlockID   `json:"id"`
	Parent  *BlockID  `json:"parent"`
	Kind    BlockKind `json:"kind"`
	Content Content   `json:"content"`
}
