package pages

import (
	"context"

	documentv1 "marginal/document-service/genproto/documentv1"
)

// A SERIES IS A PAGE WITH CHILDREN.
//
// No table, no `series_name` column, no second ordering. The page tree
// already expresses "these pages belong together, in this order", and
// sort_key already orders them — so drag-to-reorder in the rail reorders the
// series, for free and by construction. A parallel grouping would mean two
// answers to "what comes after this" that can disagree, and only one of them
// would be updated by a drag.
//
// The accepted consequence: a page cannot be in two series. That is the tree's
// own constraint, and the same one a topic has — a thing that belongs to two
// orderings has no next.

// SeriesChild is one entry as the repo reads it.
type SeriesChild struct {
	ID    PageID
	Title string
}

// GetPageSeries answers "what series is this page in", in three states that
// need different words on screen: it is a part, it IS the series, or neither.
func (s *Server) GetPageSeries(ctx context.Context, req *documentv1.GetPageSeriesRequest) (*documentv1.PageSeries, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}
	page, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}

	// LEADER first: a page that has children IS a series, even when it also
	// has a parent (a sub-series is still a series). Checking membership first
	// would report a nested series page as merely a part of its own parent,
	// which is the less useful of the two true answers.
	children, err := s.repo.ChildrenOrdered(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	if len(children) > 0 {
		parts, err := s.decorate(ctx, children)
		if err != nil {
			return nil, toStatus(err)
		}
		return &documentv1.PageSeries{
			Membership:   documentv1.Membership_MEMBERSHIP_LEADER,
			SeriesPageId: page.ID.String(),
			SeriesTitle:  page.Title,
			Parts:        parts,
		}, nil
	}

	if page.ParentID == nil {
		return &documentv1.PageSeries{Membership: documentv1.Membership_MEMBERSHIP_NONE}, nil
	}

	parent, err := s.repo.Get(ctx, *page.ParentID)
	if err != nil {
		return nil, toStatus(err)
	}
	siblings, err := s.repo.ChildrenOrdered(ctx, *page.ParentID)
	if err != nil {
		return nil, toStatus(err)
	}
	// A single child is not a series. One part is a sub-page, and calling it
	// "Part 1 of 1" is a banner that tells the reader nothing.
	if len(siblings) < 2 {
		return &documentv1.PageSeries{Membership: documentv1.Membership_MEMBERSHIP_NONE}, nil
	}
	parts, err := s.decorate(ctx, siblings)
	if err != nil {
		return nil, toStatus(err)
	}
	number := int32(0)
	for _, p := range parts {
		if p.GetPageId() == page.ID.String() {
			number = p.GetNumber()
		}
	}
	return &documentv1.PageSeries{
		Membership:   documentv1.Membership_MEMBERSHIP_MEMBER,
		SeriesPageId: parent.ID.String(),
		SeriesTitle:  parent.Title,
		Parts:        parts,
		Number:       number,
	}, nil
}

// ListSeries is every page that has children — the series index.
func (s *Server) ListSeries(ctx context.Context, _ *documentv1.ListSeriesRequest) (*documentv1.ListSeriesResponse, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	roots, err := s.repo.SeriesRoots(ctx)
	if err != nil {
		return nil, toStatus(err)
	}

	out := &documentv1.ListSeriesResponse{Series: make([]*documentv1.SeriesSummary, 0, len(roots))}
	for _, root := range roots {
		children, err := s.repo.ChildrenOrdered(ctx, root.ID)
		if err != nil {
			return nil, toStatus(err)
		}
		// A single child is not a series — the same rule GetPageSeries
		// applies, in one place would be better but the two questions load
		// different rows; the constant is what has to agree, and it is 2.
		if len(children) < 2 {
			continue
		}
		parts, err := s.decorate(ctx, children)
		if err != nil {
			return nil, toStatus(err)
		}

		ids := []PageID{root.ID}
		for _, c := range children {
			ids = append(ids, c.ID)
		}
		stats, err := s.repo.StatsFor(ctx, ids)
		if err != nil {
			return nil, toStatus(err)
		}
		total := int32(0)
		for _, st := range stats {
			total += st.Words
		}

		summary := &documentv1.SeriesSummary{
			SeriesPageId: root.ID.String(),
			Title:        root.Title,
			PartCount:    int32(len(parts)),
			WordCount:    total,
			Parts:        parts,
		}
		// The series takes the SERIES PAGE's topic, not a vote over its
		// parts: the hub declares what the series is about, and a majority
		// over parts would let one outlier part rename the whole thing.
		topics, _, err := s.repo.ClassificationFor(ctx, []PageID{root.ID})
		if err != nil {
			return nil, toStatus(err)
		}
		if t, ok := topics[root.ID]; ok {
			summary.Topic = toProtoTopic(t)
		}
		out.Series = append(out.Series, summary)
	}
	return out, nil
}

// decorate turns ordered children into parts, numbered from 1 and carrying
// the classification and size a card needs. One batched query for the whole
// list, not one per part.
func (s *Server) decorate(ctx context.Context, children []SeriesChild) ([]*documentv1.SeriesPart, error) {
	ids := make([]PageID, len(children))
	for i, c := range children {
		ids[i] = c.ID
	}
	topics, tags, err := s.repo.ClassificationFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	stats, err := s.repo.StatsFor(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]*documentv1.SeriesPart, len(children))
	for i, c := range children {
		// 1-based on purpose: 0 is never a valid part number, so an unset
		// field is visibly wrong rather than plausibly first.
		p := &documentv1.SeriesPart{PageId: c.ID.String(), Title: c.Title, Number: int32(i + 1)}
		if t, ok := topics[c.ID]; ok {
			p.Topic = toProtoTopic(t)
		}
		if g, ok := tags[c.ID]; ok {
			p.Tags = g
		} else {
			p.Tags = []string{}
		}
		if st, ok := stats[c.ID]; ok {
			p.WordCount = st.Words
		}
		out[i] = p
	}
	return out, nil
}
