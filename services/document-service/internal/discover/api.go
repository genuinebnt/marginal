package discover

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	documentv1 "marginal/document-service/genproto/documentv1"
	"marginal/document-service/internal/graph"
	"marginal/semantic"
)

// Server implements documentv1.DiscoverServiceServer — proto <-> domain
// translation only. Every algorithm is marginal/semantic's (vectors, HNSW) or
// marginal/graphalgo's (hop distance); Near above is assembly.
//
// It holds a graph repo as well as its own because one of the three signals
// § 09 reports is link distance, and that is the SAME BFS /graph/neighborhood
// runs over the same table. Reading it here rather than re-deriving it is the
// difference between a second reader of one result and a second
// implementation of it.
type Server struct {
	documentv1.UnimplementedDiscoverServiceServer
	repo   *PostgresRepo
	graph  *graph.PostgresRepo
	spaces graph.SpaceReader
}

func NewServer(repo *PostgresRepo, graphRepo *graph.PostgresRepo, spaces graph.SpaceReader) *Server {
	return &Server{repo: repo, graph: graphRepo, spaces: spaces}
}

func (s *Server) Near(ctx context.Context, req *documentv1.NearRequest) (*documentv1.NearResponse, error) {
	// A typed query needs no page id at all, so the uuid check applies only
	// when the page IS the origin. Demanding one anyway would make the text
	// box depend on a selection it has nothing to do with.
	if req.GetQueryText() == "" {
		if _, err := uuid.Parse(req.GetSourcePageId()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "discover: invalid source_page_id")
		}
	}

	// Discover names other pages and quotes their titles, so it is scoped
	// exactly like every other read (v3.3.0 — it was not before).
	spaceIDs, err := graph.ScopeFor(ctx, s.spaces)
	if err != nil {
		return nil, err
	}
	pages, err := s.repo.LoadCorpus(ctx, spaceIDs)
	if err != nil {
		return nil, status.Error(codes.Internal, "discover: loading corpus failed")
	}
	links, err := s.graph.LoadGraph(ctx, spaceIDs)
	if err != nil {
		return nil, status.Error(codes.Internal, "discover: loading link graph failed")
	}

	neighbours, stats, err := Near(pages, links.Graph, Query{
		PageID:   req.GetSourcePageId(),
		Text:     req.GetQueryText(),
		K:        int(req.GetK()),
		Topics:   req.GetTopics(),
		MustTags: req.GetTags(),
	})
	if errors.Is(err, ErrUnknownPage) {
		return nil, status.Error(codes.NotFound, "discover: source_page_id is not a live page")
	}
	if errors.Is(err, ErrEmptyQuery) {
		return nil, status.Error(codes.InvalidArgument, "discover: query has no indexable terms")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "discover: query failed")
	}

	out := &documentv1.NearResponse{
		Neighbours: make([]*documentv1.SemanticNeighbour, len(neighbours)),
		Topics:     SortedTopics(pages),
		Stats: &documentv1.DiscoverStats{
			Comparisons:      int32(stats.Comparisons),
			ExactComparisons: int32(stats.ExactComparisons),
			Hops:             int32(stats.Hops),
			Layers:           int32(stats.Layers),
			RecallAtK:        stats.RecallAtK,
			Candidates:       int32(stats.Candidates),
			LayerSizes:       toInt32s(stats.LayerSizes),
			Corpus:           int32(stats.Corpus),
			TopTerms:         stats.TopTerms,
			M:                ParamM,
			EfSearch:         ParamEfSearch,
			Dimensions:       semantic.Dim,
			HasOrigin:        stats.HasOrigin,
		},
	}
	for i, n := range neighbours {
		out.Neighbours[i] = &documentv1.SemanticNeighbour{
			PageId:        n.PageID,
			Title:         n.Title,
			Excerpt:       n.Excerpt,
			TopicName:     n.TopicName,
			TopicColorKey: n.TopicColorKey,
			Tags:          n.Tags,
			Cosine:        n.Cosine,
			SharedTags:    int32(n.SharedTags),
			TagJaccard:    n.TagJaccard,
			Hops:          int32(n.Hops),
		}
	}
	return out, nil
}

func toInt32s(in []int) []int32 {
	out := make([]int32, len(in))
	for i, n := range in {
		out[i] = int32(n)
	}
	return out
}
