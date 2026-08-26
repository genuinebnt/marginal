package documentcore

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPageID() PageID   { return PageID(uuid.New()) }
func newTestBlockID() BlockID { return BlockID(uuid.New()) }

func testParagraph(id BlockID, text string) Block {
	return Block{ID: id, Kind: NewParagraph(), Content: PlainContent(text)}
}

func TestApplyInsertBlockAtStart(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	id := newTestBlockID()

	err := page.Apply(InsertBlock{ID: id, After: nil, Kind: NewParagraph(), Content: PlainContent("")})

	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	assert.Equal(t, id, page.Blocks[0].ID)
}

func TestApplyInsertBlockAfterExisting(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	first := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: first, Kind: NewParagraph(), Content: PlainContent("")}))

	second := newTestBlockID()
	err := page.Apply(InsertBlock{ID: second, After: &first, Kind: NewParagraph(), Content: PlainContent("")})

	require.NoError(t, err)
	require.Len(t, page.Blocks, 2)
	assert.Equal(t, second, page.Blocks[1].ID)
}

func TestApplyInsertBlockRejectsDuplicateID(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	id := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: id, Kind: NewParagraph(), Content: PlainContent("")}))

	err := page.Apply(InsertBlock{ID: id, Kind: NewParagraph(), Content: PlainContent("")})

	var dup *DuplicateBlockIDError
	assert.ErrorAs(t, err, &dup)
}

func TestApplyDeleteBlockRemovesIt(t *testing.T) {
	ids := make([]BlockID, 3)
	page := NewPage(newTestPageID(), "Title")
	var after *BlockID
	for i := range ids {
		ids[i] = newTestBlockID()
		require.NoError(t, page.Apply(InsertBlock{ID: ids[i], After: after, Kind: NewParagraph(), Content: PlainContent("")}))
		id := ids[i]
		after = &id
	}

	err := page.Apply(DeleteBlock{Tombstone: page.Blocks[1], After: &ids[0]})

	require.NoError(t, err)
	require.Len(t, page.Blocks, 2)
	assert.Equal(t, ids[0], page.Blocks[0].ID)
	assert.Equal(t, ids[2], page.Blocks[1].ID)
}

func TestApplyDeleteBlockReturnsErrorIfNotFound(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	err := page.Apply(DeleteBlock{Tombstone: testParagraph(newTestBlockID(), "")})

	var notFound *BlockNotFoundError
	assert.ErrorAs(t, err, &notFound)
}

func TestApplyDeleteBlockRejectsWrongAfter(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	first := newTestBlockID()
	second := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: first, Kind: NewParagraph(), Content: PlainContent("")}))
	require.NoError(t, page.Apply(InsertBlock{ID: second, After: &first, Kind: NewParagraph(), Content: PlainContent("")}))

	// second's real predecessor is first, not nil — a stale/wrong After must
	// be rejected rather than silently deleting from the wrong recorded
	// position (rust/TASKS.md open decision #4, fixed uniformly here).
	err := page.Apply(DeleteBlock{Tombstone: page.Blocks[1], After: nil})

	var mismatch *PositionMismatchError
	assert.ErrorAs(t, err, &mismatch)
	assert.Len(t, page.Blocks, 2, "a rejected delete must not mutate the page")
}

func TestApplySetBlockKindRejectsStaleFrom(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	id := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: id, Kind: NewParagraph(), Content: PlainContent("")}))

	heading, err := NewHeading(2)
	require.NoError(t, err)
	err = page.Apply(SetBlockKind{ID: id, From: heading, To: NewQuote()})

	var pre *PreconditionError
	assert.ErrorAs(t, err, &pre)
	assert.Equal(t, Paragraph, page.Blocks[0].Kind.Tag, "a rejected op must not mutate the block")
}

func TestApplySetTitleRejectsStaleFrom(t *testing.T) {
	page := NewPage(newTestPageID(), "Original")

	err := page.Apply(SetTitle{Page: page.ID, From: "wrong", To: "New"})

	var pre *PreconditionError
	assert.ErrorAs(t, err, &pre)
	assert.Equal(t, "Original", page.Title)
}

// requirePageDeepEqual compares Blocks length+elementwise rather than via
// require.Equal on the whole slice — Apply's append-based mutations
// legitimately produce a non-nil empty slice where the page started with a
// nil one, and that's not a behavioral difference worth failing a test over.
func requirePageDeepEqual(t *testing.T, want, got Page) {
	t.Helper()
	require.Equal(t, want.ID, got.ID)
	require.Equal(t, want.Title, got.Title)
	require.Len(t, got.Blocks, len(want.Blocks))
	for i := range want.Blocks {
		require.Equal(t, want.Blocks[i], got.Blocks[i], "block %d", i)
	}
}

// assertRoundTrips applies op, checks the page actually changed, then
// applies its inverse and checks the page is restored exactly — RFC-002
// §3's invertibility law, pinned per-scenario here; property_test.go checks
// it generatively.
func assertRoundTrips(t *testing.T, page *Page, op Op) {
	t.Helper()
	before := *page
	before.Blocks = append([]Block(nil), page.Blocks...)

	require.NoError(t, page.Apply(op))
	require.False(t, pagesEqual(before, *page), "op left the page unchanged")

	require.NoError(t, page.Apply(op.Invert()))
	requirePageDeepEqual(t, before, *page)
}

// pagesEqual is a deep comparison — Block embeds Content, which holds a
// []Mark, so Page and Block aren't comparable with == or !=. Nil and
// non-nil-empty Blocks compare equal (see requirePageDeepEqual).
func pagesEqual(a, b Page) bool {
	if a.ID != b.ID || a.Title != b.Title || len(a.Blocks) != len(b.Blocks) {
		return false
	}
	for i := range a.Blocks {
		if !reflect.DeepEqual(a.Blocks[i], b.Blocks[i]) {
			return false
		}
	}
	return true
}

func TestInsertThenInvertRestoresEmptyPage(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	id := newTestBlockID()

	assertRoundTrips(t, &page, InsertBlock{ID: id, Kind: NewParagraph(), Content: PlainContent("")})

	assert.Empty(t, page.Blocks)
}

func TestSetBlockContentThenInvertRestoresOldContent(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	id := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: id, Kind: NewParagraph(), Content: PlainContent("")}))

	newContent := PlainContent("hello")
	require.NoError(t, newContent.AddMark(NewBold(), 0, 5))

	assertRoundTrips(t, &page, SetBlockContent{Block: id, Prev: PlainContent(""), Content: newContent})
}

func TestSetTitleThenInvertRestoresOldTitle(t *testing.T) {
	page := NewPage(newTestPageID(), "Original")

	assertRoundTrips(t, &page, SetTitle{Page: page.ID, From: "Original", To: "New"})
}

func TestMoveBlockThenInvertRestoresOrder(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	ids := make([]BlockID, 3)
	var after *BlockID
	for i := range ids {
		ids[i] = newTestBlockID()
		require.NoError(t, page.Apply(InsertBlock{ID: ids[i], After: after, Kind: NewParagraph(), Content: PlainContent("")}))
		id := ids[i]
		after = &id
	}

	assertRoundTrips(t, &page, MoveBlock{ID: ids[0], From: nil, To: &ids[2]})
}
