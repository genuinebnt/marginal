package pages

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	pagerepo "marginal/document-service/internal/pagerepo/gen"
)

// ReadingPosition is one resume entry, already joined to its page so a caller
// has somewhere to go rather than just an id.
type ReadingPosition struct {
	PageID     PageID
	Title      string
	BlockID    *uuid.UUID
	CaretStart int
	CaretEnd   int
	UpdatedAt  time.Time
	Topic      *Topic
}

func (r *PostgresRepo) SaveReadingPosition(ctx context.Context, userID uuid.UUID, pageID PageID, blockID *uuid.UUID, start, end int) error {
	var pgBlock pgtype.UUID
	if blockID != nil {
		pgBlock = toPgUUID(*blockID)
	}
	if err := r.q.SaveReadingPosition(ctx, pagerepo.SaveReadingPositionParams{
		UserID:     toPgUUID(userID),
		PageID:     toPgUUID(uuid.UUID(pageID)),
		BlockID:    pgBlock,
		CaretStart: int32(start),
		CaretEnd:   int32(end),
	}); err != nil {
		return fmt.Errorf("pages: save reading position: %w", err)
	}
	return nil
}

func (r *PostgresRepo) ListReadingPositions(ctx context.Context, userID uuid.UUID, limit int) ([]ReadingPosition, error) {
	rows, err := r.q.ListReadingPositions(ctx, pagerepo.ListReadingPositionsParams{
		UserID: toPgUUID(userID), Limit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("pages: list reading positions: %w", err)
	}

	// Topics are looked up in one batch rather than per row — the resume list
	// is short, but the N+1 habit is what makes a list endpoint slow later.
	ids := make([]PageID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, PageID(fromPgUUID(row.PageID)))
	}
	topics, _, err := r.ClassificationFor(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]ReadingPosition, 0, len(rows))
	for _, row := range rows {
		id := PageID(fromPgUUID(row.PageID))
		p := ReadingPosition{
			PageID:     id,
			Title:      row.Title,
			CaretStart: int(row.CaretStart),
			CaretEnd:   int(row.CaretEnd),
			UpdatedAt:  row.UpdatedAt.Time,
		}
		if row.BlockID.Valid {
			b := fromPgUUID(row.BlockID)
			p.BlockID = &b
		}
		if t, ok := topics[id]; ok {
			p.Topic = &t
		}
		out = append(out, p)
	}
	return out, nil
}
