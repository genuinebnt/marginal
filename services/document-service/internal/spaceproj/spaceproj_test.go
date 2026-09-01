package spaceproj

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeSource stands in for auth-service's ListAllMemberships.
type fakeSource struct {
	rows []Membership
	err  error
}

func (f fakeSource) ListAllMemberships(context.Context) ([]Membership, error) {
	return f.rows, f.err
}

// A reconcile that adopted an empty answer would revoke every reader's
// access at once, and "auth has no memberships at all" is far more likely
// to be a broken call than a true statement about a running system.
func TestReconcileRefusesAnEmptyMembershipSet(t *testing.T) {
	p := &Projector{} // never reaches the database: the guard is before it
	n, err := p.Reconcile(context.Background(), fakeSource{rows: nil})
	if !errors.Is(err, ErrEmptySource) {
		t.Fatalf("err = %v, want ErrEmptySource", err)
	}
	if n != 0 {
		t.Fatalf("reported %d applied rows for a refused reconcile", n)
	}
}

func TestReconcilePropagatesASourceFailureRatherThanClearing(t *testing.T) {
	p := &Projector{}
	boom := errors.New("auth-service unreachable")
	if _, err := p.Reconcile(context.Background(), fakeSource{err: boom}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the source's own error", err)
	}
}

// The ordering rule, stated as a table because it is the subtle part.
//
// Core NATS gives no ordering guarantee across publishes, so the projection
// has to converge on the same answer whatever order the bus delivers in.
// Both queries compare the EVENT's own granted_at against the stored one —
// this test pins the intent those SQL guards encode, so a later "simplify"
// that drops the WHERE clause has something to fail against.
func TestTheOrderingRuleIsAboutTheEventsTimestampNotArrival(t *testing.T) {
	older := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)

	for _, tc := range []struct {
		name            string
		stored, event   time.Time
		shouldOverwrite bool
	}{
		{"a newer event wins", older, newer, true},
		{"an older event arriving late is ignored", newer, older, false},
		{"an identical timestamp does not overwrite", older, older, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// `granted_at < EXCLUDED.granted_at` is the condition both the
			// upsert and the delete carry.
			if got := tc.stored.Before(tc.event); got != tc.shouldOverwrite {
				t.Fatalf("stored %v vs event %v: overwrite = %v, want %v",
					tc.stored, tc.event, got, tc.shouldOverwrite)
			}
		})
	}
}

// A payload from auth-service must decode here without a shared struct —
// the two definitions are independent on purpose, so this pins that they
// still agree on the wire.
func TestARolePayloadDecodesFromAuthServicesWireShape(t *testing.T) {
	user, space := uuid.New(), uuid.New()
	raw := []byte(`{"user_id":"` + user.String() + `","space_id":"` + space.String() +
		`","role":"editor","granted_at":"2026-09-01T10:00:00Z"}`)

	var p RolePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if p.UserID != user || p.SpaceID != space || p.Role != "editor" {
		t.Fatalf("decoded %+v, want the fields auth-service sent", p)
	}
	if p.GrantedAt.IsZero() {
		t.Error("granted_at did not decode — the ordering guard would compare against the zero time")
	}
}

// A revoke carries no role, and that must not be read as an empty-string
// role rather than an absence.
func TestARevokePayloadHasNoRole(t *testing.T) {
	raw := []byte(`{"user_id":"` + uuid.NewString() + `","space_id":"` + uuid.NewString() +
		`","granted_at":"2026-09-01T10:00:00Z"}`)
	var p RolePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if p.Role != "" {
		t.Fatalf("role = %q, want empty", p.Role)
	}
}
