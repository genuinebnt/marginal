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
// SpaceReader answers "which spaces may this reader see" from this
// service's own projection (docs.space_members) — an interface at its point
// of use, so this package does not depend on internal/spaceproj and a test
// can hand in a fixed set.
type SpaceReader interface {
	SpacesFor(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	// RoleFor answers the WRITE question. "" means not a member, which is
	// the same answer as an unknown role and must be refused either way.
	RoleFor(ctx context.Context, userID, spaceID uuid.UUID) (string, error)
}

type Server struct {
	documentv1.UnimplementedPageServiceServer
	repo   *PostgresRepo
	spaces SpaceReader
}

func NewServer(repo *PostgresRepo, spaces SpaceReader) *Server {
	return &Server{repo: repo, spaces: spaces}
}

// visible refuses a page outside every space the caller is in.
//
// ListPages filters in SQL, which stops ENUMERATION. It does not stop
// ADDRESSING: a page id is not a secret — it appears in links, in the
// graph, in search results and in URLs — so a scoped list beside an
// unscoped GetPage is not access control, it is a slightly inconvenient
// index. Every RPC that takes a page id asks this first.
//
// NOT_FOUND rather than PERMISSION_DENIED, for the reason
// docs/api/spaces.md §3 gives: a 403 on something you cannot see confirms
// it exists.
//
// It reads docs.space_members — this service's own projection, an indexed
// lookup in its own database rather than a call to auth-service. The
// staleness that buys is stated in ADR-013 and bounded to reads; a write
// still passes can_apply, which resolves from the source of truth at join.
func (s *Server) visible(ctx context.Context, actor uuid.UUID, id PageID) error {
	page, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	spaces, err := s.spaces.SpacesFor(ctx, actor)
	if err != nil {
		return fmt.Errorf("pages: resolving visible spaces: %w", err)
	}
	for _, sp := range spaces {
		if sp == page.SpaceID {
			return nil
		}
	}
	return ErrNotFound
}

// mayWrite refuses a page-lifecycle change from somebody who may read the
// page but not change it.
//
// This exists because ADR-013 §4 said writes go through can_apply — and
// can_apply only sees OPS. Rename, delete and reparent are
// document-service RPCs, not ops, so "writes are gated by can_apply" was
// true of the sentence and false of the system: a viewer could delete any
// page they could see. Found by auditing the entry points rather than by a
// test, which is the uncomfortable part.
//
// NOT_FOUND for a non-member (they must not learn the page exists) and
// PERMISSION_DENIED for a member without the rank (they already know).
func (s *Server) mayWrite(ctx context.Context, actor uuid.UUID, spaceID uuid.UUID) error {
	role, err := s.spaces.RoleFor(ctx, actor, spaceID)
	if err != nil {
		return fmt.Errorf("pages: resolving role: %w", err)
	}
	switch role {
	case "editor", "admin":
		return nil
	case "":
		return ErrNotFound
	default:
		return ErrForbidden
	}
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
	case errors.Is(err, ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
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
		// collaboration-service resolves an actor's role at join by asking
		// which space the page is in, so this has to be on the wire — a
		// second round trip for one column would be a round trip per
		// connection (ADR-013 §3).
		SpaceId: p.SpaceID.String(),
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

	// Which space this page lands in decides who may create it. A child
	// inherits its parent's space (repo.Create enforces that), so the
	// permission question is about the PARENT's space; a root page lands
	// in the default one.
	target := DefaultSpaceID
	if parentID != nil {
		parent, err := s.repo.Get(ctx, *parentID)
		if err != nil {
			return nil, toStatus(err)
		}
		target = parent.SpaceID
	}
	if err := s.mayWrite(ctx, createdBy, target); err != nil {
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
	actor, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	// A page id is not a secret, so scoping the LIST is not enough.
	if err := s.visible(ctx, actor, id); err != nil {
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
	// The actor used to be read only to reject unauthenticated calls and
	// then discarded — there was nothing to scope to. There is now.
	actor, err := actorID(ctx)
	if err != nil {
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

	// Scoped to the caller's spaces, in SQL. Filtering after the query
	// would return fewer rows than the LIMIT asked for, and that shortfall
	// is indistinguishable from "there were no more" (ADR-013 §4).
	visible, err := s.spaces.SpacesFor(ctx, uuid.UUID(actor))
	if err != nil {
		return nil, status.Error(codes.Internal, "pages: resolving visible spaces")
	}

	after := req.GetAfter()
	pagesList, err := s.repo.List(ctx, visible, parentID, after, limit)
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
	actor, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	if err := validateTitle(req.GetTitle()); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	// A page id is not a secret, so scoping the LIST is not enough.
	if err := s.visible(ctx, actor, id); err != nil {
		return nil, toStatus(err)
	}
	// Visible is not the same as writable: a viewer may read this page and
	// must not change it. can_apply cannot cover these — they are RPCs,
	// not ops.
	if page, err := s.repo.Get(ctx, id); err != nil {
		return nil, toStatus(err)
	} else if err := s.mayWrite(ctx, actor, page.SpaceID); err != nil {
		return nil, toStatus(err)
	}
	page, err := s.repo.Rename(ctx, id, req.GetTitle())
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(page), nil
}

func (s *Server) DeletePage(ctx context.Context, req *documentv1.DeletePageRequest) (*emptypb.Empty, error) {
	actor, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	// A page id is not a secret, so scoping the LIST is not enough.
	if err := s.visible(ctx, actor, id); err != nil {
		return nil, toStatus(err)
	}
	// Visible is not the same as writable: a viewer may read this page and
	// must not change it. can_apply cannot cover these — they are RPCs,
	// not ops.
	if page, err := s.repo.Get(ctx, id); err != nil {
		return nil, toStatus(err)
	} else if err := s.mayWrite(ctx, actor, page.SpaceID); err != nil {
		return nil, toStatus(err)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ReparentPage(ctx context.Context, req *documentv1.ReparentPageRequest) (*documentv1.Page, error) {
	actor, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	// A page id is not a secret, so scoping the LIST is not enough.
	if err := s.visible(ctx, actor, id); err != nil {
		return nil, toStatus(err)
	}
	// Visible is not the same as writable: a viewer may read this page and
	// must not change it. can_apply cannot cover these — they are RPCs,
	// not ops.
	if page, err := s.repo.Get(ctx, id); err != nil {
		return nil, toStatus(err)
	} else if err := s.mayWrite(ctx, actor, page.SpaceID); err != nil {
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
	actor, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}
	// A page id is not a secret, so scoping the LIST is not enough.
	if err := s.visible(ctx, actor, id); err != nil {
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
	actor, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}
	// A page id is not a secret, so scoping the LIST is not enough.
	if err := s.visible(ctx, actor, id); err != nil {
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
