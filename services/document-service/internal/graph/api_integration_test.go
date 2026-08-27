//go:build integration

package graph_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/document-service/internal/graph"
	"marginal/document-service/internal/pages"
)

// TestAnalyzeGraphEndToEndTriangle exercises the full stack — real
// Postgres, graph.PostgresRepo.LoadGraph, graphalgo, and the
// proto translation in api.go — for a real 3-cycle of mutually-linked
// pages: graph-algorithms.html's own smallest case where a cycle exists,
// gets filled as one triangle, and its own loop disappears from
// b1_clique. Nothing here is a hand-built graphalgo.Graph — every page
// and link is a real row.
func TestAnalyzeGraphEndToEndTriangle(t *testing.T) {
	pageRepo, graphRepo, pool := newTestRepos(t)
	server := graph.NewServer(graphRepo)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	a, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "A"})
	require.NoError(t, err)
	b, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "B"})
	require.NoError(t, err)
	c, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "C"})
	require.NoError(t, err)

	aUUID, bUUID, cUUID := uuid.UUID(a.ID), uuid.UUID(b.ID), uuid.UUID(c.ID)
	insertLink(t, pool, aUUID, "B", &bUUID)
	insertLink(t, pool, bUUID, "C", &cUUID)
	insertLink(t, pool, cUUID, "A", &aUUID)

	analysis, err := server.AnalyzeGraph(ctx, nil)
	require.NoError(t, err)

	assert.Equal(t, analysis.ComponentOf[a.ID.String()], analysis.ComponentOf[b.ID.String()])
	assert.Equal(t, analysis.ComponentOf[a.ID.String()], analysis.ComponentOf[c.ID.String()])
	assert.Empty(t, analysis.OrphanComponents, "a, b, and c were all created with no parent, so each is itself a root — the component is not orphaned")
	require.NotEmpty(t, analysis.Cycle, "a -> b -> c -> a is a real cycle")
	assert.Equal(t, int32(1), analysis.Diameter)

	require.NotNil(t, analysis.Betti)
	assert.Equal(t, int32(1), analysis.Betti.B0)
	assert.Equal(t, int32(1), analysis.Betti.B1, "one independent loop in the plain 3-cycle")
	assert.Equal(t, int32(1), analysis.Betti.Triangles)
	assert.Equal(t, int32(0), analysis.Betti.B1Clique, "filling the one triangle kills its own loop")
	assert.Equal(t, int32(0), analysis.Betti.B2)
}
