// Package roles holds each live connection's role, resolved once when it
// joined, and answers the one question can_apply asks: may this actor
// write to this page (ADR-013 §3).
//
// # WHY A DIRECTORY AND NOT A LOOKUP PER OP
//
// can_apply runs on EVERY op — every keystroke — in a session that keeps a
// rope in memory precisely to avoid per-keystroke I/O. Resolving a role
// there would put a database round trip, or a gRPC call, inside the hot
// path of typing. So the role is resolved once at join and read from here,
// and can_apply stays the pure, synchronous, auditable function RFC-002 §5
// describes.
//
// # WHY NOT CHECK AT JOIN AND BE DONE
//
// Because then can_apply would not be the chokepoint, and RFC-002 §5's
// "every op passes through one auditable check" would become "every op
// passes through one check, plus a different check somewhere else that
// actually decides". One place, or it is not a chokepoint.
//
// # THE COST, STATED
//
// A role revoked mid-session stays in effect on that connection until it
// closes or its entry expires. That window is real. It is bounded by a TTL
// here, and by auth.role_revoked closing affected connections (which is
// what makes the common case immediate rather than eventual). "Revocation
// is instant" would be a claim this architecture cannot honour, so it is
// not made.
package roles

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Role mirrors auth-service's three (docs/api/spaces.md §0). Duplicated
// rather than imported: this service must not depend on auth-service's
// internal packages, and three string constants are a smaller cost than
// that coupling.
type Role string

const (
	Viewer Role = "viewer"
	Editor Role = "editor"
	Admin  Role = "admin"
)

// CanWrite is the rule, and it is deliberately the whole rule: viewers
// read, everybody else writes. Anything finer (per-block, per-op-kind)
// would be a permission model this repo has no requirement for, and
// guessing at one now means maintaining a guess.
func (r Role) CanWrite() bool { return r == Editor || r == Admin }

// DefaultTTL bounds how long a resolved role is trusted without asking
// again. Five minutes because a revocation should not outlive a coffee
// break, and because the event that normally closes the connection first
// makes this the fallback rather than the mechanism.
const DefaultTTL = 5 * time.Minute

type entry struct {
	role     Role
	resolved time.Time
}

// Directory is the per-connection role cache. One per process.
type Directory struct {
	mu  sync.RWMutex
	by  map[key]entry
	ttl time.Duration
	now func() time.Time
}

type key struct {
	page  uuid.UUID
	actor uuid.UUID
}

type Option func(*Directory)

func WithTTL(d time.Duration) Option      { return func(dir *Directory) { dir.ttl = d } }
func WithClock(f func() time.Time) Option { return func(dir *Directory) { dir.now = f } }

func New(opts ...Option) *Directory {
	d := &Directory{by: map[key]entry{}, ttl: DefaultTTL, now: time.Now}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Set records a role resolved at join.
func (d *Directory) Set(page, actor uuid.UUID, role Role) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.by[key{page, actor}] = entry{role: role, resolved: d.now()}
}

// Clear forgets one connection's role, on disconnect.
func (d *Directory) Clear(page, actor uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.by, key{page, actor})
}

// Revoke forgets every entry for an actor, across all pages — what
// auth.role_revoked triggers. It does not distinguish spaces because this
// service does not know which pages are in which space; forgetting more
// than strictly necessary costs a re-resolve, and forgetting less would
// leave a stale grant in place, which is the failure that matters.
func (d *Directory) Revoke(actor uuid.UUID) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for k := range d.by {
		if k.actor == actor {
			delete(d.by, k)
			n++
		}
	}
	return n
}

// Role returns the cached role and whether it is still usable. A missing
// or expired entry returns ok=false, and the caller must treat that as
// "no" rather than as "unknown" — an unresolved role that permitted a
// write would make every failure to resolve an escalation.
func (d *Directory) Role(page, actor uuid.UUID) (Role, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	e, ok := d.by[key{page, actor}]
	if !ok {
		return "", false
	}
	if d.now().Sub(e.resolved) > d.ttl {
		return "", false
	}
	return e.role, true
}

// CanApply is the predicate session.CanApplyFunc wraps.
//
// Deny by default: an actor with no live entry cannot write. Every writer
// arrives through a WebSocket join that resolved a role first, so an
// absent entry means either an expired one or a path that skipped the
// join — and both should be refused rather than trusted.
func (d *Directory) CanApply(page, actor uuid.UUID) bool {
	role, ok := d.Role(page, actor)
	return ok && role.CanWrite()
}

// WS adapts Directory and Resolver to the string-typed interfaces wsapi
// declares.
//
// wsapi speaks strings because a transport package should not need this
// one's Role type to describe its own dependency — the conversion belongs
// at the boundary, and it belongs HERE because this package owns what a
// role is.
type WS struct {
	Dir      *Directory
	Resolver *Resolver
}

func (w WS) Set(page, actor uuid.UUID, role string) { w.Dir.Set(page, actor, Role(role)) }
func (w WS) Clear(page, actor uuid.UUID)            { w.Dir.Clear(page, actor) }

func (w WS) Resolve(ctx context.Context, page, actor uuid.UUID) (string, error) {
	role, err := w.Resolver.Resolve(ctx, page, actor)
	return string(role), err
}
