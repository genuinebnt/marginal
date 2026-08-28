// Package service is DiagnosticsService's translation layer: call
// document-service's PageService/GraphService as a gRPC client, build
// internal/analyzers.Context and internal/facts.PageBlocks from the
// results, run the real analysis, translate back to diagnosticsv1's
// proto types. No algorithm lives here — that split is deliberate, the
// same one internal/graph (document-service) draws between repo.go/api.go.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/google/uuid"

	"marginal/documentcore"
	"marginal/graphalgo"

	documentv1 "marginal/document-service/genproto/documentv1"

	diagnosticsv1 "marginal/diagnostics-service/genproto/diagnosticsv1"
	"marginal/diagnostics-service/internal/analyzers"
	"marginal/diagnostics-service/internal/facts"
)

// Server implements diagnosticsv1.DiagnosticsServiceServer over
// document-service's own two gRPC clients.
type Server struct {
	diagnosticsv1.UnimplementedDiagnosticsServiceServer
	pages       documentv1.PageServiceClient
	graph       documentv1.GraphServiceClient
	systemActor string
}

// NewServer. systemActor is the actor-id document-service's own
// temporary auth stand-in (pages.md's "Actor identity") requires on
// every call — this service acts as itself, not on behalf of one
// specific browser user (every analysis here reads across the whole
// workspace, and this repo's pages carry no per-actor access scoping
// anyway — DATA_MODEL.md's "shared workspace, not multi-tenant"), so a
// fixed synthetic id is the right identity, the same way
// collaboration-service's own serverActor is a synthetic identity
// distinct from a real editing user.
func NewServer(pages documentv1.PageServiceClient, graph documentv1.GraphServiceClient, systemActor string) *Server {
	return &Server{pages: pages, graph: graph, systemActor: systemActor}
}

// withActor attaches this service's own synthetic actor-id to ctx as
// outgoing gRPC metadata — document-service's PageService/GraphService
// both reject a call missing it with UNAUTHENTICATED.
func (s *Server) withActor(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "actor-id", s.systemActor)
}

// buildLinkContext calls GraphService.GetLinkGraph once and shapes it
// into analyzers.Context — the symbol table and link graph every
// cross-page analyzer (DanglingPageLink, AmbiguousPageLink, DuplicateTitle,
// OrphanPage, LinkCycle) needs, and nothing per-page-block-specific does.
func (s *Server) buildLinkContext(ctx context.Context) (analyzers.Context, error) {
	linkGraph, err := s.graph.GetLinkGraph(s.withActor(ctx), &documentv1.GetLinkGraphRequest{})
	if err != nil {
		return analyzers.Context{}, fmt.Errorf("diagnostics: loading link graph: %w", err)
	}

	pages := make([]analyzers.PageInfo, len(linkGraph.GetNodes()))
	nodes := make([]graphalgo.NodeID, len(linkGraph.GetNodes()))
	for i, n := range linkGraph.GetNodes() {
		pages[i] = analyzers.PageInfo{ID: n.GetId(), Title: n.GetTitle(), IsRoot: n.GetIsRoot()}
		nodes[i] = graphalgo.NodeID(n.GetId())
	}
	edges := make([]graphalgo.Edge, len(linkGraph.GetEdges()))
	for i, e := range linkGraph.GetEdges() {
		edges[i] = graphalgo.Edge{From: graphalgo.NodeID(e.GetFromPage()), To: graphalgo.NodeID(e.GetToPage())}
	}

	return analyzers.Context{Pages: pages, Graph: graphalgo.Graph{Nodes: nodes, Edges: edges}}, nil
}

