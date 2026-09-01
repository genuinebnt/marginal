package roles

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOnlyEditorsAndAdminsMayWrite(t *testing.T) {
	if Viewer.CanWrite() {
		t.Error("a viewer could write")
	}
	if !Editor.CanWrite() || !Admin.CanWrite() {
		t.Error("an editor or admin could not write")
	}
	// A role string this service does not recognise must not write. It can
	// only arrive from auth-service, but "it should not happen" is not a
	// reason to permit it if it does.
	if Role("owner").CanWrite() || Role("").CanWrite() {
		t.Error("an unrecognised role could write")
	}
}

// The single most important property here: absence is a DENIAL, not an
// unknown. If a missing entry permitted a write, every failure to resolve
// a role would become an escalation.
func TestAnActorWithNoEntryCannotWrite(t *testing.T) {
	d := New()
	if d.CanApply(uuid.New(), uuid.New()) {
		t.Fatal("an actor who never joined was allowed to write")
	}
}

func TestAResolvedEditorMayWriteAndAViewerMayNot(t *testing.T) {
	d := New()
	page, editor, viewer := uuid.New(), uuid.New(), uuid.New()
	d.Set(page, editor, Editor)
	d.Set(page, viewer, Viewer)

	if !d.CanApply(page, editor) {
		t.Error("a resolved editor could not write")
	}
	if d.CanApply(page, viewer) {
		t.Error("a resolved viewer could write")
	}
}

// A role is scoped to the page it was resolved for. Leaking it across
// pages would mean joining one page granted access to every other.
func TestARoleDoesNotLeakToAnotherPage(t *testing.T) {
	d := New()
	here, elsewhere, actor := uuid.New(), uuid.New(), uuid.New()
	d.Set(here, actor, Editor)

	if d.CanApply(elsewhere, actor) {
		t.Fatal("a role resolved for one page authorised a write to another")
	}
}

func TestAnExpiredRoleStopsAuthorising(t *testing.T) {
	now := time.Now()
	d := New(WithTTL(time.Minute), WithClock(func() time.Time { return now }))
	page, actor := uuid.New(), uuid.New()
	d.Set(page, actor, Editor)

	if !d.CanApply(page, actor) {
		t.Fatal("a freshly resolved role did not authorise")
	}
	now = now.Add(time.Minute + time.Second)
	if d.CanApply(page, actor) {
		t.Fatal("an expired role still authorised a write — the TTL is the bound on how long a revocation can be ignored")
	}
}

func TestClearForgetsOneConnection(t *testing.T) {
	d := New()
	page, actor, other := uuid.New(), uuid.New(), uuid.New()
	d.Set(page, actor, Editor)
	d.Set(page, other, Editor)

	d.Clear(page, actor)
	if d.CanApply(page, actor) {
		t.Error("a cleared entry still authorised")
	}
	if !d.CanApply(page, other) {
		t.Error("clearing one connection cleared another")
	}
}

// What auth.role_revoked triggers: every page, at once. Forgetting more
// than strictly necessary costs a re-resolve; forgetting less leaves a
// stale grant, which is the failure that matters.
func TestRevokeForgetsAnActorEverywhere(t *testing.T) {
	d := New()
	a, b, actor, bystander := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	d.Set(a, actor, Editor)
	d.Set(b, actor, Admin)
	d.Set(a, bystander, Editor)

	if n := d.Revoke(actor); n != 2 {
		t.Fatalf("revoked %d entries, want 2", n)
	}
	if d.CanApply(a, actor) || d.CanApply(b, actor) {
		t.Error("a revoked actor could still write")
	}
	if !d.CanApply(a, bystander) {
		t.Error("revoking one actor affected another")
	}
}

// can_apply runs on every keystroke from every connection, so the
// directory is read concurrently by definition. Run under -race.
func TestConcurrentReadsAndWrites(t *testing.T) {
	d := New()
	page := uuid.New()
	actors := make([]uuid.UUID, 16)
	for i := range actors {
		actors[i] = uuid.New()
		d.Set(page, actors[i], Editor)
	}

	var wg sync.WaitGroup
	for _, a := range actors {
		wg.Add(3)
		go func(a uuid.UUID) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				d.CanApply(page, a)
			}
		}(a)
		go func(a uuid.UUID) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				d.Set(page, a, Editor)
			}
		}(a)
		go func(a uuid.UUID) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				d.Revoke(a)
			}
		}(a)
	}
	wg.Wait()
}
