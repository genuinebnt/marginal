package pages

import (
	"context"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	documentv1 "marginal/document-service/genproto/documentv1"
)

const maxTitleBytes = 500

// Domain-level validation errors for this translation layer — actorID,
// validateTitle, and parsePageID used to construct gRPC status.Error
// values directly, which meant they weren't validating anything, they
// were doing transport-layer work from inside a plain helper function.
// That forced toStatus (meant to be the ONE place a gRPC status gets
// built) to special-case "this error might already be a status error"
// via an errors.As escape hatch, since it could no longer assume every
// error crossing it was a still-untranslated domain error. Returning
// plain errors here and pushing every status.Error construction through
// toStatus removes that escape hatch entirely.
var (
	ErrMissingActorID = errors.New("pages: missing actor-id")
	ErrInvalidActorID = errors.New("pages: invalid actor-id")
	ErrInvalidTitle   = errors.New("pages: invalid title")
)

// InvalidPageIDError reports a page id string that isn't a valid UUID.
type InvalidPageIDError struct{ Value string }

func (e *InvalidPageIDError) Error() string {
	return fmt.Sprintf("pages: invalid id %q", e.Value)
}

// Server implements documentv1.PageServiceServer over a *PostgresRepo —
// the translation layer docs/api/pages.md describes: proto <-> domain
// types, domain errors <-> gRPC status codes. No business logic of its
// own; see PostgresRepo for that.
type Server struct {
	documentv1.UnimplementedPageServiceServer
	repo *PostgresRepo
}

func NewServer(repo *PostgresRepo) *Server { return &Server{repo: repo} }

// actorID reads the caller's identity from gRPC metadata. In the full
// design this is set by api-gateway after RS256 verification
// (docs/api/pages.md § Metadata, not fields) — that gateway doesn't exist
// in this repo's scope yet, so document-service reads it directly as a
// stand-in. created_by is never taken from a request field: a client that
// could name its own author could forge authorship.
func actorID(ctx context.Context) (uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return uuid.UUID{}, ErrMissingActorID
	}
	values := md.Get("actor-id")
	if len(values) == 0 || values[0] == "" {
		return uuid.UUID{}, ErrMissingActorID
	}
	id, err := uuid.Parse(values[0])
	if err != nil {
		return uuid.UUID{}, ErrInvalidActorID
	}
	return id, nil
}

func validateTitle(title string) error {
	if title == "" {
		return fmt.Errorf("%w: must not be empty", ErrInvalidTitle)
	}
	if len(title) > maxTitleBytes {
		return fmt.Errorf("%w: must not exceed %d bytes", ErrInvalidTitle, maxTitleBytes)
	}
	if !utf8.ValidString(title) {
		return fmt.Errorf("%w: must be valid UTF-8", ErrInvalidTitle)
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: must not contain control characters", ErrInvalidTitle)
		}
	}
	return nil
}

func parsePageID(s string) (PageID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return PageID{}, &InvalidPageIDError{Value: s}
	}
	return PageID(id), nil
}

func parseOptionalPageID(s *string) (*PageID, error) {
	if s == nil {
		return nil, nil
	}
	id, err := parsePageID(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// toStatus is the only place in this package that constructs a gRPC
// status error — every domain/validation error (PostgresRepo's, and
// actorID/validateTitle/parsePageID's own) passes through here exactly
// once docs/api/pages.md § Status codes specifies. Anything unrecognized
// is INTERNAL with no detail — the cause belongs in the log inside the
// request span, per that same section.
func toStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, "page not found")
	case errors.Is(err, ErrAnchorMismatch):
		return status.Error(codes.FailedPrecondition, "anchor is not a child of the named parent")
	case errors.Is(err, ErrCycle):
		return status.Error(codes.FailedPrecondition, "cannot reparent a page under itself or its own descendant")
	case errors.Is(err, ErrMissingActorID), errors.Is(err, ErrInvalidActorID):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, ErrInvalidTitle):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		var invalidID *InvalidPageIDError
		if errors.As(err, &invalidID) {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		return status.Error(codes.Internal, "internal error")
	}
}

func toProto(p Page) *documentv1.Page {
	out := &documentv1.Page{
		Id:             p.ID.String(),
		CreatedBy:      p.CreatedBy.String(),
		Title:          p.Title,
		Path:           p.Path,
		SortKey:        p.SortKey,
		LifecycleState: toProtoLifecycle(p.LifecycleState),
		CreatedAt:      timestamppb.New(p.CreatedAt),
		UpdatedAt:      timestamppb.New(p.UpdatedAt),
	}
	if p.ParentID != nil {
		id := p.ParentID.String()
		out.ParentId = &id
	}
	if p.DeletedAt != nil {
		out.DeletedAt = timestamppb.New(*p.DeletedAt)
	}
	return out
}

func toProtoLifecycle(s LifecycleState) documentv1.LifecycleState {
	switch s {
	case Active:
		return documentv1.LifecycleState_LIFECYCLE_STATE_ACTIVE
	case Deleting:
		return documentv1.LifecycleState_LIFECYCLE_STATE_DELETING
	case Deleted:
		return documentv1.LifecycleState_LIFECYCLE_STATE_DELETED
	default:
		return documentv1.LifecycleState_LIFECYCLE_STATE_UNSPECIFIED
	}
}

