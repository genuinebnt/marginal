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

	created, err := repo.Create(ctx, pages.NewPage{CreatedBy: testUUID(), Title: "Architecture"})
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
