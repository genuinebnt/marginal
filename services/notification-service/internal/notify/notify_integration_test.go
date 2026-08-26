//go:build integration

package notify_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"marginal/notification-service/internal/migrate"
	"marginal/notification-service/internal/notify"
)

// Real Postgres via testcontainers-go, never a mock — the standing rule
// for anything touching infrastructure (.agents/agents.md). Requires
// Docker; run with: go test -tags=integration ./internal/notify/...
func newTestRepo(t *testing.T) *notify.PostgresRepo {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("notification_service_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, migrate.Up(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return notify.NewPostgresRepo(pool)
}

// newTestNATS runs an in-process NATS server — no Docker image needed for
// this piece, and it's realistic: this is the same client library and
// wire protocol talking to a real (if embedded) nats-server, not a fake.
func newTestNATS(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &natsserver.Options{Port: -1} // -1: pick a free port
	srv, err := natsserver.NewServer(opts)
	require.NoError(t, err)
	srv.Start()
	require.True(t, srv.ReadyForConnections(2*time.Second))
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}

func TestPostgresRepoCreateAndListRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7())
	eventID := uuid.Must(uuid.NewV7())

	created, err := repo.Create(ctx, userID, eventID, notify.KindWelcome, "Welcome!")
	require.NoError(t, err)
	assert.True(t, created)

	got, err := repo.ListForUser(ctx, userID, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, notify.KindWelcome, got[0].Kind)
	assert.Equal(t, "Welcome!", got[0].Message)
	assert.Nil(t, got[0].ReadAt)
}

func TestPostgresRepoCreateIsIdempotentOnSourceEventID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7())
	eventID := uuid.Must(uuid.NewV7())

	created, err := repo.Create(ctx, userID, eventID, notify.KindWelcome, "Welcome!")
	require.NoError(t, err)
	assert.True(t, created)

	created, err = repo.Create(ctx, userID, eventID, notify.KindWelcome, "Welcome!")
	require.NoError(t, err)
	assert.False(t, created, "a redelivered event's own id must not create a duplicate row")

	got, err := repo.ListForUser(ctx, userID, 10)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestPostgresRepoListScopesToOneUser(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	userA, userB := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	_, err := repo.Create(ctx, userA, uuid.Must(uuid.NewV7()), notify.KindWelcome, "for A")
	require.NoError(t, err)
	_, err = repo.Create(ctx, userB, uuid.Must(uuid.NewV7()), notify.KindWelcome, "for B")
	require.NoError(t, err)

	got, err := repo.ListForUser(ctx, userA, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "for A", got[0].Message)
}

// TestSubscribeEndToEnd is the real point of this package: a message
// published to auth.user_registered over a real NATS connection, in the
// exact envelope auth-service's outbox poller produces, ends up as a
// queryable row in Postgres.
func TestSubscribeEndToEnd(t *testing.T) {
	repo := newTestRepo(t)
	nc := newTestNATS(t)

	unsubscribe, err := notify.Subscribe(nc, repo)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unsubscribe() })

	userID := uuid.Must(uuid.NewV7())
	eventID := uuid.Must(uuid.NewV7())
	payload, err := json.Marshal(notify.UserRegisteredEvent{UserID: userID, Email: "a@b.com", DisplayName: "Ada"})
	require.NoError(t, err)
	envelope, err := json.Marshal(struct {
		ID      uuid.UUID `json:"id"`
		Payload []byte    `json:"payload"`
	}{ID: eventID, Payload: payload})
	require.NoError(t, err)

	require.NoError(t, nc.Publish(notify.SubjectUserRegistered, envelope))
	require.NoError(t, nc.Flush())

	require.Eventually(t, func() bool {
		got, err := repo.ListForUser(context.Background(), userID, 10)
		return err == nil && len(got) == 1
	}, 2*time.Second, 10*time.Millisecond, "the published event must be consumed and persisted")
}
