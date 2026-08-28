//go:build integration

package pages_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"marginal/document-service/internal/migrate"
	"marginal/document-service/internal/pages"
)

// newTestRepoWithPool is newTestRepo plus direct pool access — ListBlocks'
// own tests need to seed a docs.blocks row directly (the same "skip
// straight to the row shape blockproj itself would have produced"
// approach internal/graph's own repo_integration_test.go already uses),
// not exercised by any Create/Rename/Delete round trip above.
func newTestRepoWithPool(t *testing.T) (*pages.PostgresRepo, *pgxpool.Pool) {
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

	return pages.NewPostgresRepo(pool), pool
}

func TestListBlocksReturnsKindAndContentJSON(t *testing.T) {
	repo, pool := newTestRepoWithPool(t)
	ctx := context.Background()
	owner := testUUID()

	page, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Definitions"})
	require.NoError(t, err)

	blockID := uuid.Must(uuid.NewV7())
	_, err = pool.Exec(ctx,
		`INSERT INTO docs.blocks (id, page_id, position, kind, content, path) VALUES ($1, $2, 0, '{"tag":"paragraph"}', '{"text":"{{define ack-budget = 40ms}}"}', 'b1'::ltree)`,
		blockID, uuid.UUID(page.ID))
	require.NoError(t, err)

	blocks, err := repo.ListBlocks(ctx, page.ID)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, blockID, uuid.UUID(blocks[0].ID))
	assert.Nil(t, blocks[0].ParentID)
	assert.JSONEq(t, `{"tag":"paragraph"}`, string(blocks[0].KindJSON))
	assert.JSONEq(t, `{"text":"{{define ack-budget = 40ms}}"}`, string(blocks[0].ContentJSON))
}

func TestListBlocksReportsParentID(t *testing.T) {
	repo, pool := newTestRepoWithPool(t)
	ctx := context.Background()
	owner := testUUID()

	page, err := repo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Nested"})
	require.NoError(t, err)

	parentBlock := uuid.Must(uuid.NewV7())
	_, err = pool.Exec(ctx,
		`INSERT INTO docs.blocks (id, page_id, position, kind, content, path) VALUES ($1, $2, 0, '{"tag":"quote"}', '{}', 'p1'::ltree)`,
		parentBlock, uuid.UUID(page.ID))
	require.NoError(t, err)

	childBlock := uuid.Must(uuid.NewV7())
	_, err = pool.Exec(ctx,
		`INSERT INTO docs.blocks (id, page_id, parent_id, position, kind, content, path) VALUES ($1, $2, $3, 0, '{"tag":"paragraph"}', '{}', 'p1.c1'::ltree)`,
		childBlock, uuid.UUID(page.ID), parentBlock)
	require.NoError(t, err)

	blocks, err := repo.ListBlocks(ctx, page.ID)
	require.NoError(t, err)
	byID := make(map[uuid.UUID]pages.Block, len(blocks))
	for _, b := range blocks {
		byID[uuid.UUID(b.ID)] = b
	}
	require.NotNil(t, byID[childBlock].ParentID)
	assert.Equal(t, parentBlock, uuid.UUID(*byID[childBlock].ParentID))
}
