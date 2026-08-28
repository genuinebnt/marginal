package service_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	documentv1 "marginal/document-service/genproto/documentv1"

	diagnosticsv1 "marginal/diagnostics-service/genproto/diagnosticsv1"
	"marginal/diagnostics-service/internal/service"
)

// fakeDocumentService is a small hand-written PageService+GraphService —
// this package tests DiagnosticsService's own translation layer (gRPC
// client calls -> analyzers.Context/facts.PageBlocks -> back to proto),
// not document-service's real business logic (already covered by that
// service's own tests), so a real backend isn't needed here.
type fakeDocumentService struct {
	documentv1.UnimplementedPageServiceServer
	documentv1.UnimplementedGraphServiceServer

	pages     map[string]*documentv1.Page
	blocks    map[string][]*documentv1.Block
	linkGraph *documentv1.LinkGraph
}

func (f *fakeDocumentService) GetPage(_ context.Context, req *documentv1.GetPageRequest) (*documentv1.Page, error) {
	return f.pages[req.GetId()], nil
}
func (f *fakeDocumentService) ListPages(context.Context, *documentv1.ListPagesRequest) (*documentv1.ListPagesResponse, error) {
	pages := make([]*documentv1.Page, 0, len(f.pages))
	for _, p := range f.pages {
		pages = append(pages, p)
	}
	return &documentv1.ListPagesResponse{Pages: pages}, nil
}
func (f *fakeDocumentService) ListBlocks(_ context.Context, req *documentv1.ListBlocksRequest) (*documentv1.ListBlocksResponse, error) {
	return &documentv1.ListBlocksResponse{Blocks: f.blocks[req.GetPageId()]}, nil
}
func (f *fakeDocumentService) GetLinkGraph(context.Context, *documentv1.GetLinkGraphRequest) (*documentv1.LinkGraph, error) {
	return f.linkGraph, nil
}

func newTestServer(t *testing.T, fake *fakeDocumentService) diagnosticsv1.DiagnosticsServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	documentv1.RegisterPageServiceServer(srv, fake)
	documentv1.RegisterGraphServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	diagSrv := service.NewServer(documentv1.NewPageServiceClient(conn), documentv1.NewGraphServiceClient(conn))

	diagLis := bufconn.Listen(1024 * 1024)
	diagGrpc := grpc.NewServer()
	diagnosticsv1.RegisterDiagnosticsServiceServer(diagGrpc, diagSrv)
	go func() { _ = diagGrpc.Serve(diagLis) }()
	t.Cleanup(diagGrpc.Stop)

	diagConn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return diagLis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = diagConn.Close() })

	return diagnosticsv1.NewDiagnosticsServiceClient(diagConn)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

func TestAnalyzePageFindsARealDanglingLinkThroughTheWholeStack(t *testing.T) {
	blockID := uuid.Must(uuid.NewV7()).String()
	fake := &fakeDocumentService{
		pages: map[string]*documentv1.Page{"home": {Id: "home", Title: "Home"}},
		blocks: map[string][]*documentv1.Block{
			"home": {{
				Id:          blockID,
				KindJson:    `{"tag":"paragraph"}`,
				ContentJson: mustJSON(t, map[string]any{"text": "See [[Nowhere]]."}),
			}},
		},
		linkGraph: &documentv1.LinkGraph{Nodes: []*documentv1.GraphNode{{Id: "home", Title: "Home", IsRoot: true}}},
	}
	client := newTestServer(t, fake)

	resp, err := client.AnalyzePage(context.Background(), &diagnosticsv1.AnalyzePageRequest{PageId: "home"})
	require.NoError(t, err)
	require.Len(t, resp.Diagnostics, 1)
	assert.Equal(t, "DanglingPageLink", resp.Diagnostics[0].Analyzer)
	assert.Equal(t, "hint", resp.Diagnostics[0].Severity)
	require.NotNil(t, resp.Diagnostics[0].BlockId)
	assert.Equal(t, blockID, resp.Diagnostics[0].GetBlockId())
}

func TestAnalyzePageHeadingSkipThroughRealJSONUnmarshalling(t *testing.T) {
	fake := &fakeDocumentService{
		pages: map[string]*documentv1.Page{"home": {Id: "home", Title: "Home"}},
		blocks: map[string][]*documentv1.Block{
			"home": {
				{Id: uuid.Must(uuid.NewV7()).String(), KindJson: `{"tag":"heading","level":1}`, ContentJson: `{"text":"Intro"}`},
				{Id: uuid.Must(uuid.NewV7()).String(), KindJson: `{"tag":"heading","level":3}`, ContentJson: `{"text":"Detail"}`},
			},
		},
		linkGraph: &documentv1.LinkGraph{Nodes: []*documentv1.GraphNode{{Id: "home", Title: "Home", IsRoot: true}}},
	}
	client := newTestServer(t, fake)

	resp, err := client.AnalyzePage(context.Background(), &diagnosticsv1.AnalyzePageRequest{PageId: "home"})
	require.NoError(t, err)
	require.Len(t, resp.Diagnostics, 1)
	assert.Equal(t, "HeadingSkip", resp.Diagnostics[0].Analyzer)
}

func TestAnalyzeFactsAndStaleReferencesThroughTheWholeStack(t *testing.T) {
	fake := &fakeDocumentService{
		pages: map[string]*documentv1.Page{
			"defs": {Id: "defs", Title: "Definitions"},
			"doc":  {Id: "doc", Title: "Rollout"},
		},
		blocks: map[string][]*documentv1.Block{
			"defs": {{Id: uuid.Must(uuid.NewV7()).String(), KindJson: `{"tag":"paragraph"}`, ContentJson: mustJSON(t, map[string]any{"text": "{{define ack-budget = 40ms}}"})}},
			"doc":  {{Id: uuid.Must(uuid.NewV7()).String(), KindJson: `{"tag":"paragraph"}`, ContentJson: mustJSON(t, map[string]any{"text": "Ship under {{ack-budget}}."})}},
		},
		linkGraph: &documentv1.LinkGraph{},
	}
	client := newTestServer(t, fake)

	resp, err := client.AnalyzeFacts(context.Background(), &diagnosticsv1.AnalyzeFactsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Definitions, 1)
	assert.Equal(t, "ack-budget", resp.Definitions[0].Name)
	assert.Equal(t, "40ms", resp.Definitions[0].Value)
	require.Len(t, resp.References, 1)
	assert.Equal(t, "doc", resp.References[0].PageId)

	stale, err := client.StaleReferences(context.Background(), &diagnosticsv1.StaleReferencesRequest{FactName: "ack-budget"})
	require.NoError(t, err)
	require.Len(t, stale.References, 1)
	assert.Equal(t, "doc", stale.References[0].PageId)
}
