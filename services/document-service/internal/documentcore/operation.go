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

// InsertBlock inserts Kind/Content as a new block with id ID, immediately
// after the block named by After (or at the start of the page if After is
// nil).
type InsertBlock struct {
	ID      BlockID   `json:"id"`
	After   *BlockID  `json:"after"`
	Kind    BlockKind `json:"kind"`
	Content Content   `json:"content"`
}

func (InsertBlock) isOp() {}

func (op InsertBlock) Invert() Op {
	return DeleteBlock{
		Tombstone: Block{ID: op.ID, Kind: op.Kind, Content: op.Content},
		After:     op.After,
	}
}

// DeleteBlock removes Tombstone.ID from the page. Tombstone carries the
// full block so Invert can reinsert it; After must equal the block's
// actual predecessor at apply time (Page.Apply checks this) — the deleted
// Rust attempt's open-decisions list flagged that an unchecked After lets a
// wrong position apply cleanly and silently restore the block to the wrong
// place on undo. Checking it here, uniformly for every "records a prior
// value" op, is the fix.
type DeleteBlock struct {
	Tombstone Block    `json:"tombstone"`
	After     *BlockID `json:"after"`
}

func (DeleteBlock) isOp() {}

func (op DeleteBlock) Invert() Op {
	return InsertBlock{
		ID:      op.Tombstone.ID,
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

// MoveBlock relocates ID from immediately after From to immediately after
// To (nil means "at the start of the page" for either).
type MoveBlock struct {
	ID   BlockID  `json:"id"`
	From *BlockID `json:"from"`
	To   *BlockID `json:"to"`
}

func (MoveBlock) isOp() {}

func (op MoveBlock) Invert() Op {
	return MoveBlock{ID: op.ID, From: op.To, To: op.From}
}
