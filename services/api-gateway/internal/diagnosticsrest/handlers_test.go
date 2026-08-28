package diagnosticsrest_test

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

	diagnosticsv1 "marginal/diagnostics-service/genproto/diagnosticsv1"

	"marginal/api-gateway/internal/diagnosticsrest"
)

// fakeDiagnosticsService is a small hand-written DiagnosticsService —
// this package tests REST↔gRPC translation, not diagnostics-service's
// own analyzers/facts logic (already covered by that service's own
// tests), so a real backend isn't needed here.
type fakeDiagnosticsService struct {
	diagnosticsv1.UnimplementedDiagnosticsServiceServer

	analyzePageFn     func(*diagnosticsv1.AnalyzePageRequest) (*diagnosticsv1.AnalyzePageResponse, error)
	analyzeFactsFn    func(*diagnosticsv1.AnalyzeFactsRequest) (*diagnosticsv1.AnalyzeFactsResponse, error)
	staleReferencesFn func(*diagnosticsv1.StaleReferencesRequest) (*diagnosticsv1.StaleReferencesResponse, error)
}

func (f *fakeDiagnosticsService) AnalyzePage(_ context.Context, req *diagnosticsv1.AnalyzePageRequest) (*diagnosticsv1.AnalyzePageResponse, error) {
	return f.analyzePageFn(req)
}
func (f *fakeDiagnosticsService) AnalyzeFacts(_ context.Context, req *diagnosticsv1.AnalyzeFactsRequest) (*diagnosticsv1.AnalyzeFactsResponse, error) {
	return f.analyzeFactsFn(req)
}
func (f *fakeDiagnosticsService) StaleReferences(_ context.Context, req *diagnosticsv1.StaleReferencesRequest) (*diagnosticsv1.StaleReferencesResponse, error) {
	return f.staleReferencesFn(req)
}

func newTestServer(t *testing.T, fake *fakeDiagnosticsService) *httptest.Server {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	diagnosticsv1.RegisterDiagnosticsServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	h := diagnosticsrest.NewHandler(diagnosticsv1.NewDiagnosticsServiceClient(conn))
	r := chi.NewRouter()
	h.Mount(r)
	httpSrv := httptest.NewServer(r)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

func TestAnalyzePageTranslatesDiagnostics(t *testing.T) {
	blockID := "b1"
	fake := &fakeDiagnosticsService{
		analyzePageFn: func(req *diagnosticsv1.AnalyzePageRequest) (*diagnosticsv1.AnalyzePageResponse, error) {
			assert.Equal(t, "p1", req.PageId)
			return &diagnosticsv1.AnalyzePageResponse{Diagnostics: []*diagnosticsv1.Diagnostic{
				{Analyzer: "DanglingPageLink", Severity: "hint", Message: "nope", BlockId: &blockID},
			}}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/pages/p1/diagnostics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Diagnostics []struct {
			Analyzer string `json:"analyzer"`
			Severity string `json:"severity"`
			BlockID  string `json:"block_id"`
		} `json:"diagnostics"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Diagnostics, 1)
	assert.Equal(t, "DanglingPageLink", body.Diagnostics[0].Analyzer)
	assert.Equal(t, "b1", body.Diagnostics[0].BlockID)
}

func TestAnalyzeFactsTranslatesDefinitionsDuplicatesAndCycle(t *testing.T) {
	fake := &fakeDiagnosticsService{
		analyzeFactsFn: func(*diagnosticsv1.AnalyzeFactsRequest) (*diagnosticsv1.AnalyzeFactsResponse, error) {
			return &diagnosticsv1.AnalyzeFactsResponse{
				Definitions: []*diagnosticsv1.Definition{{Name: "a", Value: "1", PageId: "p1", BlockId: "b1"}},
				Duplicates: []*diagnosticsv1.DuplicateGroup{{
					Name:        "dup",
					Definitions: []*diagnosticsv1.Definition{{Name: "dup", Value: "x", PageId: "p1", BlockId: "b1"}, {Name: "dup", Value: "y", PageId: "p2", BlockId: "b2"}},
				}},
				Cycle:      []string{"a", "b", "a"},
				References: []*diagnosticsv1.Reference{{Name: "a", PageId: "p2", BlockId: "b2"}},
			}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/facts")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Definitions []struct {
			Name string `json:"name"`
		} `json:"definitions"`
		Duplicates []struct {
			Name        string `json:"name"`
			Definitions []any  `json:"definitions"`
		} `json:"duplicates"`
		Cycle      []string `json:"cycle"`
		References []struct {
			Name string `json:"name"`
		} `json:"references"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Definitions, 1)
	assert.Equal(t, "a", body.Definitions[0].Name)
	require.Len(t, body.Duplicates, 1)
	assert.Len(t, body.Duplicates[0].Definitions, 2)
	assert.Equal(t, []string{"a", "b", "a"}, body.Cycle)
	require.Len(t, body.References, 1)
}

func TestAnalyzeFactsAcyclicHasEmptyNotNullCycle(t *testing.T) {
	fake := &fakeDiagnosticsService{
		analyzeFactsFn: func(*diagnosticsv1.AnalyzeFactsRequest) (*diagnosticsv1.AnalyzeFactsResponse, error) {
			return &diagnosticsv1.AnalyzeFactsResponse{}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/facts")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var raw map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	assert.Equal(t, "[]", string(raw["cycle"]), "an acyclic facts graph's cycle field must be [], not null")
	assert.Equal(t, "[]", string(raw["definitions"]))
	assert.Equal(t, "[]", string(raw["duplicates"]))
	assert.Equal(t, "[]", string(raw["references"]))
}

func TestStaleReferencesPassesFactNameAndTranslatesResult(t *testing.T) {
	fake := &fakeDiagnosticsService{
		staleReferencesFn: func(req *diagnosticsv1.StaleReferencesRequest) (*diagnosticsv1.StaleReferencesResponse, error) {
			assert.Equal(t, "ack-budget", req.FactName)
			return &diagnosticsv1.StaleReferencesResponse{References: []*diagnosticsv1.Reference{{Name: "ack-budget", PageId: "p1", BlockId: "b1"}}}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/facts/ack-budget/stale")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body []struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body, 1)
	assert.Equal(t, "ack-budget", body[0].Name)
}
