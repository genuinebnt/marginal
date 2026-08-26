package session

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"marginal/collaboration-service/internal/oplog"
)

// fakeRepo is a small hand-written in-memory stand-in for opstore.Repo —
// not "mocking infrastructure" (that rule guards integration tests
// against skipping real Postgres); this exercises Session/Manager's own
// logic, which is independent of what's underneath the Repo interface.
// The replay-then-reconcile scenario this package cares about most
// (confirmed ops vs. leftover local WAL) only needs Repo's documented
// behavior, which this fake implements faithfully, including the
// idempotent-on-id semantics real opstore.PostgresRepo has.
type fakeRepo struct {
	mu      sync.Mutex
	byPage  map[uuid.UUID][]oplog.LoggedOp
	seenIDs map[uuid.UUID]struct{}
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byPage: make(map[uuid.UUID][]oplog.LoggedOp), seenIDs: make(map[uuid.UUID]struct{})}
}

func (f *fakeRepo) Append(_ context.Context, l oplog.LoggedOp) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.seenIDs[l.ID]; ok {
		return false, nil
	}
	f.seenIDs[l.ID] = struct{}{}
	f.byPage[l.PageID] = append(f.byPage[l.PageID], l)
	return true, nil
}

func (f *fakeRepo) AppendBatch(_ context.Context, ls []oplog.LoggedOp) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, l := range ls {
		if _, ok := f.seenIDs[l.ID]; ok {
			continue
		}
		f.seenIDs[l.ID] = struct{}{}
		f.byPage[l.PageID] = append(f.byPage[l.PageID], l)
		n++
	}
	return n, nil
}

func (f *fakeRepo) ListForPage(_ context.Context, pageID uuid.UUID) ([]oplog.LoggedOp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]oplog.LoggedOp(nil), f.byPage[pageID]...), nil
}

// recordingSubscriber captures every delivered op and presence event, in
// order — the test double for what a WebSocket connection's write side
// would do.
type recordingSubscriber struct {
	mu        sync.Mutex
	got       []oplog.LoggedOp
	presences []PresenceEvent
	cursors   []CursorEvent
}

func (r *recordingSubscriber) Deliver(res CommitResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, res.Op)
}

func (r *recordingSubscriber) DeliverPresence(e PresenceEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.presences = append(r.presences, e)
}

func (r *recordingSubscriber) DeliverCursor(e CursorEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cursors = append(r.cursors, e)
}

func (r *recordingSubscriber) snapshot() []oplog.LoggedOp {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]oplog.LoggedOp(nil), r.got...)
}

func (r *recordingSubscriber) presenceSnapshot() []PresenceEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]PresenceEvent(nil), r.presences...)
}

func (r *recordingSubscriber) cursorSnapshot() []CursorEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]CursorEvent(nil), r.cursors...)
}
