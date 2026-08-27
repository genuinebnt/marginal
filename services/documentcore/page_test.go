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

func TestApplyInsertBlock(t *testing.T) {
	tests := []struct {
		name      string
		withFirst bool
		wantLen   int
		wantIndex int
	}{
		{name: "at start", withFirst: false, wantLen: 1, wantIndex: 0},
		{name: "after existing", withFirst: true, wantLen: 2, wantIndex: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page := NewPage(newTestPageID(), "Title")
			var after *BlockID
			if tc.withFirst {
				first := newTestBlockID()
				require.NoError(t, page.Apply(InsertBlock{ID: first, Kind: NewParagraph(), Content: PlainContent("")}))
				after = &first
			}

			id := newTestBlockID()
			err := page.Apply(InsertBlock{ID: id, After: after, Kind: NewParagraph(), Content: PlainContent("")})

			require.NoError(t, err)
			require.Len(t, page.Blocks, tc.wantLen)
			assert.Equal(t, id, page.Blocks[tc.wantIndex].ID)
		})
	}
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

// TestSetBlockContentPreconditionTreatsNilAndEmptyMarksAsEqual pins a
// real bug: the precondition check used to be reflect.DeepEqual, which
// treats a nil Marks slice and a non-nil empty one as unequal. A block
// inserted with a bare Content{Text: s} literal (nil Marks, as an
// unmarshaled JSON payload with no "marks" field would also produce) has
// to be a valid Prev for a SetBlockContent whose actual current content
// came from PlainContent (always a non-nil empty slice) — the two are
// the same content, zero marks, constructed two different legitimate
// ways.
func TestSetBlockContentPreconditionTreatsNilAndEmptyMarksAsEqual(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	id := newTestBlockID()
	// Content{Text: "hello"} — Marks is nil, not PlainContent's non-nil []Mark{}.
	require.NoError(t, page.Apply(InsertBlock{ID: id, Kind: NewParagraph(), Content: Content{Text: "hello"}}))

	// Prev names PlainContent's shape (non-nil empty Marks) — must still
	// match the block's actual (nil-Marks) current content.
	err := page.Apply(SetBlockContent{Block: id, Prev: PlainContent("hello"), Content: PlainContent("hello world")})
	require.NoError(t, err, "nil Marks and non-nil empty Marks must compare equal — same content, no marks, two legitimate constructions")
	assert.Equal(t, "hello world", page.Blocks[0].Content.Text)
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

// --- RFC-001 §1 containment: Quote/Toggle/List/ListItem nesting ---

func TestInsertBlockAsChildOfContainerPlacesItImmediatelyAfterParent(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))

	child := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: child, Parent: &quote, Kind: NewParagraph(), Content: PlainContent("inside")}))

	require.Len(t, page.Blocks, 2)
	assert.Equal(t, quote, page.Blocks[0].ID)
	assert.Equal(t, child, page.Blocks[1].ID)
	require.NotNil(t, page.Blocks[1].Parent)
	assert.Equal(t, quote, *page.Blocks[1].Parent)
}

// TestCalloutAndAsideAcceptAnyChildKind confirms RFC-001 §10's two new
// containers behave exactly like Quote/Toggle — any child kind is
// accepted, unlike ListItem's own restricted children.
func TestCalloutAndAsideAcceptAnyChildKind(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	callout := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: callout, Kind: NewCallout(Danger, "⚠️"), Content: PlainContent("")}))
	aside := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: aside, After: &callout, Kind: NewAside("🐢"), Content: PlainContent("")}))

	require.NoError(t, page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &callout, Kind: NewList(Bulleted), Content: PlainContent("")}))
	require.NoError(t, page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &aside, Kind: NewQuote(), Content: PlainContent("")}))

	require.Len(t, page.Blocks, 4)
}

