package pages

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	pagerepo "marginal/document-service/internal/pagerepo/gen"
)

// Repository half of v2.7.0's topics and tags. Split from repo.go the same
// way classify.go is split from api.go — nothing here touches the LTREE
// tree, sort keys, or the delete saga.

func (r *PostgresRepo) ListTopics(ctx context.Context) ([]Topic, int, error) {
	rows, err := r.q.ListTopics(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("pages: list topics: %w", err)
	}
	out := make([]Topic, 0, len(rows))
	for _, row := range rows {
		out = append(out, Topic{
			ID:        TopicID(fromPgUUID(row.ID)),
			Name:      row.Name,
			ColorKey:  row.ColorKey,
			PageCount: int(row.PageCount),
		})
	}
	untopiced, err := r.q.CountUntopicedPages(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("pages: counting untopiced: %w", err)
	}
	return out, int(untopiced), nil
}

// SetTopic assigns or (with a nil topicID) clears a page's owned topic.
// A page that doesn't exist or is deleted affects zero rows and returns
// ErrNotFound — assigning a topic to a page in the trash is a caller bug,
// not something to silently accept and lose on restore.
func (r *PostgresRepo) SetTopic(ctx context.Context, id PageID, topicID *TopicID) error {
	// A zero pgtype.UUID with Valid:false is SQL NULL, which is exactly the
	// "clear the assignment" case — so nil needs no separate query.
	var pg pgtype.UUID
	if topicID != nil {
		pg = toPgUUID(uuid.UUID(*topicID))
	}
	n, err := r.q.SetPageTopic(ctx, pagerepo.SetPageTopicParams{
		PageID:  toPgUUID(uuid.UUID(id)),
		TopicID: pg,
	})
	if err != nil {
		return fmt.Errorf("pages: set topic: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// PageTopic returns the page's topic, or nil when it is untopiced — a real
// state, so absence is not an error.
func (r *PostgresRepo) PageTopic(ctx context.Context, id PageID) (*Topic, error) {
	var (
		tid      uuid.UUID
		name     string
		colorKey string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT t.id, t.name, t.color_key
		FROM docs.pages p JOIN docs.topics t ON t.id = p.topic_id
		WHERE p.id = $1`, toPgUUID(uuid.UUID(id))).Scan(&tid, &name, &colorKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pages: page topic: %w", err)
	}
	return &Topic{ID: TopicID(tid), Name: name, ColorKey: colorKey}, nil
}

// PageTags returns the page's tags, always non-nil so the wire carries an
// empty list rather than a null the frontend has to special-case.
func (r *PostgresRepo) PageTags(ctx context.Context, id PageID) ([]string, error) {
	tags, err := r.q.ListPageTags(ctx, toPgUUID(uuid.UUID(id)))
	if err != nil {
		return nil, fmt.Errorf("pages: page tags: %w", err)
	}
	if tags == nil {
		return []string{}, nil
	}
	return tags, nil
}

func (r *PostgresRepo) AddTag(ctx context.Context, id PageID, tag string) error {
	if err := r.q.AddPageTag(ctx, pagerepo.AddPageTagParams{
		PageID: toPgUUID(uuid.UUID(id)), Tag: tag,
	}); err != nil {
		return fmt.Errorf("pages: add tag: %w", err)
	}
	return nil
}

// RemoveTag is idempotent — removing a tag the page never carried satisfies
// the caller's intent already, so zero rows is success, not ErrNotFound.
func (r *PostgresRepo) RemoveTag(ctx context.Context, id PageID, tag string) error {
	if _, err := r.q.RemovePageTag(ctx, pagerepo.RemovePageTagParams{
		PageID: toPgUUID(uuid.UUID(id)), Tag: tag,
	}); err != nil {
		return fmt.Errorf("pages: remove tag: %w", err)
	}
	return nil
}

func (r *PostgresRepo) ListTagFacets(ctx context.Context, limit int) ([]TagFacet, error) {
	rows, err := r.q.ListTagFacets(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("pages: tag facets: %w", err)
	}
	out := make([]TagFacet, 0, len(rows))
	for _, row := range rows {
		out = append(out, TagFacet{
			Tag:           row.Tag,
			PageCount:     int(row.PageCount),
			TopicsSpanned: int(row.TopicsSpanned),
		})
	}
	return out, nil
}