// loadPage calls PageService.GetPage + ListBlocks and unmarshals each
// block's kind_json/content_json back into documentcore's own types —
// the whole reason ListBlocks ships them as documentcore's native JSON
// shape rather than a re-modelled proto message.
func (s *Server) loadPage(ctx context.Context, pageID string) (analyzers.Page, error) {
	page, err := s.pages.GetPage(s.withActor(ctx), &documentv1.GetPageRequest{Id: pageID})
	if err != nil {
		return analyzers.Page{}, fmt.Errorf("diagnostics: loading page: %w", err)
	}
	blocksResp, err := s.pages.ListBlocks(s.withActor(ctx), &documentv1.ListBlocksRequest{PageId: pageID})
	if err != nil {
		return analyzers.Page{}, fmt.Errorf("diagnostics: loading blocks: %w", err)
	}

	blocks := make([]analyzers.Block, len(blocksResp.GetBlocks()))
	for i, b := range blocksResp.GetBlocks() {
		block, err := unmarshalBlock(b)
		if err != nil {
			return analyzers.Page{}, err
		}
		blocks[i] = block
	}

	return analyzers.Page{ID: pageID, Title: page.GetTitle(), Blocks: blocks}, nil
}

func unmarshalBlock(b *documentv1.Block) (analyzers.Block, error) {
	rawID, err := uuid.Parse(b.GetId())
	if err != nil {
		return analyzers.Block{}, fmt.Errorf("diagnostics: parsing block id: %w", err)
	}
	id := documentcore.BlockID(rawID)
	var kind documentcore.BlockKind
	if err := json.Unmarshal([]byte(b.GetKindJson()), &kind); err != nil {
		return analyzers.Block{}, fmt.Errorf("diagnostics: parsing block kind: %w", err)
	}
	var content documentcore.Content
	if err := json.Unmarshal([]byte(b.GetContentJson()), &content); err != nil {
		return analyzers.Block{}, fmt.Errorf("diagnostics: parsing block content: %w", err)
	}
	var parent *documentcore.BlockID
	if b.ParentId != nil {
		rawParent, err := uuid.Parse(b.GetParentId())
		if err != nil {
			return analyzers.Block{}, fmt.Errorf("diagnostics: parsing parent block id: %w", err)
		}
		p := documentcore.BlockID(rawParent)
		parent = &p
	}
	return analyzers.Block{ID: id, Parent: parent, Kind: kind, Content: content}, nil
}

func (s *Server) AnalyzePage(ctx context.Context, req *diagnosticsv1.AnalyzePageRequest) (*diagnosticsv1.AnalyzePageResponse, error) {
	linkCtx, err := s.buildLinkContext(ctx)
	if err != nil {
		slog.Error("diagnostics: AnalyzePage: building link context", "err", err)
		return nil, status.Error(codes.Internal, "diagnostics: analyze page failed")
	}
	page, err := s.loadPage(ctx, req.GetPageId())
	if err != nil {
		slog.Error("diagnostics: AnalyzePage: loading page", "page_id", req.GetPageId(), "err", err)
		return nil, status.Error(codes.Internal, "diagnostics: analyze page failed")
	}

	diags := analyzers.AnalyzeAll(page, linkCtx)
	out := make([]*diagnosticsv1.Diagnostic, len(diags))
	for i, d := range diags {
		var blockID *string
		if d.BlockID != nil {
			id := d.BlockID.String()
			blockID = &id
		}
		out[i] = &diagnosticsv1.Diagnostic{
			Analyzer: string(d.Analyzer),
			Severity: string(d.Severity),
			Message:  d.Message,
			BlockId:  blockID,
		}
	}
	return &diagnosticsv1.AnalyzePageResponse{Diagnostics: out}, nil
}