func TestDeleteBlockRejectsNonEmptyCallout(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	callout := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: callout, Kind: NewCallout(Note, ""), Content: PlainContent("")}))
	require.NoError(t, page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &callout, Kind: NewParagraph(), Content: PlainContent("")}))

	err := page.Apply(DeleteBlock{Tombstone: page.Blocks[0], After: nil})

	var notEmpty *ContainerNotEmptyError
	assert.ErrorAs(t, err, &notEmpty)
}

func TestInsertBlockNilAfterInsertsAsParentsFirstChild(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))

	first := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: first, Parent: &quote, Kind: NewParagraph(), Content: PlainContent("")}))
	// A second child with After: nil must land BEFORE `first`, not after —
	// nil always means "the parent's first child," regardless of any
	// existing children (mirrors nil meaning "start of the page" at the
	// top level).
	second := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: second, Parent: &quote, Kind: NewParagraph(), Content: PlainContent("")}))

	require.Len(t, page.Blocks, 3)
	assert.Equal(t, []BlockID{quote, second, first}, []BlockID{page.Blocks[0].ID, page.Blocks[1].ID, page.Blocks[2].ID})
}

func TestInsertBlockAfterASiblingWithChildrenSkipsPastThem(t *testing.T) {
	// [Quote A (with child C), Quote B] — inserting after A must land after
	// C too, not between A and C, preserving depth-first order.
	page := NewPage(newTestPageID(), "Title")
	a := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: a, Kind: NewQuote(), Content: PlainContent("")}))
	c := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: c, Parent: &a, Kind: NewParagraph(), Content: PlainContent("")}))

	b := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: b, After: &a, Kind: NewQuote(), Content: PlainContent("")}))

	assert.Equal(t, []BlockID{a, c, b}, []BlockID{page.Blocks[0].ID, page.Blocks[1].ID, page.Blocks[2].ID})
}

func TestInsertBlockRejectsNonexistentParent(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	ghost := newTestBlockID()

	err := page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &ghost, Kind: NewParagraph(), Content: PlainContent("")})

	var notFound *BlockNotFoundError
	assert.ErrorAs(t, err, &notFound)
}

func TestInsertBlockRejectsNonContainerParent(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	paragraph := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: paragraph, Kind: NewParagraph(), Content: PlainContent("")}))

	err := page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &paragraph, Kind: NewParagraph(), Content: PlainContent("")})

	var notAContainer *NotAContainerError
	assert.ErrorAs(t, err, &notAContainer)
}

func TestInsertBlockUnderListItemRestrictsChildKind(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	item := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: item, Kind: NewListItem(false), Content: PlainContent("")}))

	// RFC-001 §1: ListChild ::= List | Paragraph — Quote is rejected...
	err := page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &item, Kind: NewQuote(), Content: PlainContent("")})
	var invalidChild *InvalidListChildError
	assert.ErrorAs(t, err, &invalidChild)

	// ...but a nested List and a continuation Paragraph are both allowed.
	assert.NoError(t, page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &item, Kind: NewList(Bulleted), Content: PlainContent("")}))
	assert.NoError(t, page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &item, Kind: NewParagraph(), Content: PlainContent("")}))
}

func TestDeleteBlockRejectsNonEmptyContainer(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))
	child := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: child, Parent: &quote, Kind: NewParagraph(), Content: PlainContent("")}))

	err := page.Apply(DeleteBlock{Tombstone: page.Blocks[0], After: nil})

	var notEmpty *ContainerNotEmptyError
	assert.ErrorAs(t, err, &notEmpty)
	assert.Len(t, page.Blocks, 2, "a rejected delete must not mutate the page")
}

func TestDeleteBlockAllowsEmptyContainer(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))

	err := page.Apply(DeleteBlock{Tombstone: page.Blocks[0], After: nil})

	require.NoError(t, err)
	assert.Empty(t, page.Blocks)
}

