package wsapi

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"marginal/collaboration-service/internal/roles"
)

type fakeLister struct {
	members []roles.Member
	err     error
	calls   int
}

func (f *fakeLister) Members(context.Context, uuid.UUID, uuid.UUID) ([]roles.Member, error) {
	f.calls++
	return f.members, f.err
}

func TestResolveMentions(t *testing.T) {
	ada, reiko, author := uuid.New(), uuid.New(), uuid.New()
	all := []roles.Member{
		{UserID: ada, DisplayName: "Ada Lovelace"},
		{UserID: reiko, DisplayName: "Reiko"},
		{UserID: author, DisplayName: "Genuine Basil"},
	}
	page := uuid.New()

	got := func(t *testing.T, body string) []uuid.UUID {
		t.Helper()
		l := &fakeLister{members: all}
		ids, err := resolveMentions(context.Background(), l, body, page, author)
		if err != nil {
			t.Fatalf("resolveMentions: %v", err)
		}
		return ids
	}

	t.Run("a display name with a space is mentioned without one", func(t *testing.T) {
		if ids := got(t, "@AdaLovelace does this hold?"); len(ids) != 1 || ids[0] != ada {
			t.Fatalf("got %v, want [%v]", ids, ada)
		}
	})

	t.Run("mentioning yourself notifies nobody", func(t *testing.T) {
		if ids := got(t, "note to self: @GenuineBasil"); len(ids) != 0 {
			t.Fatalf("got %v, want none", ids)
		}
	})

	t.Run("a handle nobody answers to is dropped, not an error", func(t *testing.T) {
		if ids := got(t, "@nobody and @Reiko"); len(ids) != 1 || ids[0] != reiko {
			t.Fatalf("got %v, want [%v]", ids, reiko)
		}
	})

	// The one that matters for the security story: membership is the whole
	// guest list. Somebody outside the space cannot be reached by typing
	// their name, however well the handle matches.
	t.Run("a non-member cannot be reached by name", func(t *testing.T) {
		l := &fakeLister{members: []roles.Member{{UserID: author, DisplayName: "Genuine Basil"}}}
		ids, err := resolveMentions(context.Background(), l, "@AdaLovelace", page, author)
		if err != nil || len(ids) != 0 {
			t.Fatalf("got %v, %v — want none", ids, err)
		}
	})

	t.Run("no @ means no network call at all", func(t *testing.T) {
		l := &fakeLister{members: all}
		if _, err := resolveMentions(context.Background(), l, "the tiebreak holds", page, author); err != nil {
			t.Fatal(err)
		}
		if l.calls != 0 {
			t.Fatalf("listed members %d times for a body with no mention", l.calls)
		}
	})

	t.Run("a colliding handle picks one person, not both", func(t *testing.T) {
		a2 := uuid.New()
		l := &fakeLister{members: []roles.Member{
			{UserID: ada, DisplayName: "Ada Lovelace"},
			{UserID: a2, DisplayName: "adalovelace"},
		}}
		ids, err := resolveMentions(context.Background(), l, "@AdaLovelace", page, author)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != 1 {
			t.Fatalf("got %v — @ada must not mean everyone called Ada", ids)
		}
	})

	t.Run("a resolver failure is reported, never treated as nobody-matched", func(t *testing.T) {
		l := &fakeLister{err: errors.New("auth-service is down")}
		if _, err := resolveMentions(context.Background(), l, "@Reiko", page, author); err == nil {
			t.Fatal("want an error so the caller can log the drop; got nil")
		}
	})
}
