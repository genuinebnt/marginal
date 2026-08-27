package documentcore

import (
	"fmt"
	"slices"
)

// Page is a page's title and its blocks. Blocks is one flat slice kept in
// depth-first order at all times — a container is always immediately
// followed by all of its descendants, then the next sibling (RFC-001 §1
// "Persisted form is not the in-memory form") — so a linear walk from the
// start already produces reading order, and every block's own Parent
// field (nil = top-level) is what actually encodes the tree; there is no
// separate per-level slice to keep in sync.
type Page struct {
	ID     PageID  `json:"id"`
	Title  string  `json:"title"`
	Blocks []Block `json:"blocks"`
}

func NewPage(id PageID, title string) Page {
	return Page{ID: id, Title: title, Blocks: []Block{}}
}

type BlockNotFoundError struct{ ID BlockID }

func (e *BlockNotFoundError) Error() string {
	return fmt.Sprintf("block not found: %s", e.ID)
}

type DuplicateBlockIDError struct{ ID BlockID }

func (e *DuplicateBlockIDError) Error() string {
	return fmt.Sprintf("block already exists: %s", e.ID)
}

// PositionMismatchError reports that an op's recorded predecessor no longer
// matches the block's actual current predecessor.
type PositionMismatchError struct {
	ID        BlockID
	Want, Got *BlockID
}

func (e *PositionMismatchError) Error() string {
	return fmt.Sprintf("block %s: op expected predecessor %s, actual predecessor is %s",
		e.ID, blockIDPtrString(e.Want), blockIDPtrString(e.Got))
}

// ParentMismatchError reports that an op's recorded parent no longer
// matches the block's actual current parent (nil = top-level) — the
// containment analogue of PositionMismatchError, checked wherever an op
// records a prior Parent (RFC-002 §1's uniform "records a prior value"
// rule, extended to containment by RFC-001 §1).
type ParentMismatchError struct {
	ID        BlockID
	Want, Got *BlockID
}

func (e *ParentMismatchError) Error() string {
	return fmt.Sprintf("block %s: op expected parent %s, actual parent is %s",
		e.ID, blockIDPtrString(e.Want), blockIDPtrString(e.Got))
}

// PreconditionError reports that an op's recorded "from" value no longer
// matches current state — see operation.go's doc comment on SetBlockKind
// for why every such op is checked uniformly.
type PreconditionError struct {
	Target string
	Field  string
}

func (e *PreconditionError) Error() string {
	return fmt.Sprintf("%s: current %s does not match op's expected prior value", e.Target, e.Field)
}

// NotAContainerError reports that an op named a Parent that exists but
// can't hold children — RFC-001 §1: only Quote, Toggle, List, and
// ListItem can (BlockTag.IsContainer).
type NotAContainerError struct{ ID BlockID }

func (e *NotAContainerError) Error() string {
	return fmt.Sprintf("block %s is not a container", e.ID)
}

// ContainerNotEmptyError reports an attempt to delete a container, or
// convert one to a leaf kind, while it still has children — RFC-001 §1:
// a whole-subtree delete has no clean Invert(), so this op is rejected
// rather than attempted; the caller deletes children first, one at a
// time, each individually invertible.
type ContainerNotEmptyError struct{ ID BlockID }

func (e *ContainerNotEmptyError) Error() string {
	return fmt.Sprintf("block %s: container has children, delete or convert them first", e.ID)
}

// InvalidListChildError reports a block placed under, or converted while
// under, a ListItem parent whose kind isn't List or Paragraph — RFC-001
// §1's ListChild ::= List | Paragraph restriction, the one child-kind
// rule specific to ListItem parents (every other container accepts any
// block kind as a child).
type InvalidListChildError struct {
	Parent BlockID
	Got    BlockTag
}

func (e *InvalidListChildError) Error() string {
	return fmt.Sprintf("block %s: a list item's child must be a list or a paragraph, got %s", e.Parent, e.Got)
}

// CycleError reports a move that would make ID its own ancestor — the
// same rule ReparentPage's ErrCycle already enforces for pages
// (docs/architecture/DATA_MODEL.md), checked here by walking the
// proposed new parent's own Parent chain rather than an LTREE prefix
// check, since documentcore has no materialised path to lean on.
type CycleError struct {
	ID        BlockID
	NewParent BlockID
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("cannot move block %s under %s: would create a cycle", e.ID, e.NewParent)
}

func blockIDPtrString(id *BlockID) string {
	if id == nil {
		return "<none>"
	}
	return id.String()
}

func indexOf(blocks []Block, id BlockID) (int, bool) {
	for i := range blocks {
		if blocks[i].ID == id {
			return i, true
		}
	}
	return -1, false
}

func (p *Page) indexOf(id BlockID) (int, bool) { return indexOf(p.Blocks, id) }

