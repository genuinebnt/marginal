package pages

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	documentv1 "marginal/document-service/genproto/documentv1"
)

// Topics and tags — RFC-free, product-level classification (v2.7.0,
// ui-mockups § 10b TOPICS & TAGS). Kept in its own file rather than added to
// api.go because it is a different noun from the page tree: nothing here
// touches path, sort_key, or the op log.
//
// The distinction the whole feature turns on:
//
//	A TOPIC is singular, owned, indexed — it clusters the graph and scopes
//	similarity search. A TAG is free-form and many — it facets search and
//	never boosts rank.
//
// Collapsing them gives you folders, and a page genuinely about two things
// then has to lie about one of them.

const (
	// Long enough for a real technique name ("consistent-hashing"), short
	// enough that a tag stays a label rather than becoming a sentence. The
	// database CHECK enforces the same bound — this exists so the caller
	// gets a useful error instead of a constraint violation.
	maxTagLen = 40

	defaultFacetLimit = 40
	maxFacetLimit     = 200
)

func (s *Server) ListTopics(ctx context.Context, _ *documentv1.ListTopicsRequest) (*documentv1.ListTopicsResponse, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	topics, untopiced, err := s.repo.ListTopics(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*documentv1.Topic, 0, len(topics))
	for _, t := range topics {
		out = append(out, toProtoTopic(t))
	}
	// untopiced_pages is returned alongside rather than left to the caller to
	// derive: "62 untopiced" is a headline the UI shows next to the list
	// (ui-mockups § 10b's status line), and deriving it client-side would
	// need every page, not every topic.
	return &documentv1.ListTopicsResponse{Topics: out, UntopicedPages: int32(untopiced)}, nil
}

func (s *Server) SetPageTopic(ctx context.Context, req *documentv1.SetPageTopicRequest) (*documentv1.Page, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	pageID, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}
	// Absent topic_id clears the assignment. Untopiced is a real state the UI
	// reports and offers to fix — not an error, and not something to reject
	// in favour of forcing a guess.
	var topicID *TopicID
	if req.TopicId != nil {
		t, err := parseTopicID(req.GetTopicId())
		if err != nil {
			return nil, toStatus(err)
		}
		topicID = &t
	}
	if err := s.repo.SetTopic(ctx, pageID, topicID); err != nil {
		return nil, toStatus(err)
	}
	return s.pageWithClassification(ctx, pageID)
}

func (s *Server) AddPageTag(ctx context.Context, req *documentv1.AddPageTagRequest) (*documentv1.Page, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	pageID, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}
	tag, err := normalizeTag(req.GetTag())
	if err != nil {
		return nil, err
	}
	if err := s.repo.AddTag(ctx, pageID, tag); err != nil {
		return nil, toStatus(err)
	}
	return s.pageWithClassification(ctx, pageID)
}

func (s *Server) RemovePageTag(ctx context.Context, req *documentv1.RemovePageTagRequest) (*documentv1.Page, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	pageID, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}
	tag, err := normalizeTag(req.GetTag())
	if err != nil {
		return nil, err
	}
	// Removing a tag that isn't there is a no-op, not a 404 — the caller's
	// intent ("this page should not carry that tag") is already satisfied.
	if err := s.repo.RemoveTag(ctx, pageID, tag); err != nil {
		return nil, toStatus(err)
	}
	return s.pageWithClassification(ctx, pageID)
}

func (s *Server) ListTagFacets(ctx context.Context, req *documentv1.ListTagFacetsRequest) (*documentv1.ListTagFacetsResponse, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultFacetLimit
	}
	if limit > maxFacetLimit {
		limit = maxFacetLimit
	}
	facets, err := s.repo.ListTagFacets(ctx, limit)
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*documentv1.TagFacet, 0, len(facets))
	for _, f := range facets {
		out = append(out, &documentv1.TagFacet{
			Tag: f.Tag, PageCount: int32(f.PageCount), TopicsSpanned: int32(f.TopicsSpanned),
		})
	}
	return &documentv1.ListTagFacetsResponse{Facets: out}, nil
}

// pageWithClassification re-reads the page so every mutation returns the
// same fully-populated shape a GET does. Returning the pre-write value with
// the change patched in locally would drift the moment anything else on the
// row changes.
func (s *Server) pageWithClassification(ctx context.Context, id PageID) (*documentv1.Page, error) {
	page, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	out := toProto(page)
	if err := s.attachClassification(ctx, out, id); err != nil {
		return nil, toStatus(err)
	}
	return out, nil
}

func (s *Server) attachClassification(ctx context.Context, out *documentv1.Page, id PageID) error {
	topic, err := s.repo.PageTopic(ctx, id)
	if err != nil {
		return err
	}
	if topic != nil {
		out.Topic = toProtoTopic(*topic)
	}
	tags, err := s.repo.PageTags(ctx, id)
	if err != nil {
		return err
	}
	out.Tags = tags
	return nil
}

func toProtoTopic(t Topic) *documentv1.Topic {
	return &documentv1.Topic{
		Id: t.ID.String(), Name: t.Name, ColorKey: t.ColorKey, PageCount: int32(t.PageCount),
	}
}

// normalizeTag lowercases and trims before validating, so a caller who typed
// " CRDT " gets the tag they meant rather than an error. The database CHECK
// requires lowercase; enforcing it by rejecting would push a formatting rule
// onto every client for no benefit.
func normalizeTag(raw string) (string, error) {
	tag := strings.ToLower(strings.TrimSpace(raw))
	if tag == "" {
		return "", status.Error(codes.InvalidArgument, "tag must not be empty")
	}
	if len(tag) > maxTagLen {
		return "", status.Errorf(codes.InvalidArgument, "tag must be at most %d characters", maxTagLen)
	}
	// A tag with whitespace inside is almost always two tags typed as one,
	// and silently accepting it makes the facet list unusable.
	if strings.ContainsAny(tag, " \t\n") {
		return "", status.Error(codes.InvalidArgument, "tag must not contain whitespace — use a hyphen")
	}
	return tag, nil
}
