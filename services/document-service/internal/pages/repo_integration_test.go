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

	got, err := repo.Get(ctx, owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, created.SortKey, got.SortKey)

	renamed, err := repo.Rename(ctx, owner, created.ID, "Architecture v2")
	require.NoError(t, err)
	require.Equal(t, "Architecture v2", renamed.Title)
	require.True(t, renamed.UpdatedAt.After(created.UpdatedAt) || renamed.UpdatedAt.Equal(created.UpdatedAt))

	require.NoError(t, repo.Delete(ctx, owner, created.ID))

	_, err = repo.Get(ctx, owner, created.ID)
	require.ErrorIs(t, err, pages.ErrNotFound, "a soft-deleted page must not be gettable — docs/api/pages.md doesn't distinguish deleted from nonexistent")

	require.NoError(t, repo.Delete(ctx, owner, created.ID), "delete must be idempotent")
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

	require.NoError(t, repo.Delete(ctx, owner, parentID))

	_, err = repo.Get(ctx, owner, parentID)
	require.ErrorIs(t, err, pages.ErrNotFound)
	_, err = repo.Get(ctx, owner, child.ID)
	require.ErrorIs(t, err, pages.ErrNotFound, "child must be deleted along with its parent")
	_, err = repo.Get(ctx, owner, grandchild.ID)
	require.ErrorIs(t, err, pages.ErrNotFound, "grandchild must be deleted too — the whole subtree")

	stillThere, err := repo.Get(ctx, owner, sibling.ID)
	require.NoError(t, err, "an unrelated page must not be touched by the cascade")
	require.Equal(t, pages.Active, stillThere.LifecycleState)

	require.NoError(t, repo.Delete(ctx, owner, parentID), "cascading delete is idempotent too")
}

// TestPagesAreScopedToTheirOwner is the security-review finding this
// covers: a page that exists but belongs to someone else must be
// indistinguishable, at every RPC, from one that doesn't exist at all
// (docs/api/pages.md, user story A-04 — "only I can read or change my
// pages," and refusal "does not reveal whether the page exists").
func TestPagesAreScopedToTheirOwner(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()
	other := testUUID()

	page, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Owner's page"})
	require.NoError(t, err)

	_, err = repo.Get(ctx, other, page.ID)
	require.ErrorIs(t, err, pages.ErrNotFound, "another actor's Get must not see it")

	_, err = repo.Rename(ctx, other, page.ID, "Hijacked")
	require.ErrorIs(t, err, pages.ErrNotFound, "another actor's Rename must not touch it")

	require.NoError(t, repo.Delete(ctx, other, page.ID), "Delete is idempotent even for a page that isn't the caller's — no error, but nothing happens")
	stillThere, err := repo.Get(ctx, owner, page.ID)
	require.NoError(t, err, "the real owner's page must survive another actor's Delete attempt")
	require.Equal(t, pages.Active, stillThere.LifecycleState)

	_, err = repo.Reparent(ctx, other, page.ID, pages.ParentChange{Change: true, ParentID: nil}, nil)
	require.ErrorIs(t, err, pages.ErrNotFound, "another actor's Reparent must not touch it")

	list, err := repo.List(ctx, other, nil, "", 10)
	require.NoError(t, err)
	require.Empty(t, list, "List must never surface another actor's pages")
}

// TestCreateCannotNestUnderAnotherActorsPage covers the same principle
// from the other direction: naming someone else's page as a new parent or
// "after" anchor must fail the same as naming a page that doesn't exist.
func TestCreateCannotNestUnderAnotherActorsPage(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	owner := testUUID()
	other := testUUID()

	theirPage, err := repo.Create(ctx, pages.NewPage{CreatedBy: other, Title: "Someone else's page"})
	require.NoError(t, err)
	theirPageID := theirPage.ID

	_, err = repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Should fail", ParentID: &theirPageID})
	require.Error(t, err)

	_, err = repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Should also fail", After: &theirPageID})
	require.Error(t, err)
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

	list, err := repo.List(ctx, owner, nil, "", 10)
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

	updated, err := repo.Reparent(ctx, owner, movedID, pages.ParentChange{Change: true, ParentID: &newParentID}, nil)
	require.NoError(t, err)
	require.NotNil(t, updated.ParentID)
	require.Equal(t, newParentID, *updated.ParentID)
	require.Contains(t, updated.Path, newParent.Path)
	require.NotContains(t, updated.Path, oldParent.Path)

	gotGrandchild, err := repo.Get(ctx, owner, grandchild.ID)
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
	updated, err := repo.Reparent(ctx, owner, child.ID, pages.ParentChange{Change: true, ParentID: nil}, nil)
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
	_, err = repo.Reparent(ctx, owner, a.ID, pages.ParentChange{}, &cID)
	require.NoError(t, err)

	list, err := repo.List(ctx, owner, nil, "", 10)
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
	_, err = repo.Reparent(ctx, owner, parent.ID, pages.ParentChange{Change: true, ParentID: &parent.ID}, nil)
	require.ErrorIs(t, err, pages.ErrCycle)

	// Under its own descendant.
	_, err = repo.Reparent(ctx, owner, parent.ID, pages.ParentChange{Change: true, ParentID: &childID}, nil)
	require.ErrorIs(t, err, pages.ErrCycle)
}
