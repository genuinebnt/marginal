package graphrest_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	documentv1 "marginal/document-service/genproto/documentv1"

	"marginal/api-gateway/internal/graphrest"
)

// fakeGraphService is a small hand-written GraphService implementation —
// this package tests REST↔gRPC translation, not graphalgo's own
// algorithms (already covered by document-service's own tests), so a
// real backend isn't needed here.
type fakeGraphService struct {
	documentv1.UnimplementedGraphServiceServer

	getLinkGraphFn func(*documentv1.GetLinkGraphRequest) (*documentv1.LinkGraph, error)
	analyzeFn      func(*documentv1.AnalyzeGraphRequest) (*documentv1.GraphAnalysis, error)
	neighborhoodFn func(*documentv1.GraphNeighborhoodRequest) (*documentv1.GraphNeighborhoodResponse, error)
}

func (f *fakeGraphService) GetLinkGraph(_ context.Context, req *documentv1.GetLinkGraphRequest) (*documentv1.LinkGraph, error) {
	return f.getLinkGraphFn(req)
}
func (f *fakeGraphService) AnalyzeGraph(_ context.Context, req *documentv1.AnalyzeGraphRequest) (*documentv1.GraphAnalysis, error) {
	return f.analyzeFn(req)
}
func (f *fakeGraphService) GraphNeighborhood(_ context.Context, req *documentv1.GraphNeighborhoodRequest) (*documentv1.GraphNeighborhoodResponse, error) {
	return f.neighborhoodFn(req)
}

func newTestServer(t *testing.T, fake *fakeGraphService) *httptest.Server {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	documentv1.RegisterGraphServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	h := graphrest.NewHandler(documentv1.NewGraphServiceClient(conn))
	r := chi.NewRouter()
	h.Mount(r)
	httpSrv := httptest.NewServer(r)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

func TestGetLinkGraphTranslatesNodesAndEdges(t *testing.T) {
	fake := &fakeGraphService{
		getLinkGraphFn: func(*documentv1.GetLinkGraphRequest) (*documentv1.LinkGraph, error) {
			return &documentv1.LinkGraph{
				Nodes: []*documentv1.GraphNode{{Id: "a", Title: "A", IsRoot: true}, {Id: "b", Title: "B"}},
				Edges: []*documentv1.GraphEdge{{FromPage: "a", ToPage: "b"}},
			}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/graph")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Nodes []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			IsRoot bool   `json:"is_root"`
		} `json:"nodes"`
		Edges []struct {
			FromPage string `json:"from_page"`
			ToPage   string `json:"to_page"`
		} `json:"edges"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Nodes, 2)
	assert.True(t, body.Nodes[0].IsRoot)
	require.Len(t, body.Edges, 1)
	assert.Equal(t, "a", body.Edges[0].FromPage)
}

func TestAnalyzeGraphTranslatesComponentsAndCycle(t *testing.T) {
	fake := &fakeGraphService{
		analyzeFn: func(*documentv1.AnalyzeGraphRequest) (*documentv1.GraphAnalysis, error) {
			return &documentv1.GraphAnalysis{
				ComponentOf:      map[string]int32{"a": 0, "b": 0},
				OrphanComponents: []int32{0},
				Cycle:            []string{"a", "b", "a"},
				Diameter:         1,
				Betti:            &documentv1.BettiNumbers{B0: 1, B1: 1, Triangles: 1, Rank2: 1},
			}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/graph/analysis")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		ComponentOf      map[string]int32 `json:"component_of"`
		OrphanComponents []int32          `json:"orphan_components"`
		Cycle            []string         `json:"cycle"`
		Diameter         int32            `json:"diameter"`
		Betti            struct {
			B0        int32 `json:"b0"`
			B1        int32 `json:"b1"`
			B1Clique  int32 `json:"b1_clique"`
			Triangles int32 `json:"triangles"`
			Rank2     int32 `json:"rank2"`
		} `json:"betti"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, int32(0), body.ComponentOf["a"])
	assert.Equal(t, []int32{0}, body.OrphanComponents)
	assert.Equal(t, []string{"a", "b", "a"}, body.Cycle)
	assert.Equal(t, int32(1), body.Diameter)
	assert.Equal(t, int32(1), body.Betti.B0)
	assert.Equal(t, int32(1), body.Betti.Triangles)
	assert.Equal(t, int32(1), body.Betti.Rank2)
}

func TestAnalyzeGraphAcyclicHasEmptyNotNullCycle(t *testing.T) {
	fake := &fakeGraphService{
		analyzeFn: func(*documentv1.AnalyzeGraphRequest) (*documentv1.GraphAnalysis, error) {
			return &documentv1.GraphAnalysis{ComponentOf: map[string]int32{}}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/graph/analysis")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var raw map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	assert.Equal(t, "[]", string(raw["cycle"]), "an acyclic graph's cycle field must be [], not null")
	assert.Equal(t, "[]", string(raw["orphan_components"]))
}

func TestGraphNeighborhoodPassesSourceIDAndTranslatesDistances(t *testing.T) {
	fake := &fakeGraphService{
		neighborhoodFn: func(req *documentv1.GraphNeighborhoodRequest) (*documentv1.GraphNeighborhoodResponse, error) {
			assert.Equal(t, "page-a", req.SourcePageId)
			return &documentv1.GraphNeighborhoodResponse{
				UndirectedDistance: map[string]int32{"page-a": 0, "page-b": 1},
				ForwardReachable:   map[string]int32{"page-a": 0},
			}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/graph/neighborhood/page-a")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		UndirectedDistance map[string]int32 `json:"undirected_distance"`
		ForwardReachable   map[string]int32 `json:"forward_reachable"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, int32(1), body.UndirectedDistance["page-b"])
	assert.Equal(t, int32(0), body.ForwardReachable["page-a"])
}

func TestGraphNeighborhoodNotFoundMapsTo404(t *testing.T) {
	fake := &fakeGraphService{
		neighborhoodFn: func(*documentv1.GraphNeighborhoodRequest) (*documentv1.GraphNeighborhoodResponse, error) {
			return nil, status.Error(codes.NotFound, "graph: source_page_id is not a live page")
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/graph/neighborhood/does-not-exist")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