// buildFactsGraph loads every page's blocks and runs facts.Build once —
// shared by AnalyzeFacts and StaleReferences so both compute over the
// exact same scan of the workspace, not two slightly different ones.
func (s *Server) buildFactsGraph(ctx context.Context) (facts.Graph, error) {
	pagesResp, err := s.pages.ListPages(s.withActor(ctx), &documentv1.ListPagesRequest{})
	if err != nil {
		return facts.Graph{}, fmt.Errorf("diagnostics: listing pages: %w", err)
	}

	pageBlocks := make([]facts.PageBlocks, 0, len(pagesResp.GetPages()))
	for _, p := range pagesResp.GetPages() {
		blocksResp, err := s.pages.ListBlocks(s.withActor(ctx), &documentv1.ListBlocksRequest{PageId: p.GetId()})
		if err != nil {
			return facts.Graph{}, fmt.Errorf("diagnostics: listing blocks for page %s: %w", p.GetId(), err)
		}
		pb := facts.PageBlocks{PageID: p.GetId()}
		for _, b := range blocksResp.GetBlocks() {
			var content documentcore.Content
			if err := json.Unmarshal([]byte(b.GetContentJson()), &content); err != nil {
				return facts.Graph{}, fmt.Errorf("diagnostics: parsing block content: %w", err)
			}
			rawID, err := uuid.Parse(b.GetId())
			if err != nil {
				return facts.Graph{}, fmt.Errorf("diagnostics: parsing block id: %w", err)
			}
			pb.Blocks = append(pb.Blocks, struct {
				ID   documentcore.BlockID
				Text string
			}{ID: documentcore.BlockID(rawID), Text: content.Text})
		}
		pageBlocks = append(pageBlocks, pb)
	}

	return facts.Build(pageBlocks), nil
}

func (s *Server) AnalyzeFacts(ctx context.Context, _ *diagnosticsv1.AnalyzeFactsRequest) (*diagnosticsv1.AnalyzeFactsResponse, error) {
	g, err := s.buildFactsGraph(ctx)
	if err != nil {
		slog.Error("diagnostics: AnalyzeFacts: building facts graph", "err", err)
		return nil, status.Error(codes.Internal, "diagnostics: analyze facts failed")
	}

	definitions := make([]*diagnosticsv1.Definition, 0, len(g.Definitions))
	for _, d := range g.Definitions {
		definitions = append(definitions, toProtoDefinition(d))
	}
	duplicates := make([]*diagnosticsv1.DuplicateGroup, 0, len(g.Duplicates))
	for name, defs := range g.Duplicates {
		protoDefs := make([]*diagnosticsv1.Definition, len(defs))
		for i, d := range defs {
			protoDefs[i] = toProtoDefinition(d)
		}
		duplicates = append(duplicates, &diagnosticsv1.DuplicateGroup{Name: name, Definitions: protoDefs})
	}
	references := make([]*diagnosticsv1.Reference, len(g.References))
	for i, r := range g.References {
		references[i] = toProtoReference(r)
	}

	return &diagnosticsv1.AnalyzeFactsResponse{
		Definitions: definitions,
		Duplicates:  duplicates,
		Cycle:       g.Cycle,
		References:  references,
	}, nil
}

func (s *Server) StaleReferences(ctx context.Context, req *diagnosticsv1.StaleReferencesRequest) (*diagnosticsv1.StaleReferencesResponse, error) {
	g, err := s.buildFactsGraph(ctx)
	if err != nil {
		slog.Error("diagnostics: StaleReferences: building facts graph", "err", err)
		return nil, status.Error(codes.Internal, "diagnostics: stale references failed")
	}
	refs := g.StaleReferences(req.GetFactName())
	out := make([]*diagnosticsv1.Reference, len(refs))
	for i, r := range refs {
		out[i] = toProtoReference(r)
	}
	return &diagnosticsv1.StaleReferencesResponse{References: out}, nil
}

func toProtoDefinition(d facts.Definition) *diagnosticsv1.Definition {
	return &diagnosticsv1.Definition{Name: d.Name, Value: d.Value, PageId: d.PageID, BlockId: d.BlockID.String()}
}

func toProtoReference(r facts.Reference) *diagnosticsv1.Reference {
	return &diagnosticsv1.Reference{Name: r.Name, PageId: r.PageID, BlockId: r.BlockID.String()}
}
