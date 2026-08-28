package graph

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	documentv1 "marginal/document-service/genproto/documentv1"
	"marginal/graphalgo"
)

// Server implements documentv1.GraphServiceServer over a *PostgresRepo —
// proto <-> domain translation only; graphalgo has every
// algorithm. See docs/api/graph.md.
type Server struct {
	documentv1.UnimplementedGraphServiceServer
	repo *PostgresRepo
}

func NewServer(repo *PostgresRepo) *Server { return &Server{repo: repo} }

func (s *Server) GetLinkGraph(ctx context.Context, _ *documentv1.GetLinkGraphRequest) (*documentv1.LinkGraph, error) {
	g, err := s.repo.LoadGraph(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "graph: loading link graph failed")
	}

	nodes := make([]*documentv1.GraphNode, 0, len(g.Graph.Nodes))
	for _, id := range g.Graph.Nodes {
		n := g.Nodes[id]
		nodes = append(nodes, &documentv1.GraphNode{Id: string(id), Title: n.Title, IsRoot: n.IsRoot})
	}
	edges := make([]*documentv1.GraphEdge, 0, len(g.Graph.Edges))
	for _, e := range g.Graph.Edges {
		edges = append(edges, &documentv1.GraphEdge{FromPage: string(e.From), ToPage: string(e.To)})
	}
	return &documentv1.LinkGraph{Nodes: nodes, Edges: edges}, nil
}

func (s *Server) AnalyzeGraph(ctx context.Context, _ *documentv1.AnalyzeGraphRequest) (*documentv1.GraphAnalysis, error) {
	g, err := s.repo.LoadGraph(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "graph: loading link graph failed")
	}

	comp := graphalgo.Components(g.Graph)
	componentOf := make(map[string]int32, len(comp))
	for id, c := range comp {
		componentOf[string(id)] = int32(c)
	}

	orphans := graphalgo.Orphans(comp, g.Roots)
	orphanComponents := make([]int32, len(orphans))
	for i, o := range orphans {
		orphanComponents[i] = int32(o)
	}

	var cycle []string
	if c := graphalgo.DetectCycle(g.Graph); c != nil {
		cycle = make([]string, len(c))
		for i, id := range c {
			cycle[i] = string(id)
		}
	}

	betti := graphalgo.Betti(g.Graph)

	// Brandes' betweenness over the undirected view — a page's role in the
	// graph, which its degree does not report (graphalgo.Betweenness).
	bc := graphalgo.Betweenness(g.Graph)
	betweenness := make(map[string]float64, len(bc))
	for id, v := range bc {
		betweenness[string(id)] = v
	}

	// Q is scored twice, against two different partitions. By TOPIC is what
	// pages declare; by COMPONENT is what the wiring implies. Reporting one
	// without the other would be a number with nothing to read it against —
	// the gap between them is the actual finding.
	byTopic := make(map[graphalgo.NodeID]string, len(g.Nodes))
	byComponent := make(map[graphalgo.NodeID]string, len(comp))
	for id, n := range g.Nodes {
		byTopic[id] = n.Topic
	}
	for id, c := range comp {
		byComponent[id] = strconv.Itoa(c)
	}

	return &documentv1.GraphAnalysis{
		Betweenness:           betweenness,
		ModularityByTopic:     graphalgo.Modularity(g.Graph, byTopic),
		ModularityByComponent: graphalgo.Modularity(g.Graph, byComponent),
		ComponentOf:           componentOf,
		OrphanComponents:      orphanComponents,
		Cycle:                 cycle,
		Diameter:              int32(graphalgo.Diameter(g.Graph)),
		Betti: &documentv1.BettiNumbers{
			B0:        int32(betti.B0),
			B1:        int32(betti.B1),
			B1Clique:  int32(betti.B1Clique),
			B2:        int32(betti.B2),
			Chi:       int32(betti.Chi),
			Triangles: int32(betti.Triangles),
			Rank2:     int32(betti.Rank2),
		},
	}, nil
}

func (s *Server) GraphNeighborhood(ctx context.Context, req *documentv1.GraphNeighborhoodRequest) (*documentv1.GraphNeighborhoodResponse, error) {
	if _, err := uuid.Parse(req.SourcePageId); err != nil {
		return nil, status.Error(codes.InvalidArgument, "graph: invalid source_page_id")
	}
	source := graphalgo.NodeID(req.SourcePageId)

	g, err := s.repo.LoadGraph(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "graph: loading link graph failed")
	}
	if _, ok := g.Nodes[source]; !ok {
		return nil, status.Error(codes.NotFound, "graph: source_page_id is not a live page")
	}

	dist, _ := graphalgo.BFS(g.Graph, source)
	undirected := make(map[string]int32, len(dist))
	for id, d := range dist {
		undirected[string(id)] = int32(d)
	}

	forward := graphalgo.ForwardReachable(g.Graph, source)
	forwardOut := make(map[string]int32, len(forward))
	for id, d := range forward {
		forwardOut[string(id)] = int32(d)
	}

	return &documentv1.GraphNeighborhoodResponse{
		UndirectedDistance: undirected,
		ForwardReachable:   forwardOut,
	}, nil
}
