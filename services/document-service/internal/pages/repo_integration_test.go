//go:build integration

package pages_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"marginal/document-service/internal/migrate"
	"marginal/document-service/internal/pages"
)

// Real Postgres via testcontainers-go, never a mock — the standing rule
// for anything touching infrastructure (.agents/agents.md). Requires
// Docker; run with: go test -tags=integration ./internal/pages/...
func newTestRepo(t *testing.T) *pages.PostgresRepo {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("document_service_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sqlOpen(connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, migrate.Up(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pages.NewPostgresRepo(pool)
}

func TestCreateGetRenameDeleteRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	created, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Architecture"})
	require.NoError(t, err)
	require.Equal(t, "Architecture", created.Title)
	require.Equal(t, pages.Active, created.LifecycleState)

	got, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, created.SortKey, got.SortKey)

	renamed, err := repo.Rename(ctx, created.ID, "Architecture v2")
	require.NoError(t, err)
	require.Equal(t, "Architecture v2", renamed.Title)
	require.True(t, renamed.UpdatedAt.After(created.UpdatedAt) || renamed.UpdatedAt.Equal(created.UpdatedAt))

	require.NoError(t, repo.Delete(ctx, created.ID))

	_, err = repo.Get(ctx, created.ID)
	require.ErrorIs(t, err, pages.ErrNotFound, "a soft-deleted page must not be gettable — docs/api/pages.md doesn't distinguish deleted from nonexistent")

	require.NoError(t, repo.Delete(ctx, created.ID), "delete must be idempotent")
}

func TestDeleteCascadesToDescendants(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	parent, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Parent"})
	require.NoError(t, err)
	parentID := parent.ID
	child, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Child", ParentID: &parentID})
	require.NoError(t, err)
	childID := child.ID
	grandchild, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Grandchild", ParentID: &childID})
	require.NoError(t, err)

	// An unrelated sibling must survive.
	sibling, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Sibling"})
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, parentID))

	_, err = repo.Get(ctx, parentID)
	require.ErrorIs(t, err, pages.ErrNotFound)
	_, err = repo.Get(ctx, child.ID)
	require.ErrorIs(t, err, pages.ErrNotFound, "child must be deleted along with its parent")
	_, err = repo.Get(ctx, grandchild.ID)
	require.ErrorIs(t, err, pages.ErrNotFound, "grandchild must be deleted too — the whole subtree")

	stillThere, err := repo.Get(ctx, sibling.ID)
	require.NoError(t, err, "an unrelated page must not be touched by the cascade")
	require.Equal(t, pages.Active, stillThere.LifecycleState)

	require.NoError(t, repo.Delete(ctx, parentID), "cascading delete is idempotent too")
}

// TestPagesAreVisibleToEveryActor covers the reversal of this repo's old
// owner-scoping model (2026-08-26, docs/porting/PROGRESS.md): a live,
// multi-user "shared workspace, not multi-tenant" instance needs a second
// person to actually see and edit the first person's pages — created_by is
// still recorded (who authored a page) but is no longer an access filter,
// on any of Get/Rename/Delete/Reparent/List.
func TestPagesAreVisibleToEveryActor(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	page, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Owner's page"})
	require.NoError(t, err)

	got, err := repo.Get(ctx, page.ID)
	require.NoError(t, err, "any actor's Get must see it")
	require.Equal(t, owner, got.CreatedBy, "created_by is still recorded even though it's no longer a filter")

	renamed, err := repo.Rename(ctx, page.ID, "Renamed by someone else")
	require.NoError(t, err, "any actor's Rename must be able to touch it")
	require.Equal(t, "Renamed by someone else", renamed.Title)

	moved, err := repo.Reparent(ctx, page.ID, pages.ParentChange{Change: true, ParentID: nil}, nil)
	require.NoError(t, err, "any actor's Reparent must be able to touch it")
	require.Nil(t, moved.ParentID)

	list, err := repo.List(ctx, nil, "", 10)
	require.NoError(t, err)
	require.Len(t, list, 1, "List must surface every page on the instance, not just the caller's own")

	require.NoError(t, repo.Delete(ctx, page.ID), "any actor's Delete must be able to touch it")
	_, err = repo.Get(ctx, page.ID)
	require.ErrorIs(t, err, pages.ErrNotFound)
}

// TestCreateCanNestUnderAnyExistingPage covers the same principle from the
// other direction: naming another actor's page as a new parent or "after"
// anchor succeeds exactly like naming one's own — existence is the only
// requirement, per Create's own doc comment.
func TestCreateCanNestUnderAnyExistingPage(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()
	other := testUUID()

	theirPage, err := repo.Create(ctx, pages.NewPage{CreatedBy: other, Title: "Someone else's page"})
	require.NoError(t, err)
	theirPageID := theirPage.ID

	child, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Nested under someone else", ParentID: &theirPageID})
	require.NoError(t, err)
	require.NotNil(t, child.ParentID)
	require.Equal(t, theirPageID, *child.ParentID)

	sibling, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "After someone else's page", After: &theirPageID})
	require.NoError(t, err)
	require.Nil(t, sibling.ParentID)
}

