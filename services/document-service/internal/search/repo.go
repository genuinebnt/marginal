package search

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"marginal/document-service/internal/searchrepo/gen"
)

// Hit is one full-text search result — a title match (BlockID/Snippet
// nil) or a block-text match (both set) — SearchFullText's own merged,
// re-ranked view over SearchPageTitles/SearchBlockText's two separate
// queries.
type Hit struct {
	PageID    uuid.UUID
	PageTitle string
	BlockID   *uuid.UUID
	Snippet   *string
	Rank      float32
}

// PostgresRepo is SearchService's only port onto docs.pages/docs.blocks.
type PostgresRepo struct {
	q *searchrepo.Queries
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{q: searchrepo.New(pool)}
}

// SearchFullText runs both SearchPageTitles and SearchBlockText and
// merges them by rank, highest first — two queries rather than one
// UNION so each side keeps its own simple, sqlc-inferred shape (a title
// hit has no snippet to compute; a block hit always does) instead of
// forcing one shared row shape with columns the other side leaves null.
func (r *PostgresRepo) SearchFullText(ctx context.Context, query string, spaceIDs []uuid.UUID) ([]Hit, error) {
	scope := make([]pgtype.UUID, len(spaceIDs))
	for i, id := range spaceIDs {
		scope[i] = pgtype.UUID{Bytes: id, Valid: true}
	}
	titleRows, err := r.q.SearchPageTitles(ctx, searchrepo.SearchPageTitlesParams{
		Query: query, SpaceIds: scope,
	})
	if err != nil {
		return nil, fmt.Errorf("search: searching page titles: %w", err)
	}
	blockRows, err := r.q.SearchBlockText(ctx, searchrepo.SearchBlockTextParams{
		Query: query, SpaceIds: scope,
	})
	if err != nil {
		return nil, fmt.Errorf("search: searching block text: %w", err)
	}

	hits := make([]Hit, 0, len(titleRows)+len(blockRows))
	for _, row := range titleRows {
		hits = append(hits, Hit{PageID: uuid.UUID(row.ID.Bytes), PageTitle: row.Title, Rank: row.Rank})
	}
	for _, row := range blockRows {
		blockID := uuid.UUID(row.BlockID.Bytes)
		snippet := string(row.Snippet)
		hits = append(hits, Hit{
			PageID: uuid.UUID(row.PageID.Bytes), PageTitle: row.PageTitle,
			BlockID: &blockID, Snippet: &snippet, Rank: row.Rank,
		})
	}

	// insertion sort: at most 40 rows total (20 + 20, both queries' own
	// LIMIT), not worth pulling in sort.Slice's reflection overhead for.
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Rank > hits[j-1].Rank; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	return hits, nil
}

// ListPageTitles reads every live page's id/title — TitleIndex's own
// refresh cadence calls this, never a per-request path.
func (r *PostgresRepo) ListPageTitles(ctx context.Context) ([]PageTitle, error) {
	rows, err := r.q.ListPageTitlesForIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("search: listing page titles: %w", err)
	}
	titles := make([]PageTitle, len(rows))
	for i, row := range rows {
		titles[i] = PageTitle{ID: uuid.UUID(row.ID.Bytes), Title: row.Title}
	}
	return titles, nil
}
