package notify_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/notification-service/internal/notify"
)

// fakeRepo is a small in-memory Repo — this package tests HandleUserRegistered's
// own decode/dedup-call logic, independent of what's underneath Repo (the
// real Postgres path is covered by its own integration test).
type fakeRepo struct {
	mu      sync.Mutex
	created []notify.Notification
	seen    map[uuid.UUID]bool
}

func newFakeRepo() *fakeRepo { return &fakeRepo{seen: make(map[uuid.UUID]bool)} }

func (f *fakeRepo) Create(_ context.Context, userID, sourceEventID uuid.UUID, kind, message string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen[sourceEventID] {
		return false, nil
	}
	f.seen[sourceEventID] = true
	f.created = append(f.created, notify.Notification{UserID: userID, SourceEventID: sourceEventID, Kind: kind, Message: message})
	return true, nil
}

func (f *fakeRepo) CreatePointer(
	_ context.Context, userID, sourceEventID, actorID uuid.UUID, kind string, pointer json.RawMessage,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen[sourceEventID] {
		return false, nil
	}
	f.seen[sourceEventID] = true
	a := actorID
	f.created = append(f.created, notify.Notification{
		UserID: userID, SourceEventID: sourceEventID, Kind: kind,
		ActorID: &a, Pointer: pointer,
	})
	return true, nil
}

func (f *fakeRepo) ListForUser(context.Context, uuid.UUID, int32) ([]notify.Notification, error) {
	panic("not used by these tests")
}

func (f *fakeRepo) MarkRead(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	panic("not used by these tests")
}

func (f *fakeRepo) MarkAllRead(context.Context, uuid.UUID) (int64, error) {
	panic("not used by these tests")
}

func (f *fakeRepo) CountUnread(context.Context, uuid.UUID) (int64, error) {
	panic("not used by these tests")
}

func TestHandleUserRegisteredCreatesAWelcomeNotification(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.Must(uuid.NewV7())
	eventID := uuid.Must(uuid.NewV7())
	payload, err := json.Marshal(notify.UserRegisteredEvent{UserID: userID, Email: "a@b.com", DisplayName: "Ada"})
	require.NoError(t, err)

	err = notify.HandleUserRegistered(context.Background(), repo, eventID, payload)
	require.NoError(t, err)

	require.Len(t, repo.created, 1)
	assert.Equal(t, userID, repo.created[0].UserID)
	assert.Equal(t, eventID, repo.created[0].SourceEventID)
	assert.Equal(t, notify.KindWelcome, repo.created[0].Kind)
	assert.Contains(t, repo.created[0].Message, "Ada")
}

func TestHandleUserRegisteredRejectsInvalidPayload(t *testing.T) {
	repo := newFakeRepo()
	err := notify.HandleUserRegistered(context.Background(), repo, uuid.Must(uuid.NewV7()), []byte("{not json"))
	assert.Error(t, err)
	assert.Empty(t, repo.created)
}

func TestHandleUserRegisteredIsIdempotentOnRedelivery(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.Must(uuid.NewV7())
	eventID := uuid.Must(uuid.NewV7())
	payload, err := json.Marshal(notify.UserRegisteredEvent{UserID: userID, Email: "a@b.com", DisplayName: "Ada"})
	require.NoError(t, err)

	require.NoError(t, notify.HandleUserRegistered(context.Background(), repo, eventID, payload))
	require.NoError(t, notify.HandleUserRegistered(context.Background(), repo, eventID, payload))

	assert.Len(t, repo.created, 1, "a redelivered event must not create a second notification")
}

func TestHandleMentionStoresAPointerAndNoText(t *testing.T) {
	repo := newFakeRepo()
	p := notify.MentionPointer{
		PageID: uuid.New(), BlockID: uuid.New(), ThreadID: uuid.New(),
		CommentID: uuid.New(), ActorID: uuid.New(), UserID: uuid.New(),
	}
	payload, err := json.Marshal(p)
	require.NoError(t, err)
	eventID := uuid.Must(uuid.NewV7())

	require.NoError(t, notify.HandleMention(context.Background(), repo, eventID, payload))
	require.Len(t, repo.created, 1)

	n := repo.created[0]
	assert.Equal(t, notify.KindMention, n.Kind)
	assert.Equal(t, p.UserID, n.UserID, "a mention is stored for the person mentioned, not the one who wrote it")
	require.NotNil(t, n.ActorID)
	assert.Equal(t, p.ActorID, *n.ActorID)
	assert.Empty(t, n.Message, "a mention carries a pointer, never a rendered sentence")

	// The rule § 20 states, asserted rather than trusted: what is stored
	// must be resolvable back to the anchor, and must not be a copy of
	// anything a reader will later be shown.
	var back notify.MentionPointer
	require.NoError(t, json.Unmarshal(n.Pointer, &back))
	assert.Equal(t, p, back)
}

func TestHandleMentionIsIdempotentPerEvent(t *testing.T) {
	repo := newFakeRepo()
	payload, err := json.Marshal(notify.MentionPointer{UserID: uuid.New(), ActorID: uuid.New()})
	require.NoError(t, err)
	eventID := uuid.Must(uuid.NewV7())

	require.NoError(t, notify.HandleMention(context.Background(), repo, eventID, payload))
	require.NoError(t, notify.HandleMention(context.Background(), repo, eventID, payload))
	assert.Len(t, repo.created, 1, "NATS redelivers; a mention must not arrive twice")
}

func TestHandleMentionRefusesAPayloadWithNobodyToTell(t *testing.T) {
	repo := newFakeRepo()
	payload, err := json.Marshal(notify.MentionPointer{ActorID: uuid.New()})
	require.NoError(t, err)

	err = notify.HandleMention(context.Background(), repo, uuid.Must(uuid.NewV7()), payload)
	require.Error(t, err, "a mention with no user_id belongs to nobody — storing it hides it")
	assert.Empty(t, repo.created)
}

// A payload from a NEWER producer must survive this hop intact. The
// pointer is stored verbatim precisely so a field this service does not
// understand yet still reaches the client that does.
func TestHandleMentionPreservesUnknownPayloadFields(t *testing.T) {
	repo := newFakeRepo()
	user := uuid.New()
	payload := []byte(`{"user_id":"` + user.String() + `","space_id":"future","weight":3}`)

	require.NoError(t, notify.HandleMention(context.Background(), repo, uuid.Must(uuid.NewV7()), payload))
	require.Len(t, repo.created, 1)
	assert.JSONEq(t, string(payload), string(repo.created[0].Pointer))
}
