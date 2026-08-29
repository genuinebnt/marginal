package pages

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	documentv1 "marginal/document-service/genproto/documentv1"
)

// Resume — where the caret was, per user per page (v2.8.0,
// DATA_MODEL.md § Reading positions).
//
// This is view state, kept beside the document rather than in it. RFC-001
// §1's rule about view state applies here with a sharper edge than it does to
// toggle collapse: if the caret were model state, moving your cursor would be
// a collaborative edit that moved everyone else's.

const (
	defaultResumeLimit = 6
	maxResumeLimit     = 50
)

func (s *Server) SaveReadingPosition(ctx context.Context, req *documentv1.SaveReadingPositionRequest) (*emptypb.Empty, error) {
	userID, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	pageID, err := parsePageID(req.GetPageId())
	if err != nil {
		return nil, toStatus(err)
	}

	// An absent block id is meaningful: a page opened and scrolled but never
	// clicked into still has a position worth resuming to.
	var blockID *uuid.UUID
	if req.BlockId != nil {
		b, err := uuid.Parse(req.GetBlockId())
		if err != nil {
			return nil, toStatus(ErrInvalidActorID)
		}
		blockID = &b
	}

	if err := s.repo.SaveReadingPosition(ctx, userID, pageID, blockID,
		int(req.GetCaretStart()), int(req.GetCaretEnd())); err != nil {
		return nil, toStatus(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ListReadingPositions(ctx context.Context, req *documentv1.ListReadingPositionsRequest) (*documentv1.ListReadingPositionsResponse, error) {
	userID, err := actorID(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultResumeLimit
	}
	if limit > maxResumeLimit {
		limit = maxResumeLimit
	}

	rows, err := s.repo.ListReadingPositions(ctx, userID, limit)
	if err != nil {
		return nil, toStatus(err)
	}

	out := make([]*documentv1.ReadingPosition, 0, len(rows))
	for _, r := range rows {
		p := &documentv1.ReadingPosition{
			PageId:     r.PageID.String(),
			PageTitle:  r.Title,
			CaretStart: int32(r.CaretStart),
			CaretEnd:   int32(r.CaretEnd),
			UpdatedAt:  timestamppb.New(r.UpdatedAt),
		}
		if r.BlockID != nil {
			b := r.BlockID.String()
			p.BlockId = &b
		}
		if r.Topic != nil {
			p.Topic = toProtoTopic(*r.Topic)
		}
		out = append(out, p)
	}
	return &documentv1.ListReadingPositionsResponse{Positions: out}, nil
}
