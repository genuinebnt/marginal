package documentcore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pageWithOneBlock() (Page, BlockID) {
	page := NewPage(newTestPageID(), "Title")
	id := newTestBlockID()
	_ = page.Apply(InsertBlock{ID: id, Kind: NewParagraph(), Content: PlainContent("")})
	return page, id
}

func retypeBlock(id BlockID) Op {
	c := PlainContent("hello")
	_ = c.AddMark(NewBold(), 0, 5)
	return SetBlockContent{Block: id, Prev: PlainContent(""), Content: c}
}

// doOp applies op and records it, the way a caller is expected to pair them.
func doOp(t *testing.T, h *History, page *Page, op Op) {
	t.Helper()
	require.NoError(t, page.Apply(op))
	h.Record(op)
}

func TestUndoRestoresThePreOpPage(t *testing.T) {
	page, id := pageWithOneBlock()
	h := NewHistory(8)
	before := page.Blocks[0].Content

	doOp(t, h, &page, retypeBlock(id))
	require.NotEqual(t, before, page.Blocks[0].Content)

	require.NoError(t, h.Undo(&page))
	assert.Equal(t, before, page.Blocks[0].Content)
}

func TestRedoRestoresThePostOpPage(t *testing.T) {
	page, id := pageWithOneBlock()
	h := NewHistory(8)

	doOp(t, h, &page, retypeBlock(id))
	after := page.Blocks[0].Content

	require.NoError(t, h.Undo(&page))
	require.NoError(t, h.Redo(&page))
	assert.Equal(t, after, page.Blocks[0].Content)
}

func TestUndoRedoCyclesConverge(t *testing.T) {
	page, id := pageWithOneBlock()
	h := NewHistory(8)
	before := page.Blocks[0].Content

	doOp(t, h, &page, retypeBlock(id))
	after := page.Blocks[0].Content

	for i := 0; i < 10; i++ {
		require.NoError(t, h.Undo(&page))
		assert.Equal(t, before, page.Blocks[0].Content, "drifted from pre-op state on cycle %d", i)

		require.NoError(t, h.Redo(&page))
		assert.Equal(t, after, page.Blocks[0].Content, "drifted from post-op state on cycle %d", i)
	}
}

func TestRecordingANewOpClearsTheRedoStack(t *testing.T) {
	page, id := pageWithOneBlock()
	h := NewHistory(8)

	doOp(t, h, &page, retypeBlock(id))
	require.NoError(t, h.Undo(&page))
	require.Equal(t, 1, h.RedoDepth(), "undo should have filled redo")

	heading, err := NewHeading(2)
	require.NoError(t, err)
	doOp(t, h, &page, SetBlockKind{ID: id, From: NewParagraph(), To: heading})

	assert.Equal(t, 0, h.RedoDepth(), "a new op must invalidate the redo branch")
}

func TestUndoOnEmptyHistoryLeavesThePageAlone(t *testing.T) {
	page, _ := pageWithOneBlock()
	h := NewHistory(8)
	before := page.Blocks[0].Content

	require.NoError(t, h.Undo(&page))
	require.NoError(t, h.Redo(&page))

	assert.Equal(t, before, page.Blocks[0].Content)
	assert.Equal(t, 0, h.UndoDepth())
	assert.Equal(t, 0, h.RedoDepth())
}

func TestMaxDepthEvictsTheOldestOp(t *testing.T) {
	page, id := pageWithOneBlock()
	h := NewHistory(2)

	h1, err := NewHeading(1)
	require.NoError(t, err)
	h2, err := NewHeading(2)
	require.NoError(t, err)
	h3, err := NewHeading(3)
	require.NoError(t, err)
	sequence := []BlockKind{h1, h2, h3, NewQuote()}

	kind := NewParagraph()
	for _, next := range sequence {
		doOp(t, h, &page, SetBlockKind{ID: id, From: kind, To: next})
		kind = next
	}

	assert.Equal(t, 2, h.UndoDepth(), "undo stack must respect max_depth")

	for i := 0; i < 5; i++ {
		require.NoError(t, h.Undo(&page), "undo must not error even past the retained depth")
	}
}

func TestFailedUndoOrRedoLeavesBothStacksUnchanged(t *testing.T) {
	tests := []struct {
		name string
		// setup runs after the recorded op but before the block disappears
		// behind history's back — Redo needs a prior Undo to have something
		// to redo at all.
		setup       func(t *testing.T, h *History, page *Page)
		act         func(h *History, page *Page) error
		wantErrMsg  string
		wantUndoMsg string
		wantRedoMsg string
	}{
		{
			name:        "undo",
			setup:       func(t *testing.T, h *History, page *Page) {},
			act:         func(h *History, page *Page) error { return h.Undo(page) },
			wantErrMsg:  "undo must report that the inverse could not apply",
			wantUndoMsg: "a failed undo must not consume the op",
			wantRedoMsg: "a failed undo must not push onto redo",
		},
		{
			name: "redo",
			setup: func(t *testing.T, h *History, page *Page) {
				require.NoError(t, h.Undo(page))
			},
			act:         func(h *History, page *Page) error { return h.Redo(page) },
			wantErrMsg:  "redo must report that the op could not apply",
			wantUndoMsg: "a failed redo must not push onto undo",
			wantRedoMsg: "a failed redo must not consume the op",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, id := pageWithOneBlock()
			h := NewHistory(8)

			doOp(t, h, &page, retypeBlock(id))
			tc.setup(t, h, &page)

			// The block the recorded op refers to disappears behind history's back.
			require.NoError(t, page.Apply(DeleteBlock{Tombstone: page.Blocks[0], After: nil}))

			undoDepth := h.UndoDepth()
			redoDepth := h.RedoDepth()

			err := tc.act(h, &page)
			require.Error(t, err, tc.wantErrMsg)
			assert.Equal(t, undoDepth, h.UndoDepth(), tc.wantUndoMsg)
			assert.Equal(t, redoDepth, h.RedoDepth(), tc.wantRedoMsg)
		})
	}
}
