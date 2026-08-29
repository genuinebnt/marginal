package pages

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	documentv1 "marginal/document-service/genproto/documentv1"
	"marginal/document-service/internal/pagesaga"
)

// PurgeWindow is how long a deleted page stays restorable.
//
// Derived at read time from deleted_at rather than stored per row: changing
// the window must move every pending purge, and a stored purge_at would need
// a backfill to do that (DATA_MODEL.md § Page deletions).
const PurgeWindow = 30 * 24 * time.Hour

// PreviewDelete answers "what would this take with it" BEFORE it takes it.
//
// § 23c's whole argument is that deleting is a state rather than an event,
// and the first half of that is being able to see the state's cost before
// entering it. The referrer list is the part that matters: those links do not
// break, they DANGLE — a real state the graph already models and the
// diagnostics engine already reports.
func (s *Server) PreviewDelete(ctx context.Context, req *documentv1.PreviewDeleteRequest) (*documentv1.PreviewDeleteResponse, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, toStatus(err)
	}

	preview, err := s.repo.PreviewDelete(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &documentv1.PreviewDeleteResponse{
		BlockCount:  preview.Blocks,
		Descendants: make([]*documentv1.Page, 0, len(preview.Descendants)),
		Referrers:   make([]*documentv1.Page, 0, len(preview.Referrers)),
	}
	for _, p := range preview.Descendants {
		out.Descendants = append(out.Descendants, toProto(p))
	}
	for _, p := range preview.Referrers {
		out.Referrers = append(out.Referrers, toProto(p))
	}
	return out, nil
}

// ListTrash is the delete saga, visible.
func (s *Server) ListTrash(ctx context.Context, req *documentv1.ListTrashRequest) (*documentv1.ListTrashResponse, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	limit := req.GetLimit()
	if limit <= 0 || limit > maxListLimit {
		limit = defaultListLimit
	}

	entries, total, err := s.repo.ListTrash(ctx, PurgeWindow, limit, req.GetOffset())
	if err != nil {
		return nil, toStatus(err)
	}

	out := &documentv1.ListTrashResponse{
		Total:   total,
		Entries: make([]*documentv1.TrashEntry, 0, len(entries)),
	}
	for _, e := range entries {
		entry := &documentv1.TrashEntry{
			Page:    toProto(e.Page),
			PurgeAt: timestamppb.New(e.PurgeAt),
		}
		if e.Progress != nil {
			// steps_left comes from pagesaga.Steps, not from the database:
			// that slice is the authority on what "finished" means, which is
			// why appending a step reopens every completed saga.
			p := &documentv1.SagaProgress{
				StepsDone: e.Progress.StepsDone,
				StepsLeft: pagesaga.Remaining(e.Progress.StepsDone),
				Attempts:  e.Progress.Attempts,
			}
			if e.Progress.LastError != "" {
				msg := e.Progress.LastError
				p.LastError = &msg
			}
			// Steps with no backing store at this repo's scope are reported
			// as such rather than rendered as work performed or silently
			// dropped — the same honesty the mockup applies to undrawn routes.
			for _, step := range pagesaga.Steps {
				if pagesaga.NotApplicable(step) {
					p.NotApplicable = append(p.NotApplicable, step)
				}
			}
			entry.Progress = p
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}

// RestorePage undoes the first saga step and nothing else — the rest is
// re-derived. page_links and the FTS index are projections that rebuild from
// the op log, and the op log was sealed, never deleted.
func (s *Server) RestorePage(ctx context.Context, req *documentv1.RestorePageRequest) (*documentv1.Page, error) {
	if _, err := actorID(ctx); err != nil {
		return nil, toStatus(err)
	}
	id, err := parsePageID(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}

	n, err := s.repo.Restore(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	if n == 0 {
		// Either it is not in the trash, or the saga has already purged past
		// the point of return. FAILED_PRECONDITION rather than NOT_FOUND:
		// the page may well exist, and saying "not found" would be a
		// different and wrong story.
		return nil, status.Error(codes.FailedPrecondition,
			"pages: not restorable — already active, already purged, or mid-saga")
	}

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
