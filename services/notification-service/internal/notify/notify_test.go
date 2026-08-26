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

func (f *fakeRepo) ListForUser(context.Context, uuid.UUID, int32) ([]notify.Notification, error) {
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
