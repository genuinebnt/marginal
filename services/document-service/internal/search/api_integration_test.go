//go:build integration

package search_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	documentv1 "marginal/document-service/genproto/documentv1"
	"marginal/document-service/internal/pages"
	"marginal/document-service/internal/search"
)

// TestSearchEndToEndFindsATitleAndABlockHit exercises the full stack —
// real Postgres, search.PostgresRepo, and the proto translation in
// api.go — for both kinds of hit at once.
func TestSearchEndToEndFindsATitleAndABlockHit(t *testing.T) {
	pageRepo, searchRepo, pool := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	server, err := search.NewServer(ctx, searchRepo)
	require.NoError(t, err)

	titlePage, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Rollout plan"})
	require.NoError(t, err)
	blockPage, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Unrelated"})
	require.NoError(t, err)
	insertBlockWithText(t, pool, uuid.UUID(blockPage.ID), "the rollout plan ships next week")

	resp, err := server.Search(ctx, &documentv1.SearchRequest{Query: "rollout"})
	require.NoError(t, err)
	require.Len(t, resp.Hits, 2)

	var sawTitle, sawBlock bool
	for _, h := range resp.Hits {
		if h.PageId == titlePage.ID.String() {
			sawTitle = true
			assert.Nil(t, h.BlockId)
		}
		if h.PageId == blockPage.ID.String() {
			sawBlock = true
			require.NotNil(t, h.BlockId)
			require.NotNil(t, h.Snippet)
		}
	}
	assert.True(t, sawTitle, "the title hit must be present")
	assert.True(t, sawBlock, "the block hit must be present")
}

// TestSuggestTitlesEndToEndFindsATypo pins the BK-tree's real refresh
// cadence: NewServer builds the index synchronously (so a request right
// after startup already sees real data, no empty-index race).
func TestSuggestTitlesEndToEndFindsATypo(t *testing.T) {
	pageRepo, searchRepo, _ := newTestRepos(t)
	ctx := context.Background()
	owner := uuid.Must(uuid.NewV7())

	_, err := pageRepo.Create(ctx, pages.NewPage{CreatedBy: owner, Title: "Performance budget"})
	require.NoError(t, err)

	server, err := search.NewServer(ctx, searchRepo)
	require.NoError(t, err)

	resp, err := server.SuggestTitles(ctx, &documentv1.SuggestTitlesRequest{Query: "Performnace budget", MaxDistance: 2})
	require.NoError(t, err)
	require.Len(t, resp.Suggestions, 1)
	assert.Equal(t, "Performance budget", resp.Suggestions[0].Title)
	assert.Equal(t, int32(2), resp.Suggestions[0].Distance)
}

func TestSuggestTitlesEmptyQueryReturnsEmptyNotNil(t *testing.T) {
	_, searchRepo, _ := newTestRepos(t)
	ctx := context.Background()
	server, err := search.NewServer(ctx, searchRepo)
	require.NoError(t, err)

	resp, err := server.SuggestTitles(ctx, &documentv1.SuggestTitlesRequest{Query: "", MaxDistance: 2})
	require.NoError(t, err)
	assert.NotNil(t, resp.Suggestions)
	assert.Empty(t, resp.Suggestions)
}
