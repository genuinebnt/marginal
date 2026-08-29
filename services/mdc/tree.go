package mdc

import "fmt"

// BlockKind mirrors documentcore's wire shape field for field. Same reason
// MarkTag does: what the two must agree on is the JSON, not the type.
type BlockKind struct {
	Tag      string `json:"tag"`
	Level    int    `json:"level,omitempty"`
	Language string `json:"language,omitempty"`
	ListKind string `json:"list_kind,omitempty"`
	Checked  bool   `json:"checked,omitempty"`
}

// Block is one node of the lowered tree. Flat store plus parent pointers —
// the same shape documentcore uses, and for the same three reasons: ops
// address blocks by id, the projection is flat, and a tree of owned children
// makes "mutate one while reading its parent" awkward in every language.
type Block struct {
	ID      string    `json:"id"`
	Parent  string    `json:"parent,omitempty"`
	Kind    BlockKind `json:"kind"`
	Content Content   `json:"content"`
}

// Tree is the lowered document: blocks in DEPTH-FIRST order, which is the
// order they must be created in and the order docs.blocks stores.
type Tree struct {
	Blocks []Block `json:"blocks"`
}

// Op is one emitted operation, in `internal/pageop`'s own wire shape — so the
// editor can send what this produces without translating it. That is the
// point: paste is not a special path, it is a batch of ordinary ops.
type Op struct {
	Scope   string    `json:"scope"` // always "block" here
	Type    string    `json:"type"`  // always "InsertBlock"
	ID      string    `json:"id"`
	Parent  *string   `json:"parent"`
	After   *string   `json:"after"`
	Kind    BlockKind `json:"kind"`
	Content Content   `json:"content"`
}

// Equal compares two trees field by field, in order, and says WHERE they
// differ rather than only that they do.
//
// This is the comparison § 11's header reads: it says HOLDS because a
// comparison happened, not because we are confident. reflect.DeepEqual would
// answer the same question and be unable to say which field.
func (t Tree) Equal(other Tree) (bool, string) {
	if len(t.Blocks) != len(other.Blocks) {
		return false, fmt.Sprintf("block count: %d vs %d", len(t.Blocks), len(other.Blocks))
	}
	for i := range t.Blocks {
		a, b := t.Blocks[i], other.Blocks[i]
		if a.ID != b.ID {
			return false, fmt.Sprintf("block %d id: %s vs %s", i, a.ID, b.ID)
		}
		if a.Parent != b.Parent {
			return false, fmt.Sprintf("block %d parent: %q vs %q", i, a.Parent, b.Parent)
		}
		if a.Kind != b.Kind {
			return false, fmt.Sprintf("block %d kind: %+v vs %+v", i, a.Kind, b.Kind)
		}
		if a.Content.Text != b.Content.Text {
			return false, fmt.Sprintf("block %d text: %q vs %q", i, a.Content.Text, b.Content.Text)
		}
		if len(a.Content.Marks) != len(b.Content.Marks) {
			return false, fmt.Sprintf("block %d mark count: %d vs %d", i, len(a.Content.Marks), len(b.Content.Marks))
		}
		for m := range a.Content.Marks {
			if a.Content.Marks[m] != b.Content.Marks[m] {
				return false, fmt.Sprintf("block %d mark %d: %+v vs %+v", i, m, a.Content.Marks[m], b.Content.Marks[m])
			}
		}
	}
	return true, ""
}

// Replay rebuilds a tree from ops, exactly as a fresh page would.
//
// It is a REAL apply, not a shortcut: `after` names a sibling that must
// already exist, and `parent` a block that must already exist. That is what
// makes the round-trip property meaningful — if emission produced an order
// where a child preceded its parent, this fails rather than silently
// tolerating it.
func Replay(ops []Op) (Tree, error) {
	byID := map[string]Block{}
	var order []string

	for i, op := range ops {
		if op.Type != "InsertBlock" {
			return Tree{}, fmt.Errorf("mdc: replay: op %d: unsupported type %q", i, op.Type)
		}
		if _, dup := byID[op.ID]; dup {
			return Tree{}, fmt.Errorf("mdc: replay: op %d: duplicate block %s", i, op.ID)
		}
		parent := ""
		if op.Parent != nil {
			parent = *op.Parent
			if _, ok := byID[parent]; !ok {
				return Tree{}, fmt.Errorf("mdc: replay: op %d: parent %s does not exist yet", i, parent)
			}
		}
		at := len(order)
		if op.After != nil {
			found := -1
			for j, id := range order {
				if id == *op.After {
					found = j
					break
				}
			}
			if found < 0 {
				return Tree{}, fmt.Errorf("mdc: replay: op %d: after %s does not exist yet", i, *op.After)
			}
			// Insert after the whole SUBTREE of `after`, not immediately
			// after the row — otherwise a new sibling lands between a block
			// and its own children, which is a different tree.
			at = found + 1
			for at < len(order) && descends(byID, order[at], *op.After) {
				at++
			}
		} else if parent != "" {
			// First child: immediately after the parent.
			for j, id := range order {
				if id == parent {
					at = j + 1
					break
				}
			}
		}
		order = append(order, "")
		copy(order[at+1:], order[at:])
		order[at] = op.ID
		byID[op.ID] = Block{ID: op.ID, Parent: parent, Kind: op.Kind, Content: op.Content}
	}

	out := Tree{Blocks: make([]Block, 0, len(order))}
	for _, id := range order {
		out.Blocks = append(out.Blocks, byID[id])
	}
	return out, nil
}

func descends(byID map[string]Block, id, ancestor string) bool {
	for cur := byID[id].Parent; cur != ""; cur = byID[cur].Parent {
		if cur == ancestor {
			return true
		}
	}
	return false
}
