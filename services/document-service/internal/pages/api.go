package pages

import (
	"context"
	"errors"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	documentv1 "marginal/document-service/internal/genproto/documentv1"
)

const maxTitleBytes = 500

// Server implements documentv1.PageServiceServer over a Repo — the
// translation layer docs/api/pages.md describes: proto <-> domain types,
// domain errors <-> gRPC status codes. No business logic of its own; see
// Repo for that.
type Server struct {
	documentv1.UnimplementedPageServiceServer
	Repo Repo
}

// actorID reads the caller's identity from gRPC metadata. In the full
// design this is set by api-gateway after RS256 verification
// (docs/api/pages.md § Metadata, not fields) — that gateway doesn't exist
// in this repo's scope yet, so document-service reads it directly as a
// stand-in. created_by is never taken from a request field: a client that
// could name its own author could forge authorship.
func actorID(ctx context.Context) (uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return uuid.UUID{}, status.Error(codes.Unauthenticated, "missing actor-id")
	}
	values := md.Get("actor-id")
	if len(values) == 0 || values[0] == "" {
		return uuid.UUID{}, status.Error(codes.Unauthenticated, "missing actor-id")
	}
	id, err := uuid.Parse(values[0])
	if err != nil {
		return uuid.UUID{}, status.Error(codes.Unauthenticated, "invalid actor-id")
	}
	return id, nil
}

func validateTitle(title string) error {
	if title == "" {
		return status.Error(codes.InvalidArgument, "title must not be empty")
	}
	if len(title) > maxTitleBytes {
		return status.Errorf(codes.InvalidArgument, "title must not exceed %d bytes", maxTitleBytes)
	}
	if !utf8.ValidString(title) {
		return status.Error(codes.InvalidArgument, "title must be valid UTF-8")
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return status.Error(codes.InvalidArgument, "title must not contain control characters")
		}
	}
	return nil
}

func parsePageID(s string) (PageID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return PageID{}, status.Errorf(codes.InvalidArgument, "invalid id %q", s)
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

// toStatus translates a Repo error into the gRPC status docs/api/pages.md
// § Status codes specifies. Anything unrecognized is INTERNAL with no
// detail — the cause belongs in the log inside the request span, per that
// same section.
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
	default:
		var st interface{ GRPCStatus() *status.Status }
		if errors.As(err, &st) {
			return err // already a status error (validation, actorID, ...)
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
		return nil, err
	}
	createdBy, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	parentID, err := parseOptionalPageID(req.ParentId)
	if err != nil {
		return nil, err
	}
	after, err := parseOptionalPageID(req.After)
	if err != nil {
		return nil, err
	}

	page, err := s.Repo.Create(ctx, NewPage{
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
	owner, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, err
	}
	page, err := s.Repo.Get(ctx, owner, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(page), nil
}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

func (s *Server) ListPages(ctx context.Context, req *documentv1.ListPagesRequest) (*documentv1.ListPagesResponse, error) {
	owner, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	parentID, err := parseOptionalPageID(req.ParentId)
	if err != nil {
		return nil, err
	}

	limit := int32(defaultListLimit)
	if req.Limit != nil {
		limit = req.GetLimit()
		if limit <= 0 || limit > maxListLimit {
			return nil, status.Errorf(codes.InvalidArgument, "limit must be between 1 and %d", maxListLimit)
		}
	}

	after := req.GetAfter()
	pagesList, err := s.Repo.List(ctx, owner, parentID, after, limit)
	if err != nil {
		return nil, toStatus(err)
	}

	resp := &documentv1.ListPagesResponse{Pages: make([]*documentv1.Page, len(pagesList))}
	for i, p := range pagesList {
		resp.Pages[i] = toProto(p)
	}
	if int32(len(pagesList)) == limit {
		cursor := pagesList[len(pagesList)-1].SortKey
		resp.NextCursor = &cursor
	}
	return resp, nil
}

func (s *Server) RenamePage(ctx context.Context, req *documentv1.RenamePageRequest) (*documentv1.Page, error) {
	owner, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateTitle(req.GetTitle()); err != nil {
		return nil, err
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, err
	}
	page, err := s.Repo.Rename(ctx, owner, id, req.GetTitle())
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(page), nil
}

func (s *Server) DeletePage(ctx context.Context, req *documentv1.DeletePageRequest) (*emptypb.Empty, error) {
	owner, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.Repo.Delete(ctx, owner, id); err != nil {
		return nil, toStatus(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ReparentPage(ctx context.Context, req *documentv1.ReparentPageRequest) (*documentv1.Page, error) {
	owner, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, err
	}

	var parent ParentChange
	if req.ParentId != nil {
		parent.Change = true
		if *req.ParentId != "" {
			parentID, err := parsePageID(*req.ParentId)
			if err != nil {
				return nil, err
			}
			parent.ParentID = &parentID
		}
		// *req.ParentId == "" leaves parent.ParentID nil: promote to root.
	}

	after, err := parseOptionalPageID(req.After)
	if err != nil {
		return nil, err
	}

	page, err := s.Repo.Reparent(ctx, owner, id, parent, after)
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(page), nil
}
