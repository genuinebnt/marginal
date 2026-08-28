package searchrest_test

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
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	documentv1 "marginal/document-service/genproto/documentv1"

	"marginal/api-gateway/internal/searchrest"
)

// fakeSearchService is a small hand-written SearchService — this package
// tests REST↔gRPC translation, not internal/search's own full-text/
// BK-tree logic (already covered by document-service's own tests), so a
// real backend isn't needed here.
type fakeSearchService struct {
	documentv1.UnimplementedSearchServiceServer

	searchFn  func(*documentv1.SearchRequest) (*documentv1.SearchResponse, error)
	suggestFn func(*documentv1.SuggestTitlesRequest) (*documentv1.SuggestTitlesResponse, error)
}

func (f *fakeSearchService) Search(_ context.Context, req *documentv1.SearchRequest) (*documentv1.SearchResponse, error) {
	return f.searchFn(req)
}
func (f *fakeSearchService) SuggestTitles(_ context.Context, req *documentv1.SuggestTitlesRequest) (*documentv1.SuggestTitlesResponse, error) {
	return f.suggestFn(req)
}

func newTestServer(t *testing.T, fake *fakeSearchService) *httptest.Server {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	documentv1.RegisterSearchServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	h := searchrest.NewHandler(documentv1.NewSearchServiceClient(conn))
	r := chi.NewRouter()
	h.Mount(r)
	httpSrv := httptest.NewServer(r)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

func TestSearchTranslatesTitleAndBlockHits(t *testing.T) {
	blockID := "b1"
	snippet := "ship on <b>budget</b>-critical dates"
	fake := &fakeSearchService{
		searchFn: func(req *documentv1.SearchRequest) (*documentv1.SearchResponse, error) {
			assert.Equal(t, "budget", req.Query)
			return &documentv1.SearchResponse{Hits: []*documentv1.SearchHit{
				{PageId: "p1", PageTitle: "Performance budget", Rank: 0.6},
				{PageId: "p2", PageTitle: "Rollout plan", BlockId: &blockID, Snippet: &snippet, Rank: 0.3},
			}}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/search?q=budget")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Hits []struct {
			PageID  string  `json:"page_id"`
			BlockID *string `json:"block_id"`
			Snippet *string `json:"snippet"`
		} `json:"hits"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Hits, 2)
	assert.Equal(t, "p1", body.Hits[0].PageID)
	assert.Nil(t, body.Hits[0].BlockID)
	require.NotNil(t, body.Hits[1].BlockID)
	assert.Equal(t, "b1", *body.Hits[1].BlockID)
	require.NotNil(t, body.Hits[1].Snippet)
	assert.Equal(t, snippet, *body.Hits[1].Snippet)
}

func TestSearchEmptyResultsIsEmptyArrayNotNull(t *testing.T) {
	fake := &fakeSearchService{
		searchFn: func(*documentv1.SearchRequest) (*documentv1.SearchResponse, error) {
			return &documentv1.SearchResponse{}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/search?q=nothing")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var raw map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	assert.Equal(t, "[]", string(raw["hits"]))
}

func TestSuggestPassesQueryAndMaxDistance(t *testing.T) {
	fake := &fakeSearchService{
		suggestFn: func(req *documentv1.SuggestTitlesRequest) (*documentv1.SuggestTitlesResponse, error) {
			assert.Equal(t, "Performnace", req.Query)
			assert.Equal(t, int32(2), req.MaxDistance)
			return &documentv1.SuggestTitlesResponse{Suggestions: []*documentv1.TitleSuggestion{
				{PageId: "p1", Title: "Performance budget", Distance: 2},
			}}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/search/suggest?q=Performnace&max_distance=2")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Suggestions []struct {
			Title    string `json:"title"`
			Distance int32  `json:"distance"`
		} `json:"suggestions"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Suggestions, 1)
	assert.Equal(t, "Performance budget", body.Suggestions[0].Title)
	assert.Equal(t, int32(2), body.Suggestions[0].Distance)
}

func TestSuggestOmittedMaxDistanceDefaultsToZeroOnTheWire(t *testing.T) {
	fake := &fakeSearchService{
		suggestFn: func(req *documentv1.SuggestTitlesRequest) (*documentv1.SuggestTitlesResponse, error) {
			assert.Equal(t, int32(0), req.MaxDistance, "the gateway sends 0 when unset — SuggestTitles' own server-side default (2) takes over from there")
			return &documentv1.SuggestTitlesResponse{}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/search/suggest?q=Performnace")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
