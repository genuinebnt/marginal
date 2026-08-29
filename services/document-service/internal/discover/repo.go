// Package discover is § 09 DISCOVER's translation layer: read every live
// page's prose and classification, build a semantic index over it
// (marginal/semantic — hashed TF-IDF vectors, HNSW), and answer "what is near
// this page" with the three signals kept separate.
//
// No algorithm lives here. The vectors and the index are marginal/semantic's;
// the hop distances are marginal/graphalgo's. This package does I/O, assembly
// and wire translation — the same split internal/graph already draws.
//
// WHY THE INDEX IS REBUILT PER REQUEST, and what would replace it. The corpus
// is tens of pages. Building the index costs a single table scan and a few
// hundred distance computations, which is well under the latency budget of
// the screen it serves, and it means the answer is always over the current
// document rather than over whatever was true when a background job last ran.
// The moment that stops being true — a corpus where the scan dominates —
// the replacement is not a cache but a persisted index: an `embedding` column
// written by internal/blockproj as it materialises blocks, and the HNSW graph
// itself stored beside it. That is v4.4.0's work (RELEASES.md), and it is
// deliberately not being half-built here.
package discover

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"marginal/document-service/internal/discoverrepo/gen"
)

// Page is one row of the corpus.
type Page struct {
	ID            uuid.UUID
	Title         string
	Body          string
	Tags          []string
	TopicName     string
	TopicColorKey string
}

// PostgresRepo is this package's only port onto docs.
type PostgresRepo struct {
	q *discoverrepo.Queries
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{q: discoverrepo.New(pool)}
}

func (r *PostgresRepo) LoadCorpus(ctx context.Context) ([]Page, error) {
	rows, err := r.q.ListPagesForDiscover(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover: loading corpus: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		p := Page{
			ID:    row.ID.Bytes,
			Title: row.Title,
			Body:  row.Body,
			Tags:  row.Tags,
		}
		if row.TopicName != nil {
			p.TopicName = *row.TopicName
		}
		if row.TopicColorKey != nil {
			p.TopicColorKey = *row.TopicColorKey
		}
		out[i] = p
	}
	return out, nil
}