func (s *Server) CreatePage(ctx context.Context, req *documentv1.CreatePageRequest) (*documentv1.Page, error) {
	if err := validateTitle(req.GetTitle()); err != nil {
		return nil, toStatus(err)
	}
	createdBy, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	parentID, err := parseOptionalPageID(req.ParentId)
	if err != nil {
		return nil, toStatus(err)
	}
	after, err := parseOptionalPageID(req.After)
	if err != nil {
		return nil, toStatus(err)
	}

	page, err := s.repo.Create(ctx, NewPage{
		CreatedBy: createdBy,
		Title:     req.GetTitle(),
		ParentID:  parentID,
		After:     after,
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(page), nil
}

func (s *Server) GetPage(ctx context.Context, req *documentv1.GetPageRequest) (*documentv1.Page, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	page, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	// Classification travels with every page read rather than needing a
	// second round trip: a topic chip is drawn wherever a page title is,
	// so a caller that has the page but not its topic has an incomplete
	// page (v2.7.0, ui-mockups § 10b).
	out := toProto(page)
	if err := s.attachClassification(ctx, out, id); err != nil {
		return nil, toStatus(err)
	}
	return out, nil
}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

func (s *Server) ListPages(ctx context.Context, req *documentv1.ListPagesRequest) (*documentv1.ListPagesResponse, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	parentID, err := parseOptionalPageID(req.ParentId)
	if err != nil {
		return nil, toStatus(err)
	}

	limit := int32(defaultListLimit)
	if req.Limit != nil {
		limit = req.GetLimit()
		if limit <= 0 || limit > maxListLimit {
			return nil, status.Errorf(codes.InvalidArgument, "limit must be between 1 and %d", maxListLimit)
		}
	}

	after := req.GetAfter()
	pagesList, err := s.repo.List(ctx, parentID, after, limit)
	if err != nil {
		return nil, toStatus(err)
	}

	resp := &documentv1.ListPagesResponse{Pages: make([]*documentv1.Page, len(pagesList))}
	// Classification travels with the list for the same reason it travels
	// with GetPage: a topic chip is drawn wherever a page title is. Batched
	// into two queries for the whole page rather than two per row.
	ids := make([]PageID, 0, len(pagesList))
	for _, p := range pagesList {
		ids = append(ids, p.ID)
	}
	topics, tags, err := s.repo.ClassificationFor(ctx, ids)
	if err != nil {
		return nil, toStatus(err)
	}
	stats, err := s.repo.StatsFor(ctx, ids)
	if err != nil {
		return nil, toStatus(err)
	}
	for i, p := range pagesList {
		out := toProto(p)
		if t, ok := topics[p.ID]; ok {
			out.Topic = toProtoTopic(t)
		}
		if g, ok := tags[p.ID]; ok {
			out.Tags = g
		} else {
			out.Tags = []string{}
		}
		if st, ok := stats[p.ID]; ok {
			out.BlockCount, out.WordCount = st.Blocks, st.Words
		}
		resp.Pages[i] = out
	}
	if int32(len(pagesList)) == limit {
		cursor := pagesList[len(pagesList)-1].SortKey
		resp.NextCursor = &cursor
	}
	return resp, nil
}

func (s *Server) RenamePage(ctx context.Context, req *documentv1.RenamePageRequest) (*documentv1.Page, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	if err := validateTitle(req.GetTitle()); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	page, err := s.repo.Rename(ctx, id, req.GetTitle())
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(page), nil
}

func (s *Server) DeletePage(ctx context.Context, req *documentv1.DeletePageRequest) (*emptypb.Empty, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ReparentPage(ctx context.Context, req *documentv1.ReparentPageRequest) (*documentv1.Page, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}

	var parent ParentChange
	if req.ParentId != nil {
		parent.Change = true
		if *req.ParentId != "" {
			parentID, err := parsePageID(*req.ParentId)
			if err != nil {
				return nil, toStatus(err)
			}
			parent.ParentID = &parentID
		}
		// *req.ParentId == "" leaves parent.ParentID nil: promote to root.
	}

	after, err := parseOptionalPageID(req.After)
	if err != nil {
		return nil, toStatus(err)
	}

	page, err := s.repo.Reparent(ctx, id, parent, after)
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(page), nil
}

// ListBacklinks confirms the target page exists (and isn't soft-deleted)
// via PostgresRepo.Get before reading docs.page_links. No ownership check —
// pages carry no access control on this instance (PostgresRepo's own doc
// comment).
func (s *Server) ListBacklinks(ctx context.Context, req *documentv1.ListBacklinksRequest) (*documentv1.ListBacklinksResponse, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, toStatus(err)
	}

	links, err := s.repo.ListBacklinks(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*documentv1.Backlink, len(links))
	for i, l := range links {
		out[i] = &documentv1.Backlink{
			FromPage:        l.FromPage.String(),
			FromPageTitle:   l.FromPageTitle,
			FromPageDeleted: l.FromPageDeleted,
			TargetTitle:     l.TargetTitle,
		}
	}
	return &documentv1.ListBacklinksResponse{Backlinks: out}, nil
}

// ListBlocks is ListBacklinks' own twin for docs.blocks — same existence
// check, same "no ownership check on this instance" reasoning.
func (s *Server) ListBlocks(ctx context.Context, req *documentv1.ListBlocksRequest) (*documentv1.ListBlocksResponse, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, toStatus(err)
	}

	blocks, err := s.repo.ListBlocks(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*documentv1.Block, len(blocks))
	for i, b := range blocks {
		var parentID *string
		if b.ParentID != nil {
			s := b.ParentID.String()
			parentID = &s
		}
		out[i] = &documentv1.Block{
			Id:          b.ID.String(),
			ParentId:    parentID,
			KindJson:    string(b.KindJSON),
			ContentJson: string(b.ContentJSON),
		}
	}
	return &documentv1.ListBlocksResponse{Blocks: out}, nil
}

func parseTopicID(s string) (TopicID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return TopicID{}, status.Error(codes.InvalidArgument, "topic id must be a UUID")
	}
	return TopicID(id), nil
}
