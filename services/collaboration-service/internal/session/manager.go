package session

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"marginal/collaboration-service/internal/flush"
	"marginal/collaboration-service/internal/opstore"
)

// Manager is the one-Session-per-page registry a transport layer (the
// WebSocket handler, docs/porting/PROGRESS.md "Next") calls Get against
// per incoming connection. Held open indefinitely once opened — see the
// package doc comment on why idle-eviction is deferred, not missing by
// oversight.
type Manager struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]*Session

	repo        opstore.Repo
	walDir      string
	serverActor string
	canApply    CanApplyFunc
	flushOpts   []flush.Option
	logger      *slog.Logger
}

type ManagerOption func(*Manager)

func WithCanApply(f CanApplyFunc) ManagerOption { return func(m *Manager) { m.canApply = f } }
func WithFlushOptions(opts ...flush.Option) ManagerOption {
	return func(m *Manager) { m.flushOpts = opts }
}

// WithLogger sets where a Session logs background failures it can't
// surface to any caller (a flush-enqueue failing, say) — defaults to
// slog.Default() if never set.
func WithLogger(l *slog.Logger) ManagerOption { return func(m *Manager) { m.logger = l } }

// NewManager. serverActor is this collaboration-service instance's own
// identity for Lamport ItemIDs (doctext.New) — not an editing user;
// distinct per running instance (e.g. uuid.NewV7().String(), generated
// once at process startup and passed in, so the caller controls where
// that identity comes from rather than this package inventing one).
// walDir is where each page's local WAL segment lives.
func NewManager(repo opstore.Repo, walDir string, serverActor string, opts ...ManagerOption) *Manager {
	m := &Manager{
		sessions:    make(map[uuid.UUID]*Session),
		repo:        repo,
		walDir:      walDir,
		serverActor: serverActor,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Get returns pageID's live Session, opening (replaying + reconciling) it
// on first access. Held for the whole open, not just the map lookup —
// two concurrent Gets for the same never-yet-opened page must not both
// open it (that would double-claim the same WAL file); this repo's scale
// makes that global-lock cost a non-issue, unlike it would be at
// ARCHITECTURE.md's stated 15k-concurrent-editor scale.
func (m *Manager) Get(ctx context.Context, pageID uuid.UUID) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[pageID]; ok {
		return s, nil
	}
	s, err := open(ctx, pageID, m.repo, m.walDir, m.serverActor, m.canApply, m.flushOpts, m.logger)
	if err != nil {
		return nil, fmt.Errorf("session: manager: opening %s: %w", pageID, err)
	}
	m.sessions[pageID] = s
	return s, nil
}

// OpenSessions counts the pages currently held in memory — § 18
// ADMIN's SESSIONS readout, from this service's side.
//
// This is pages with a live rope, not people: one page with four
// editors is one session here. It is also not a count of people
// signed in, which is auth-service's number and a different one
// again. Three plausible meanings of the word, so the screen
// labels which it is showing.
//
// Note that Manager never evicts (CLAUDE.md's stated demo-scale
// limitation), so this only grows until restart. A rising number
// on an idle instance is that, not load.
func (m *Manager) OpenSessions() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// CloseAll closes every open session — graceful shutdown's job, so
// buffered flush data drains instead of relying solely on what's already
// in each session's WAL segment.
func (m *Manager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for id, s := range m.sessions {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("session: manager: closing %s: %w", id, err)
		}
		delete(m.sessions, id)
	}
	return firstErr
}
