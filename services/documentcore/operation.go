package documentcore

// Op is the block-granular tier of RFC-002 §2's ISA — the part that needs
// no rope: InsertBlock, DeleteBlock, SetBlockKind, SetBlockContent,
// SetTitle, MoveBlock. Character-granular ops (InsertText, DeleteText,
// SetMark over an Anchor/AnchorRange) belong to collaboration-service's
// rope once Phase 3 exists; document-core doesn't need them.
//
// This package models Op as an interface with one concrete type per
// variant rather than a tagged struct (unlike BlockKind/MarkKind) because
// the variants' field sets are large and unrelated enough that a shared
// struct would carry mostly-unused fields — the standard shape for a closed
// Go sum type with heterogeneous payloads (compare go/ast.Node).
type Op interface {
	// Invert returns the op that undoes this one. RFC-002 §3: invertibility
	// is a design constraint, not a later feature — apply(invert(op),
	// apply(op, page)) must equal page for every op and every page it can
	// apply to. Every variant below records what it overwrites for exactly
	// this reason.
	Invert() Op
	isOp()
}

// InsertBlock inserts Kind/Content as a new block with id ID, as a child
// of Parent (nil = top-level), immediately after the sibling named by
// After (or as Parent's first child — or the page's first top-level
// block, if Parent is also nil — if After is nil).
type InsertBlock struct {
	ID      BlockID   `json:"id"`
	Parent  *BlockID  `json:"parent"`
	After   *BlockID  `json:"after"`
	Kind    BlockKind `json:"kind"`
	Content Content   `json:"content"`
}

func (InsertBlock) isOp() {}

func (op InsertBlock) Invert() Op {
	return DeleteBlock{
		Tombstone: Block{ID: op.ID, Parent: op.Parent, Kind: op.Kind, Content: op.Content},
		After:     op.After,
	}
}

// DeleteBlock removes Tombstone.ID from the page. Tombstone carries the
// full block (including its Parent — RFC-001 §1's containment) so Invert
// can reinsert it exactly; After must equal the block's actual predecessor
// at apply time (Page.Apply checks this) — the deleted Rust attempt's
// open-decisions list flagged that an unchecked After lets a wrong
// position apply cleanly and silently restore the block to the wrong
// place on undo. Checking it here, uniformly for every "records a prior
// value" op, is the fix.
//
// Only an empty container (or a leaf) may be deleted — Page.Apply rejects
// deleting a non-empty Quote/Toggle/List/ListItem (ContainerNotEmptyError).
// A whole-subtree delete has no clean Invert(): reinserting a subtree
// atomically is a different, harder operation than reinserting one block,
// so this repo's scope stops at requiring the caller to delete a
// container's children first, each individually invertible.
type DeleteBlock struct {
	Tombstone Block    `json:"tombstone"`
	After     *BlockID `json:"after"`
}

func (DeleteBlock) isOp() {}

func (op DeleteBlock) Invert() Op {
	return InsertBlock{
		ID:      op.Tombstone.ID,
		Parent:  op.Tombstone.Parent,
		After:   op.After,
		Kind:    op.Tombstone.Kind,
		Content: op.Tombstone.Content,
	}
}

// SetBlockKind changes ID's kind from From to To. Page.Apply checks the
// block's current kind equals From before applying — an op whose recorded
// "from" no longer matches current state fails loudly rather than
// corrupting it silently (the same principle DeleteBlock.After above
// applies, generalised to every op that records a prior value).
//
// Converting a non-empty container (From.Tag.IsContainer()) to a leaf
// kind is rejected (ContainerNotEmptyError) — same reasoning as
// DeleteBlock. Converting a block whose own Parent is a ListItem to
// anything other than List or Paragraph is rejected too
// (InvalidListChildError) — RFC-001 §1's ListChild ::= List | Paragraph
// restriction, the one child-kind rule specific to ListItem parents.
type SetBlockKind struct {
	ID   BlockID   `json:"id"`
	From BlockKind `json:"from"`
	To   BlockKind `json:"to"`
}

func (SetBlockKind) isOp() {}

func (op SetBlockKind) Invert() Op {
	return SetBlockKind{ID: op.ID, From: op.To, To: op.From}
}

// SetBlockContent changes Block's content from Prev to Content. Field
// names match RFC-002 §2's ISA exactly (block/content/prev) — the deleted
// Rust draft's UpdateBlockContent{id, old_content, new_content} drifted
// from this, which is one of the reasons it isn't the reference here.
type SetBlockContent struct {
	Block   BlockID `json:"block"`
	Prev    Content `json:"prev"`
	Content Content `json:"content"`
}

func (SetBlockContent) isOp() {}

func (op SetBlockContent) Invert() Op {
	return SetBlockContent{Block: op.Block, Prev: op.Content, Content: op.Prev}
}

// SetTitle changes Page's title from From to To.
type SetTitle struct {
	Page PageID `json:"page"`
	From string `json:"from"`
	To   string `json:"to"`
}

func (SetTitle) isOp() {}

func (op SetTitle) Invert() Op {
	return SetTitle{Page: op.Page, From: op.To, To: op.From}
}

// MoveBlock relocates ID from being FromParent's child (nil = top-level)
// immediately after sibling From, to being ToParent's child immediately
// after sibling To (nil means "first child of the parent" — or "first
// top-level block" if the parent is also nil — for either From or To).
//
// If ID is a container with children, the whole subtree moves as one
// unit (RFC-001 §1's depth-first order invariant: a parent is always
// immediately followed by all its descendants) — the only op that ever
// relocates more than one block, since InsertBlock only ever creates one
// block and DeleteBlock only ever removes an empty one. Page.Apply
// rejects a move that would make ID its own ancestor (CycleError, the
// same rule ReparentPage's ErrCycle already enforces for pages) and
// checks ToParent's container-ness and, if ToParent is a ListItem, ID's
// own kind against ListChild's restriction — the same two checks
// InsertBlock's Parent already needs.
type MoveBlock struct {
	ID         BlockID  `json:"id"`
	FromParent *BlockID `json:"from_parent"`
	From       *BlockID `json:"from"`
	ToParent   *BlockID `json:"to_parent"`
	To         *BlockID `json:"to"`
}

func (MoveBlock) isOp() {}

func (op MoveBlock) Invert() Op {
	return MoveBlock{ID: op.ID, FromParent: op.ToParent, From: op.To, ToParent: op.FromParent, To: op.From}
}
