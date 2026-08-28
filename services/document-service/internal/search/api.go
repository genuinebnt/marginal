package search

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	documentv1 "marginal/document-service/genproto/documentv1"
)

// DefaultRefreshInterval is how often Server rebuilds its in-memory
// title index from Postgres — search.html's own admitted gap made
// concrete: a page renamed (or created) up to this long ago may not
// show up in a "did you mean" suggestion yet, even though full-text
// search (never index-lagged; its tsvector column commits with the row)
// already sees it.
const DefaultRefreshInterval = 30 * time.Second

// Server implements documentv1.SearchServiceServer. Search hits Postgres
// directly, every request (transactionally fresh); SuggestTitles reads
// an in-memory TitleIndex rebuilt on its own cadence in the background —
// the same "index has its own rebuild cadence" ADR-001 already accepts
// for anything that isn't the database of record.
type Server struct {
	documentv1.UnimplementedSearchServiceServer
	repo *PostgresRepo

	idx atomic.Pointer[TitleIndex]

	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// NewServer builds the initial title index synchronously (so the first
// request after startup already has real suggestions, not an empty
// index) and returns a Server; call Start to begin the background
// refresh loop.
func NewServer(ctx context.Context, repo *PostgresRepo) (*Server, error) {
	s := &Server{repo: repo}
	if err := s.refresh(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Start launches the periodic title-index refresh — mirrors
// collaboration-service/internal/flush.Loop's own Start/Stop shape.
// ctx bounds the loop's lifetime beyond an explicit Stop.
func (s *Server) Start(ctx context.Context, interval time.Duration) {
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go s.run(runCtx, interval)
}

func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}

func (s *Server) run(ctx context.Context, interval time.Duration) {
	defer s.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refresh(ctx); err != nil {
				slog.Error("search: refreshing title index failed", "err", err)
			}
		}
	}
}

func (s *Server) refresh(ctx context.Context) error {
	pages, err := s.repo.ListPageTitles(ctx)
	if err != nil {
		return err
	}
	s.idx.Store(BuildTitleIndex(pages))
	return nil
}

func (s *Server) Search(ctx context.Context, req *documentv1.SearchRequest) (*documentv1.SearchResponse, error) {
	if req.GetQuery() == "" {
		return &documentv1.SearchResponse{Hits: []*documentv1.SearchHit{}}, nil
	}

	hits, err := s.repo.SearchFullText(ctx, req.GetQuery())
	if err != nil {
		slog.Error("search: full-text search failed", "err", err)
		return nil, status.Error(codes.Internal, "search: search failed")
	}

	out := make([]*documentv1.SearchHit, len(hits))
	for i, h := range hits {
		hit := &documentv1.SearchHit{PageId: h.PageID.String(), PageTitle: h.PageTitle, Rank: h.Rank}
		if h.BlockID != nil {
			blockID := h.BlockID.String()
			hit.BlockId = &blockID
		}
		if h.Snippet != nil {
			hit.Snippet = h.Snippet
		}
		out[i] = hit
	}
	return &documentv1.SearchResponse{Hits: out}, nil
}

func (s *Server) SuggestTitles(_ context.Context, req *documentv1.SuggestTitlesRequest) (*documentv1.SuggestTitlesResponse, error) {
	if req.GetQuery() == "" {
		return &documentv1.SuggestTitlesResponse{Suggestions: []*documentv1.TitleSuggestion{}}, nil
	}
	maxDistance := int(req.GetMaxDistance())
	if maxDistance <= 0 {
		maxDistance = 2 // ROADMAP.md's own "construct it for k <= 2" — the same practical bound the Levenshtein-automaton alternative names, kept here as this RPC's default rather than trusting every caller to pick one
	}

	idx := s.idx.Load()
	if idx == nil {
		return &documentv1.SuggestTitlesResponse{Suggestions: []*documentv1.TitleSuggestion{}}, nil
	}

	suggestions := idx.Suggest(req.GetQuery(), maxDistance)
	out := make([]*documentv1.TitleSuggestion, len(suggestions))
	for i, sug := range suggestions {
		out[i] = &documentv1.TitleSuggestion{PageId: sug.PageID.String(), Title: sug.Title, Distance: int32(sug.Distance)}
	}
	return &documentv1.SuggestTitlesResponse{Suggestions: out}, nil
}
