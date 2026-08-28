//go:build integration

package graph_test

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

	"marginal/document-service/internal/graph"
	"marginal/document-service/internal/migrate"
	"marginal/document-service/internal/pages"
	"marginal/graphalgo"
)

// Real Postgres via testcontainers-go, never a mock — the standing rule
// for anything touching infrastructure (.agents/agents.md). Requires
// Docker; run with: go test -tags=integration ./internal/graph/...
func newTestRepos(t *testing.T) (*pages.PostgresRepo, *graph.PostgresRepo, *pgxpool.Pool) {
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

	return pages.NewPostgresRepo(pool), graph.NewPostgresRepo(pool), pool
}

// insertLink writes a docs.page_links row directly (via a throwaway
// docs.blocks row to satisfy page_links.from_block's own foreign key) —
// this test is about graph.PostgresRepo.LoadGraph's own query
// correctness, not blockproj's link-resolution pipeline, so it skips
// straight to the row shape blockproj itself would have produced.
func insertLink(t *testing.T, pool *pgxpool.Pool, fromPage uuid.UUID, targetTitle string, targetPage *uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	blockID := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(ctx,
		`INSERT INTO docs.blocks (id, page_id, position, kind, content) VALUES ($1, $2, 0, '{"tag":"paragraph"}', '{}')`,
		blockID, fromPage)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO docs.page_links (id, from_page, from_block, target_title, target_page) VALUES ($1, $2, $3, $4, $5)`,
		uuid.Must(uuid.NewV7()), fromPage, blockID, targetTitle, targetPage)
	require.NoError(t, err)
}

func TestLoadGraphBuildsNodesRootsAndEdges(t *testing.T) {
	pageRepo, graphRepo, pool := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	root, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Root"})
	require.NoError(t, err)
	child, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Child", ParentID: &root.ID})
	require.NoError(t, err)
	lonely, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Lonely"})
	require.NoError(t, err)

	rootUUID, childUUID := uuid.UUID(root.ID), uuid.UUID(child.ID)
	insertLink(t, pool, rootUUID, "Child", &childUUID)
	insertLink(t, pool, childUUID, "Nonexistent Page", nil) // dangling — must not become an edge

	g, err := graphRepo.LoadGraph(ctx)
	require.NoError(t, err)

	rootID := graphalgo.NodeID(root.ID.String())
	childID := graphalgo.NodeID(child.ID.String())
	lonelyID := graphalgo.NodeID(lonely.ID.String())

	assert.ElementsMatch(t, []graphalgo.NodeID{rootID, childID, lonelyID}, g.Graph.Nodes)
	assert.Equal(t, []graphalgo.Edge{{From: rootID, To: childID}}, g.Graph.Edges, "the dangling link must not appear as an edge")

	assert.True(t, g.Nodes[rootID].IsRoot)
	assert.True(t, g.Nodes[lonelyID].IsRoot, "a page with no parent is a root even with no links at all")
	assert.False(t, g.Nodes[childID].IsRoot)
	assert.ElementsMatch(t, []graphalgo.NodeID{rootID, lonelyID}, g.Roots)

	assert.Equal(t, "Child", g.Nodes[childID].Title)
}

func TestLoadGraphExcludesDeletedPages(t *testing.T) {
	pageRepo, graphRepo, _ := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	kept, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Kept"})
	require.NoError(t, err)
	deleted, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Deleted"})
	require.NoError(t, err)
	require.NoError(t, pageRepo.Delete(ctx, deleted.ID))

	g, err := graphRepo.LoadGraph(ctx)
	require.NoError(t, err)

	assert.Contains(t, g.Graph.Nodes, graphalgo.NodeID(kept.ID.String()))
	assert.NotContains(t, g.Graph.Nodes, graphalgo.NodeID(deleted.ID.String()))
}