// blockIndex maps every block's id to its current slice index — built
// fresh by whichever Apply case needs repeated parent/ancestor lookups
// (InsertBlock, MoveBlock, and the container-emptiness checks), rather
// than re-scanning blocks (indexOf's own O(n) cost) for each one.
func blockIndex(blocks []Block) map[BlockID]int {
	m := make(map[BlockID]int, len(blocks))
	for i, b := range blocks {
		m[b.ID] = i
	}
	return m
}

// predecessorOf returns the id of the sibling immediately before
// blocks[i] — the nearest earlier block sharing blocks[i]'s own Parent —
// or nil if blocks[i] is its parent's (or the page's) first child. A
// backward scan, not blocks[i-1] directly: the immediately preceding
// slice element may be a sibling's own descendant instead (RFC-001 §1's
// depth-first order), which the caller must skip past.
func predecessorOf(blocks []Block, i int) *BlockID {
	for j := i - 1; j >= 0; j-- {
		if blockIDPtrEqual(blocks[j].Parent, blocks[i].Parent) {
			id := blocks[j].ID
			return &id
		}
	}
	return nil
}

func (p *Page) predecessorOf(i int) *BlockID { return predecessorOf(p.Blocks, i) }

func blockIDPtrEqual(a, b *BlockID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// isDescendant reports whether id is ancestorID's descendant, walking
// the Parent chain from id upward via byID.
func isDescendant(blocks []Block, byID map[BlockID]int, id, ancestorID BlockID) bool {
	for {
		i, ok := byID[id]
		if !ok {
			return false
		}
		parent := blocks[i].Parent
		if parent == nil {
			return false
		}
		if *parent == ancestorID {
			return true
		}
		id = *parent
	}
}

// subtreeEnd returns the index one past blocks[i]'s last descendant in
// blocks' depth-first order (i+1 if it has none) — the boundary a
// whole-subtree relocation (MoveBlock) or an emptiness check (DeleteBlock,
// SetBlockKind) needs.
func subtreeEnd(blocks []Block, byID map[BlockID]int, i int) int {
	id := blocks[i].ID
	j := i + 1
	for j < len(blocks) && isDescendant(blocks, byID, blocks[j].ID, id) {
		j++
	}
	return j
}

// validateParent checks that parent (if non-nil) names an existing
// container block, and — if that container is specifically a ListItem —
// that childTag obeys RFC-001 §1's ListChild ::= List | Paragraph
// restriction. Shared by InsertBlock and MoveBlock, the two ops that can
// place a block under a new Parent.
func validateParent(blocks []Block, byID map[BlockID]int, parent *BlockID, childTag BlockTag) error {
	if parent == nil {
		return nil
	}
	parentIdx, ok := byID[*parent]
	if !ok {
		return &BlockNotFoundError{ID: *parent}
	}
	parentTag := blocks[parentIdx].Kind.Tag
	if !parentTag.IsContainer() {
		return &NotAContainerError{ID: *parent}
	}
	if parentTag == ListItem && childTag != List && childTag != Paragraph {
		return &InvalidListChildError{Parent: *parent, Got: childTag}
	}
	return nil
}

// insertIndexAfterInParent resolves (parent, after) to an insertion index
// into blocks — the nested generalisation of a flat "insert after this
// id": nil after means "parent's first child" (or the page's first
// top-level block, if parent is also nil); a non-nil after must be an
// existing child of parent (ParentMismatchError otherwise), and the new
// block goes after all of after's own descendants (subtreeEnd),
// preserving the depth-first order invariant.
func insertIndexAfterInParent(blocks []Block, byID map[BlockID]int, parent, after *BlockID) (int, error) {
	if after == nil {
		if parent == nil {
			return 0, nil
		}
		parentIdx, ok := byID[*parent]
		if !ok {
			return 0, &BlockNotFoundError{ID: *parent}
		}
		return parentIdx + 1, nil
	}
	afterIdx, ok := byID[*after]
	if !ok {
		return 0, &BlockNotFoundError{ID: *after}
	}
	if !blockIDPtrEqual(blocks[afterIdx].Parent, parent) {
		return 0, &ParentMismatchError{ID: *after, Want: parent, Got: blocks[afterIdx].Parent}
	}
	return subtreeEnd(blocks, byID, afterIdx), nil
}

// insertAt inserts block into blocks at idx, shifting later elements right.
func insertAt(blocks []Block, idx int, block Block) []Block {
	return slices.Insert(blocks, idx, block)
}

// Apply mutates the page according to op, or leaves it unchanged and
// returns an error if op's preconditions don't hold against current state.
// RFC-002 §1: this is the only way the tree changes — nothing else in this
// package mutates a Page's Blocks.
func (p *Page) Apply(op Op) error {
	switch op := op.(type) {
	case InsertBlock:
		if _, exists := p.indexOf(op.ID); exists {
			return &DuplicateBlockIDError{ID: op.ID}
		}
		byID := blockIndex(p.Blocks)
		if err := validateParent(p.Blocks, byID, op.Parent, op.Kind.Tag); err != nil {
			return err
		}
		idx, err := insertIndexAfterInParent(p.Blocks, byID, op.Parent, op.After)
		if err != nil {
			return err
		}
		block := Block{ID: op.ID, Parent: op.Parent, Kind: op.Kind, Content: op.Content}
		p.Blocks = insertAt(p.Blocks, idx, block)
		return nil

	case DeleteBlock:
		i, ok := p.indexOf(op.Tombstone.ID)
		if !ok {
			return &BlockNotFoundError{ID: op.Tombstone.ID}
		}
		if !blockIDPtrEqual(p.Blocks[i].Parent, op.Tombstone.Parent) {
			return &ParentMismatchError{ID: op.Tombstone.ID, Want: op.Tombstone.Parent, Got: p.Blocks[i].Parent}
		}
		if got := p.predecessorOf(i); !blockIDPtrEqual(got, op.After) {
			return &PositionMismatchError{ID: op.Tombstone.ID, Want: op.After, Got: got}
		}
		byID := blockIndex(p.Blocks)
		if subtreeEnd(p.Blocks, byID, i) != i+1 {
			return &ContainerNotEmptyError{ID: op.Tombstone.ID}
		}
		p.Blocks = slices.Delete(p.Blocks, i, i+1)
		return nil

	case SetBlockKind:
		i, ok := p.indexOf(op.ID)
		if !ok {
			return &BlockNotFoundError{ID: op.ID}
		}
		if p.Blocks[i].Kind != op.From {
			return &PreconditionError{Target: op.ID.String(), Field: "kind"}
		}
		if op.From.Tag.IsContainer() && !op.To.Tag.IsContainer() {
			byID := blockIndex(p.Blocks)
			if subtreeEnd(p.Blocks, byID, i) != i+1 {
				return &ContainerNotEmptyError{ID: op.ID}
			}
		}
		if parent := p.Blocks[i].Parent; parent != nil {
			if parentIdx, ok := p.indexOf(*parent); ok && p.Blocks[parentIdx].Kind.Tag == ListItem {
				if op.To.Tag != List && op.To.Tag != Paragraph {
					return &InvalidListChildError{Parent: *parent, Got: op.To.Tag}
				}
			}
		}
		p.Blocks[i].Kind = op.To
		return nil

	case SetBlockContent:
		i, ok := p.indexOf(op.Block)
		if !ok {
			return &BlockNotFoundError{ID: op.Block}
		}
		if !p.Blocks[i].Content.Equal(op.Prev) {
			return &PreconditionError{Target: op.Block.String(), Field: "content"}
		}
		p.Blocks[i].Content = op.Content
		return nil

	case SetTitle:
		if p.Title != op.From {
			return &PreconditionError{Target: p.ID.String(), Field: "title"}
		}
		p.Title = op.To
		return nil

	case MoveBlock:
		i, ok := p.indexOf(op.ID)
		if !ok {
			return &BlockNotFoundError{ID: op.ID}
		}
		if !blockIDPtrEqual(p.Blocks[i].Parent, op.FromParent) {
			return &ParentMismatchError{ID: op.ID, Want: op.FromParent, Got: p.Blocks[i].Parent}
		}
		if got := p.predecessorOf(i); !blockIDPtrEqual(got, op.From) {
			return &PositionMismatchError{ID: op.ID, Want: op.From, Got: got}
		}

		byID := blockIndex(p.Blocks)
		if op.ToParent != nil {
			if *op.ToParent == op.ID || isDescendant(p.Blocks, byID, *op.ToParent, op.ID) {
				return &CycleError{ID: op.ID, NewParent: *op.ToParent}
			}
		}
		if err := validateParent(p.Blocks, byID, op.ToParent, p.Blocks[i].Kind.Tag); err != nil {
			return err
		}

		// subtree is an independent copy of blocks[i:end], taken before the
		// delete below touches p.Blocks' own backing array — the same
		// reasoning the single-block move this generalises already used.
		// Moving id relocates its whole subtree as one unit (RFC-001 §1's
		// depth-first order invariant): only the subtree's own root gets
		// reparented, every descendant keeps its existing Parent (each
		// other), unaffected by the move.
		end := subtreeEnd(p.Blocks, byID, i)
		subtree := append([]Block(nil), p.Blocks[i:end]...)
		withoutSubtree := slices.Delete(p.Blocks, i, end)

		idx, err := insertIndexAfterInParent(withoutSubtree, blockIndex(withoutSubtree), op.ToParent, op.To)
		if err != nil {
			return err
		}
		subtree[0].Parent = op.ToParent
		p.Blocks = slices.Insert(withoutSubtree, idx, subtree...)
		return nil

	default:
		return fmt.Errorf("documentcore: unknown op type %T", op)
	}
}
