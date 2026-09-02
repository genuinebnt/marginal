package graph

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	documentv1 "marginal/document-service/genproto/documentv1"
	"marginal/graphalgo"
)

// Server implements documentv1.GraphServiceServer over a *PostgresRepo —
// proto <-> domain translation only; graphalgo has every
// algorithm. See docs/api/graph.md.
// SpaceReader answers "which spaces is this caller in" — the same port
// pages.Server takes, declared here at its own point of use.
type SpaceReader interface {
	SpacesFor(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

type Server struct {
	documentv1.UnimplementedGraphServiceServer
	repo   *PostgresRepo
	spaces SpaceReader
}

func NewServer(repo *PostgresRepo, spaces SpaceReader) *Server {
	return &Server{repo: repo, spaces: spaces}
}

// scope is every graph read's first step, and the reason this file changed
// in v3.3.0: none of them had one. GetLinkGraph, AnalyzeGraph and
// GraphNeighborhood each returned EVERY page title on the instance to
// anybody who asked, including titles in spaces the caller is not a member
// of — the same class of hole v3.1.0's audit found twice elsewhere, in a
// service that had already been given the tool to avoid it.
//
// A caller with no spaces gets an empty graph, not the whole one.
func (s *Server) scope(ctx context.Context) ([]uuid.UUID, error) { return ScopeFor(ctx, s.spaces) }

// ScopeFor is the same resolution, exported for the other packages that
// read across pages — discover and search both do, and both were missing
// it. One implementation, so a future reader cannot be scoped "nearly".
func ScopeFor(ctx context.Context, spaces SpaceReader) ([]uuid.UUID, error) {
	actor, err := actorID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "graph: an actor id is required")
	}
	ids, err := spaces.SpacesFor(ctx, actor)
	if err != nil {
		return nil, status.Error(codes.Internal, "graph: resolving visible spaces failed")
	}
	return ids, nil
}

// actorID mirrors pages.actorID — the same gRPC metadata key, read the
// same way. Duplicated rather than exported across packages because it is
// four lines and a shared one would make two unrelated packages share a
// dependency for the sake of it.
func actorID(ctx context.Context) (uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return uuid.UUID{}, errNoActor
	}
	values := md.Get("actor-id")
	if len(values) == 0 || values[0] == "" {
		return uuid.UUID{}, errNoActor
	}
	return uuid.Parse(values[0])
}

var errNoActor = status.Error(codes.Unauthenticated, "graph: missing actor id")

func (s *Server) GetLinkGraph(ctx context.Context, _ *documentv1.GetLinkGraphRequest) (*documentv1.LinkGraph, error) {
	spaceIDs, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.repo.LoadGraph(ctx, spaceIDs)
	if err != nil {
		return nil, status.Error(codes.Internal, "graph: loading link graph failed")
	}

	nodes := make([]*documentv1.GraphNode, 0, len(g.Graph.Nodes))
	for _, id := range g.Graph.Nodes {
		n := g.Nodes[id]
		nodes = append(nodes, &documentv1.GraphNode{
			Id: string(id), Title: n.Title, IsRoot: n.IsRoot,
			TopicName: n.TopicName, TopicColorKey: n.TopicColorKey, Tags: n.Tags,
		})
	}
	edges := make([]*documentv1.GraphEdge, 0, len(g.Graph.Edges))
	for _, e := range g.Graph.Edges {
		edges = append(edges, &documentv1.GraphEdge{FromPage: string(e.From), ToPage: string(e.To)})
	}
	return &documentv1.LinkGraph{Nodes: nodes, Edges: edges}, nil
}

