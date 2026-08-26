package documentcore

// History is a per-actor undo/redo stack over Ops. Undo/Redo are atomic:
// if the inverse op fails to apply (e.g. the page changed underneath it in
// a way that breaks the precondition), the stacks are left exactly as they
// were, so a caller can retry or surface the error without losing state.
type History struct {
	undo     []Op
	redo     []Op
	maxDepth int
}

// NewHistory. maxDepth <= 0 means "record nothing" (every Record call is
// then a no-op, and Undo/Redo stay permanently empty) — a valid choice
// for a caller that wants Page.Apply's other behavior without paying for
// an undo stack, not a footgun to guard against.
func NewHistory(maxDepth int) *History {
	return &History{maxDepth: maxDepth}
}

func (h *History) UndoDepth() int { return len(h.undo) }
func (h *History) RedoDepth() int { return len(h.redo) }

// Record pushes op onto the undo stack and clears redo — a new op
// invalidates whatever branch redo was pointing at. If maxDepth is
// exceeded, the oldest recorded op is evicted.
func (h *History) Record(op Op) {
	if h.maxDepth <= 0 {
		return
	}
	h.undo = append(h.undo, op)
	h.redo = nil
	if len(h.undo) > h.maxDepth {
		// copy+reslice, not h.undo[1:]: slicing off the front advances
		// the slice header but keeps referencing the same backing array,
		// so append at the tail keeps writing past where the array
		// already is — over a long session the backing array (and every
		// evicted Op it's still holding onto — Content strings, Block
		// tombstones, not free) grows without bound even though len stays
		// capped at maxDepth. Shifting the live window down to index 0
		// lets the evicted entry's own storage actually be reclaimed.
		copy(h.undo, h.undo[1:])
		h.undo = h.undo[:h.maxDepth]
	}
}

// Undo applies the inverse of the most recently recorded op to page. A
// no-op (not an error) when the undo stack is empty.
func (h *History) Undo(page *Page) error {
	if len(h.undo) == 0 {
		return nil
	}
	last := len(h.undo) - 1
	undoOp := h.undo[last]
	redoOp := undoOp.Invert()

	if err := page.Apply(redoOp); err != nil {
		return err
	}
	h.undo = h.undo[:last]
	h.redo = append(h.redo, redoOp)
	return nil
}

// Redo re-applies the most recently undone op to page. A no-op (not an
// error) when the redo stack is empty.
func (h *History) Redo(page *Page) error {
	if len(h.redo) == 0 {
		return nil
	}
	last := len(h.redo) - 1
	redoOp := h.redo[last]
	undoOp := redoOp.Invert()

	if err := page.Apply(undoOp); err != nil {
		return err
	}
	h.redo = h.redo[:last]
	h.undo = append(h.undo, undoOp)
	return nil
}