func TestCreateOrdersSiblingsBySortKey(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	first, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "First"})
	require.NoError(t, err)
	second, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Second"})
	require.NoError(t, err)

	// Insert between them.
	afterFirst := first.ID
	middle, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Middle", After: &afterFirst})
	require.NoError(t, err)

	list, err := repo.List(ctx, nil, "", 10)
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.Equal(t, first.ID, list[0].ID)
	require.Equal(t, middle.ID, list[1].ID)
	require.Equal(t, second.ID, list[2].ID)
}

func TestCreateChildComputesNestedPath(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	parent, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Parent"})
	require.NoError(t, err)

	parentID := parent.ID
	child, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Child", ParentID: &parentID})
	require.NoError(t, err)

	require.Contains(t, child.Path, parent.Path)
	require.NotEqual(t, parent.Path, child.Path)
	require.NotNil(t, child.ParentID)
	require.Equal(t, parent.ID, *child.ParentID)
}

func TestCreateAfterAnchorUnderWrongParentFails(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	root, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Root sibling"})
	require.NoError(t, err)

	otherParent, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Other parent"})
	require.NoError(t, err)
	otherParentID := otherParent.ID

	rootID := root.ID
	_, err = repo.Create(ctx, pages.NewPage{
		CreatedBy: owner,
		Title:     "Should fail",
		ParentID:  &otherParentID,
		After:     &rootID, // root is not a child of otherParent
	})
	require.ErrorIs(t, err, pages.ErrAnchorMismatch)
}

func TestReparentMovesToNewParentAndRewritesDescendantPaths(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	oldParent, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Old parent"})
	require.NoError(t, err)
	oldParentID := oldParent.ID
	newParent, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "New parent"})
	require.NoError(t, err)
	newParentID := newParent.ID

	moved, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Moved", ParentID: &oldParentID})
	require.NoError(t, err)
	movedID := moved.ID
	grandchild, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Grandchild", ParentID: &movedID})
	require.NoError(t, err)

	updated, err := repo.Reparent(ctx, movedID, pages.ParentChange{Change: true, ParentID: &newParentID}, nil)
	require.NoError(t, err)
	require.NotNil(t, updated.ParentID)
	require.Equal(t, newParentID, *updated.ParentID)
	require.Contains(t, updated.Path, newParent.Path)
	require.NotContains(t, updated.Path, oldParent.Path)

	gotGrandchild, err := repo.Get(ctx, grandchild.ID)
	require.NoError(t, err)
	require.Contains(t, gotGrandchild.Path, updated.Path, "descendant's path must follow the moved subtree, in the same transaction")
	require.NotContains(t, gotGrandchild.Path, oldParent.Path)
}

func TestReparentPromoteToRoot(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	parent, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Parent"})
	require.NoError(t, err)
	parentID := parent.ID
	child, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Child", ParentID: &parentID})
	require.NoError(t, err)

	// parent_id present and empty: promote to root.
	updated, err := repo.Reparent(ctx, child.ID, pages.ParentChange{Change: true, ParentID: nil}, nil)
	require.NoError(t, err)
	require.Nil(t, updated.ParentID)
	require.NotContains(t, updated.Path, parent.Path)
}

func TestReparentReordersWithinSameParent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	a, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "A"})
	require.NoError(t, err)
	b, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "B"})
	require.NoError(t, err)
	c, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "C"})
	require.NoError(t, err)

	// Move A to be after C: leave parent alone (root, Change: false), reorder only.
	cID := c.ID
	_, err = repo.Reparent(ctx, a.ID, pages.ParentChange{}, &cID)
	require.NoError(t, err)

	list, err := repo.List(ctx, nil, "", 10)
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.Equal(t, b.ID, list[0].ID)
	require.Equal(t, c.ID, list[1].ID)
	require.Equal(t, a.ID, list[2].ID)
}

func TestReparentUnderSelfOrDescendantFails(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()

	parent, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Parent"})
	require.NoError(t, err)
	parentID := parent.ID
	child, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Child", ParentID: &parentID})
	require.NoError(t, err)
	childID := child.ID

	// Under itself.
	_, err = repo.Reparent(ctx, parent.ID, pages.ParentChange{Change: true, ParentID: &parent.ID}, nil)
	require.ErrorIs(t, err, pages.ErrCycle)

	// Under its own descendant.
	_, err = repo.Reparent(ctx, parent.ID, pages.ParentChange{Change: true, ParentID: &childID}, nil)
	require.ErrorIs(t, err, pages.ErrCycle)
}