func TestDeleteBlockRejectsParentMismatch(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))
	child := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: child, Parent: &quote, Kind: NewParagraph(), Content: PlainContent("")}))

	// child's real parent is quote, not nil (top-level).
	err := page.Apply(DeleteBlock{Tombstone: Block{ID: child, Parent: nil, Kind: NewParagraph(), Content: PlainContent("")}, After: &quote})

	var mismatch *ParentMismatchError
	assert.ErrorAs(t, err, &mismatch)
}

func TestSetBlockKindRejectsConvertingNonEmptyContainerToLeaf(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))
	require.NoError(t, page.Apply(InsertBlock{ID: newTestBlockID(), Parent: &quote, Kind: NewParagraph(), Content: PlainContent("")}))

	err := page.Apply(SetBlockKind{ID: quote, From: NewQuote(), To: NewParagraph()})

	var notEmpty *ContainerNotEmptyError
	assert.ErrorAs(t, err, &notEmpty)
	assert.Equal(t, Quote, page.Blocks[0].Kind.Tag, "a rejected conversion must not mutate the block")
}

func TestSetBlockKindUnderListItemRestrictsChildKind(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	item := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: item, Kind: NewListItem(false), Content: PlainContent("")}))
	para := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: para, Parent: &item, Kind: NewParagraph(), Content: PlainContent("")}))

	err := page.Apply(SetBlockKind{ID: para, From: NewParagraph(), To: NewQuote()})

	var invalidChild *InvalidListChildError
	assert.ErrorAs(t, err, &invalidChild)
}

func TestMoveBlockRejectsCycleUnderOwnDescendant(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))
	child := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: child, Parent: &quote, Kind: NewQuote(), Content: PlainContent("")}))

	// Moving quote to become its own child's child.
	err := page.Apply(MoveBlock{ID: quote, FromParent: nil, From: nil, ToParent: &child, To: nil})

	var cycle *CycleError
	assert.ErrorAs(t, err, &cycle)
}

func TestMoveBlockRejectsMovingUnderItself(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))

	err := page.Apply(MoveBlock{ID: quote, FromParent: nil, From: nil, ToParent: &quote, To: nil})

	var cycle *CycleError
	assert.ErrorAs(t, err, &cycle)
}

func TestMoveBlockRelocatesWholeSubtreeAsOneUnit(t *testing.T) {
	// [Quote(children: A, B), Target] — moving Quote under Target must
	// bring A and B along, contiguous and in order, immediately after it.
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))
	a := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: a, Parent: &quote, Kind: NewParagraph(), Content: PlainContent("a")}))
	b := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: b, Parent: &quote, After: &a, Kind: NewParagraph(), Content: PlainContent("b")}))
	target := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: target, After: &quote, Kind: NewToggle(), Content: PlainContent("")}))

	err := page.Apply(MoveBlock{ID: quote, FromParent: nil, From: nil, ToParent: &target, To: nil})
	require.NoError(t, err)

	require.Len(t, page.Blocks, 4)
	assert.Equal(t, []BlockID{target, quote, a, b},
		[]BlockID{page.Blocks[0].ID, page.Blocks[1].ID, page.Blocks[2].ID, page.Blocks[3].ID})
	require.NotNil(t, page.Blocks[1].Parent)
	assert.Equal(t, target, *page.Blocks[1].Parent, "the subtree's own root is reparented")
	require.NotNil(t, page.Blocks[2].Parent)
	assert.Equal(t, quote, *page.Blocks[2].Parent, "descendants keep their existing parent")
}

func TestMoveBlockSubtreeThenInvertRestoresNestedOrder(t *testing.T) {
	page := NewPage(newTestPageID(), "Title")
	quote := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: quote, Kind: NewQuote(), Content: PlainContent("")}))
	child := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: child, Parent: &quote, Kind: NewParagraph(), Content: PlainContent("")}))
	target := newTestBlockID()
	require.NoError(t, page.Apply(InsertBlock{ID: target, After: &quote, Kind: NewToggle(), Content: PlainContent("")}))

	assertRoundTrips(t, &page, MoveBlock{ID: quote, FromParent: nil, From: nil, ToParent: &target, To: nil})
}
