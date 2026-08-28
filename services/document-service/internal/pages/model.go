// Package pages owns docs.pages — page metadata (title, tree position,
// lifecycle), per docs/api/pages.md and DATA_MODEL.md §4. It does not own
// block content: that's internal/documentcore, a separate concern with no
// dependency on this package (and vice versa).
package pages

import (
	"time"

	"github.com/google/uuid"
)

// PageID is its own type, not documentcore.PageID — pages and
// documentcore are separate bounded contexts for now (metadata vs.
// content); sharing a type would couple them for no current benefit.
// DATA_MODEL.md/PROJECT_STRUCTURE's rule is to share only at the second
// consumer that actually needs it, and there isn't one yet.
type PageID uuid.UUID

func (id PageID) String() string { return uuid.UUID(id).String() }

type LifecycleState string

const (
	Active   LifecycleState = "active"
	Deleting LifecycleState = "deleting"
	Deleted  LifecycleState = "deleted"
)

// Page mirrors docs.pages, and (deliberately) the PageService proto
// message's field set — see docs/api/pages.md.
type Page struct {
	ID             PageID
	CreatedBy      uuid.UUID
	Title          string
	ParentID       *PageID
	Path           string // materialised LTREE ancestry, opaque to callers
	SortKey        string // opaque, lexicographic — see internal/sortkey
	LifecycleState LifecycleState
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Backlink is one page linking into another — docs.page_links
// (internal/blockproj's projection), read-only here: nothing in this
// package writes that table.
type Backlink struct {
	FromPage        PageID
	FromPageTitle   string
	FromPageDeleted bool
	TargetTitle     string
}

// BlockID is documentcore.BlockID's own identity, re-declared here rather
// than imported — this package (pages) doesn't otherwise depend on
// documentcore, and a raw uuid.UUID already carries everything ListBlocks'
// callers need; see PageID's own doc comment for the same reasoning.
type BlockID uuid.UUID

func (id BlockID) String() string { return uuid.UUID(id).String() }

// Block is one page's block, read-only here — docs.blocks
// (internal/blockproj's projection), the same "not this package's to
// write" boundary as Backlink above. KindJSON/ContentJSON are
// documentcore.BlockKind/Content's own JSON shapes, passed through as
// raw bytes rather than parsed: this package has no reason to understand
// block content, only to read it back out for a caller (diagnostics-service,
// v2.3.0) that does.
type Block struct {
	ID          BlockID
	ParentID    *BlockID
	KindJSON    []byte
	ContentJSON []byte
}

// NewPage is Create's input. After names the sibling the new page
// follows; nil means append at the end (docs/api/pages.md § Create).
type NewPage struct {
	CreatedBy uuid.UUID
	Title     string
	ParentID  *PageID
	After     *PageID
}

// TopicID is a topic's identity. Its own type rather than a bare uuid.UUID
// for the same reason PageID is: the two are structurally identical and
// swapping them at a call site would compile.
type TopicID uuid.UUID

func (t TopicID) String() string { return uuid.UUID(t).String() }

// Topic is one owned classification (v2.7.0, ui-mockups § 10b). ColorKey
// names a hue in the design system's categorical ramp; it is deliberately
// not a hex value, so retuning the palette does not need a data migration.
type Topic struct {
	ID        TopicID
	Name      string
	ColorKey  string
	PageCount int // live pages only; zero unless the query computed it
}

// TagFacet is one tag with its usage, for search's facet rail. TopicsSpanned
// is what makes "a tag that lives in three topics is doing real work"
// (ui-mockups § 10b) a computed fact rather than a claim.
type TagFacet struct {
	Tag           string
	PageCount     int
	TopicsSpanned int
}