func (s *Server) AnalyzeGraph(ctx context.Context, _ *documentv1.AnalyzeGraphRequest) (*documentv1.GraphAnalysis, error) {
	spaceIDs, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.repo.LoadGraph(ctx, spaceIDs)
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

	// Strong connectivity, over the DIRECTED graph. component_of above
	// ignores direction and answers "is this reachable at all"; this answers
	// "is it reachable both ways", and a component of size > 1 is a set of
	// pages citing each other in a loop.
	scc := graphalgo.StronglyConnected(g.Graph)
	stronglyConnected := make(map[string]int32, len(scc))
	for id, c := range scc {
		stronglyConnected[string(id)] = int32(c)
	}
	sizes := graphalgo.SCCSizes(scc)
	sccSizes := make([]int32, len(sizes))
	for i, n := range sizes {
		sccSizes[i] = int32(n)
	}

	// A reading order in which nothing precedes what it links to. Partial
	// when the graph has a cycle — which is the useful half of that failure,
	// so `unplaced` ships alongside rather than the whole thing collapsing
	// into an error.
	order, isDAG := graphalgo.TopologicalSort(g.Graph)
	topological := make([]string, len(order))
	for i, id := range order {
		topological[i] = string(id)
	}
	unplacedIDs := graphalgo.Unplaced(g.Graph, order)
	unplaced := make([]string, len(unplacedIDs))
	for i, id := range unplacedIDs {
		unplaced[i] = string(id)
	}
	layers := make([]*documentv1.GraphLayer, 0)
	for _, level := range graphalgo.Layers(g.Graph, order) {
		ids := make([]string, len(level))
		for i, id := range level {
			ids[i] = string(id)
		}
		layers = append(layers, &documentv1.GraphLayer{PageIds: ids})
	}

	return &documentv1.GraphAnalysis{
		Betweenness:           betweenness,
		StronglyConnected:     stronglyConnected,
		SccSizes:              sccSizes,
		TopologicalOrder:      topological,
		IsDag:                 isDAG,
		Unplaced:              unplaced,
		Layers:                layers,
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

	spaceIDs, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.repo.LoadGraph(ctx, spaceIDs)
	if err != nil {
		return nil, status.Error(codes.Internal, "graph: loading link graph failed")
	}
	if _, ok := g.Nodes[source]; !ok {
		return nil, status.Error(codes.NotFound, "graph: source_page_id is not a live page")
	}

	dist, prev := graphalgo.BFS(g.Graph, source)
	undirected := make(map[string]int32, len(dist))
	for id, d := range dist {
		undirected[string(id)] = int32(d)
	}

	forward := graphalgo.ForwardReachable(g.Graph, source)
	forwardOut := make(map[string]int32, len(forward))
	for id, d := range forward {
		forwardOut[string(id)] = int32(d)
	}

	// The ranked ring around the source. Titles are joined here rather than
	// left to the client: a "nearest pages" list that ships bare ids forces
	// every caller to re-fetch the graph to render one panel.
	const nearestLimit = 12
	nearest := make([]*documentv1.GraphNeighbour, 0, nearestLimit)
	for _, nb := range graphalgo.NearestNeighbours(g.Graph, source, nearestLimit) {
		nearest = append(nearest, &documentv1.GraphNeighbour{
			PageId: string(nb.ID),
			Title:  g.Nodes[nb.ID].Title,
			Hops:   int32(nb.Hops),
		})
	}
	rings := graphalgo.RingSizes(g.Graph, source)
	ringSizes := make([]int32, len(rings))
	for i, n := range rings {
		ringSizes[i] = int32(n)
	}

	// "What to read before this page." Over the same graph, one more walk —
	// a second reader of the link structure, not a second copy of it.
	path := make([]*documentv1.PathStep, 0)
	for _, step := range graphalgo.ReadingPath(g.Graph, source) {
		path = append(path, &documentv1.PathStep{
			PageId:      string(step.ID),
			Title:       g.Nodes[step.ID].Title,
			Depth:       int32(step.Depth),
			Destination: step.Destination,
		})
	}

	// source → target, when a target was named. Falls out of the BFS
	// already run above: one traversal answers both questions.
	//
	// A target naming a page that is not live is INVALID_ARGUMENT rather
	// than an empty path — "you asked about a page that does not exist"
	// and "those two are not connected" are different answers, and a
	// screen that renders them identically teaches the wrong thing.
	shortest := make([]*documentv1.PathStep, 0)
	pathExists := false
	if req.TargetPageId != "" {
		if _, err := uuid.Parse(req.TargetPageId); err != nil {
			return nil, status.Error(codes.InvalidArgument, "graph: invalid target_page_id")
		}
		target := graphalgo.NodeID(req.TargetPageId)
		if _, ok := g.Nodes[target]; !ok {
			return nil, status.Error(codes.NotFound, "graph: target_page_id is not a live page")
		}
		hops, ok := graphalgo.ShortestPath(dist, prev, source, target)
		pathExists = ok
		for i, id := range hops {
			shortest = append(shortest, &documentv1.PathStep{
				PageId:      string(id),
				Title:       g.Nodes[id].Title,
				Depth:       int32(i),
				Destination: i == len(hops)-1,
			})
		}
	}

	return &documentv1.GraphNeighborhoodResponse{
		ReadingPath:        path,
		UndirectedDistance: undirected,
		ForwardReachable:   forwardOut,
		Nearest:            nearest,
		RingSizes:          ringSizes,
		ShortestPath:       shortest,
		PathExists:         pathExists,
	}, nil
}

func (s *Server) ListDanglingLinks(ctx context.Context, _ *documentv1.ListDanglingLinksRequest) (*documentv1.ListDanglingLinksResponse, error) {
	spaceIDs, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	links, err := s.repo.DanglingLinks(ctx, spaceIDs)
	if err != nil {
		return nil, status.Error(codes.Internal, "graph: loading dangling links failed")
	}
	out := make([]*documentv1.DanglingLink, 0, len(links))
	for _, l := range links {
		out = append(out, &documentv1.DanglingLink{
			TargetTitle: l.TargetTitle, FromPage: l.FromPage.String(),
			FromPageTitle: l.FromPageTitle, FromBlock: l.FromBlock.String(),
		})
	}
	return &documentv1.ListDanglingLinksResponse{Links: out}, nil
}
