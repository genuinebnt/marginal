//go:build integration

package search_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"marginal/document-service/internal/migrate"
	"marginal/document-service/internal/pages"
	"marginal/document-service/internal/search"
)

// Real Postgres via testcontainers-go, never a mock — the standing rule
// for anything touching infrastructure (.agents/agents.md). Requires
// Docker; run with: go test -tags=integration ./internal/search/...
func newTestRepos(t *testing.T) (*pages.PostgresRepo, *search.PostgresRepo, *pgxpool.Pool) {
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

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, migrate.Up(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pages.NewPostgresRepo(pool), search.NewPostgresRepo(pool), pool
}

// insertBlockWithText writes a docs.blocks row directly with real text
// content — this test is about search.PostgresRepo's own query
// correctness against docs.blocks' GENERATED ALWAYS tsvector column
// (migration 00004), not blockproj's own write path, so it skips
// straight to the row shape blockproj itself would have produced.
func insertBlockWithText(t *testing.T, pool *pgxpool.Pool, pageID uuid.UUID, text string) uuid.UUID {
	t.Helper()
	blockID := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(context.Background(),
		`INSERT INTO docs.blocks (id, page_id, position, kind, content) VALUES ($1, $2, 0, '{"tag":"paragraph"}', jsonb_build_object('text', $3::text))`,
		blockID, pageID, text)
	require.NoError(t, err)
	return blockID
}

func TestSearchFullTextFindsATitleMatch(t *testing.T) {
	pageRepo, searchRepo, _ := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	page, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Performance budget"})
	require.NoError(t, err)

	hits, err := searchRepo.SearchFullText(ctx, "performance")
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, uuid.UUID(page.ID), hits[0].PageID)
	assert.Nil(t, hits[0].BlockID, "a title-only hit carries no block")
}

func TestSearchFullTextFindsABlockMatchWithASnippet(t *testing.T) {
	pageRepo, searchRepo, pool := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	page, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Architecture notes"})
	require.NoError(t, err)
	blockID := insertBlockWithText(t, pool, uuid.UUID(page.ID), "We hold sync acknowledgement under a strict budget.")

	hits, err := searchRepo.SearchFullText(ctx, "acknowledgement")
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, uuid.UUID(page.ID), hits[0].PageID)
	require.NotNil(t, hits[0].BlockID)
	assert.Equal(t, blockID, *hits[0].BlockID)
	require.NotNil(t, hits[0].Snippet)
	assert.Contains(t, *hits[0].Snippet, "<b>acknowledgement</b>", "ts_headline must wrap the matched term")
}

func TestSearchFullTextRanksBothKindsOfHitTogether(t *testing.T) {
	pageRepo, searchRepo, pool := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	titleHit, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "budget review"})
	require.NoError(t, err)
	blockHit, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Unrelated"})
	require.NoError(t, err)
	insertBlockWithText(t, pool, uuid.UUID(blockHit.ID), "the budget review happens quarterly")

	hits, err := searchRepo.SearchFullText(ctx, "budget review")
	require.NoError(t, err)
	require.Len(t, hits, 2, "both the title match and the block match must be present")
	pageIDs := []uuid.UUID{hits[0].PageID, hits[1].PageID}
	assert.Contains(t, pageIDs, uuid.UUID(titleHit.ID))
	assert.Contains(t, pageIDs, uuid.UUID(blockHit.ID))
	assert.GreaterOrEqual(t, hits[0].Rank, hits[1].Rank, "must be sorted rank descending")
}

func TestSearchFullTextIgnoresDeletedPages(t *testing.T) {
	pageRepo, searchRepo, _ := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	page, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Soon deleted"})
	require.NoError(t, err)
	require.NoError(t, pageRepo.Delete(ctx, page.ID))

	hits, err := searchRepo.SearchFullText(ctx, "deleted")
	require.NoError(t, err)
	assert.Empty(t, hits)
}

func TestListPageTitlesForIndexOnlyListsLivePages(t *testing.T) {
	pageRepo, searchRepo, _ := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	live, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Live page"})
	require.NoError(t, err)
	deleted, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Dead page"})
	require.NoError(t, err)
	require.NoError(t, pageRepo.Delete(ctx, deleted.ID))

	titles, err := searchRepo.ListPageTitles(ctx)
	require.NoError(t, err)
	var ids []uuid.UUID
	for _, pt := range titles {
		ids = append(ids, uuid.UUID(pt.ID))
	}
	assert.Contains(t, ids, uuid.UUID(live.ID))
	assert.NotContains(t, ids, uuid.UUID(deleted.ID))
}
