package documentcore

import (
	"fmt"
	"reflect"
)

// Page is a page's title and its blocks, in display order. The order is
// flat for now (After-pointer linked list, like a doubly-linked list
// materialized as a slice) — RFC-001 §1's real grammar has List/Toggle
// blocks that nest, which needs a parent/child tree (DATA_MODEL.md's
// adjacency list + LTREE path). That's added when a nesting block kind is
// actually implemented, not before.
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

func blockIDPtrString(id *BlockID) string {
	if id == nil {
		return "<start of page>"
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

// predecessorOf returns the id immediately before blocks[i], or nil if i is
// the first block.
func predecessorOf(blocks []Block, i int) *BlockID {
	if i == 0 {
		return nil
	}
	id := blocks[i-1].ID
	return &id
}

func (p *Page) predecessorOf(i int) *BlockID { return predecessorOf(p.Blocks, i) }

func blockIDPtrEqual(a, b *BlockID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// insertIndexAfter resolves an After pointer to an insertion index into
// blocks: 0 for nil, or one past the named block.
func insertIndexAfter(blocks []Block, after *BlockID) (int, error) {
	if after == nil {
		return 0, nil
	}
	i, ok := indexOf(blocks, *after)
	if !ok {
		return 0, &BlockNotFoundError{ID: *after}
	}
	return i + 1, nil
}

func (p *Page) insertIndexAfter(after *BlockID) (int, error) {
	return insertIndexAfter(p.Blocks, after)
}

// insertAt inserts block into blocks at idx, shifting later elements right.
func insertAt(blocks []Block, idx int, block Block) []Block {
	blocks = append(blocks, Block{})
	copy(blocks[idx+1:], blocks[idx:])
	blocks[idx] = block
	return blocks
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
		idx, err := p.insertIndexAfter(op.After)
		if err != nil {
			return err
		}
		block := Block{ID: op.ID, Kind: op.Kind, Content: op.Content}
		p.Blocks = insertAt(p.Blocks, idx, block)
		return nil

	case DeleteBlock:
		i, ok := p.indexOf(op.Tombstone.ID)
		if !ok {
			return &BlockNotFoundError{ID: op.Tombstone.ID}
		}
		if got := p.predecessorOf(i); !blockIDPtrEqual(got, op.After) {
			return &PositionMismatchError{ID: op.Tombstone.ID, Want: op.After, Got: got}
		}
		p.Blocks = append(p.Blocks[:i], p.Blocks[i+1:]...)
		return nil

	case SetBlockKind:
		i, ok := p.indexOf(op.ID)
		if !ok {
			return &BlockNotFoundError{ID: op.ID}
		}
		if p.Blocks[i].Kind != op.From {
			return &PreconditionError{Target: op.ID.String(), Field: "kind"}
		}
		p.Blocks[i].Kind = op.To
		return nil

	case SetBlockContent:
		i, ok := p.indexOf(op.Block)
		if !ok {
			return &BlockNotFoundError{ID: op.Block}
		}
		if !reflect.DeepEqual(p.Blocks[i].Content, op.Prev) {
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
		if got := p.predecessorOf(i); !blockIDPtrEqual(got, op.From) {
			return &PositionMismatchError{ID: op.ID, Want: op.From, Got: got}
		}
		block := p.Blocks[i]
		withoutBlock := append(p.Blocks[:i:i], p.Blocks[i+1:]...)
		idx, err := insertIndexAfter(withoutBlock, op.To)
		if err != nil {
			return err
		}
		p.Blocks = insertAt(withoutBlock, idx, block)
		return nil

	default:
		return fmt.Errorf("documentcore: unknown op type %T", op)
	}
}
