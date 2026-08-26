//go:build integration

package outbox_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"marginal/auth-service/internal/authrepo/gen"
	"marginal/auth-service/internal/migrate"
	"marginal/auth-service/internal/outbox"
)

// Real Postgres via testcontainers-go, never a mock — the standing rule
// for anything touching infrastructure (.agents/agents.md). Requires
// Docker; run with: go test -tags=integration ./internal/outbox/...
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("auth_service_outbox_test"),
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
	return pool
}

func newTestNATS(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &natsserver.Options{Port: -1}
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

// TestPollerPublishesAndMarksPublished is the Poller's whole
// job: a row written via WriteUserRegistered (simulating what Register's
// own transaction does) gets claimed, published to NATS, and marked
// published_at — exactly once, not left for a second poll to redeliver
// needlessly.
func TestPollerPublishesAndMarksPublished(t *testing.T) {
	pool := newTestPool(t)
	nc := newTestNATS(t)
	ctx := context.Background()

	userID := uuid.Must(uuid.NewV7())
	require.NoError(t, outbox.WriteUserRegistered(ctx, authrepo.New(pool), userID, "a@b.com", "Ada"))

	received := make(chan []byte, 1)
	sub, err := nc.Subscribe(outbox.EventUserRegistered, func(msg *nats.Msg) { received <- msg.Data })
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	poller := outbox.NewPoller(pool, nc, outbox.WithInterval(20*time.Millisecond))
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go poller.Run(pollCtx, func(err error) { t.Logf("poller error: %v", err) })

	var envelope struct {
		ID      uuid.UUID `json:"id"`
		Payload []byte    `json:"payload"`
	}
	select {
	case data := <-received:
		require.NoError(t, json.Unmarshal(data, &envelope))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the poller to publish")
	}

	var decoded struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	require.NoError(t, json.Unmarshal(envelope.Payload, &decoded))
	require.Equal(t, userID.String(), decoded.UserID)
	require.Equal(t, "a@b.com", decoded.Email)

	require.Eventually(t, func() bool {
		var publishedAt *time.Time
		err := pool.QueryRow(ctx, "SELECT published_at FROM auth.outbox WHERE id = $1", pgtype.UUID{Bytes: envelope.ID, Valid: true}).Scan(&publishedAt)
		return err == nil && publishedAt != nil
	}, time.Second, 10*time.Millisecond, "the row must be marked published after a successful publish")
}

// TestPollerSkipsAlreadyPublishedRows confirms a second poll doesn't
// redeliver a row the first poll already marked published — the whole
// point of the published_at filter.
func TestPollerSkipsAlreadyPublishedRows(t *testing.T) {
	pool := newTestPool(t)
	nc := newTestNATS(t)
	ctx := context.Background()

	require.NoError(t, outbox.WriteUserRegistered(ctx, authrepo.New(pool), uuid.Must(uuid.NewV7()), "a@b.com", "Ada"))

	// NATS delivers on its own goroutine, distinct from this test's — a
	// plain int here was a real, race-detector-confirmed data race
	// (deliveries++ racing the read below with no happens-before between
	// them), not a hypothetical one.
	var deliveries atomic.Int32
	sub, err := nc.Subscribe(outbox.EventUserRegistered, func(*nats.Msg) { deliveries.Add(1) })
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	poller := outbox.NewPoller(pool, nc, outbox.WithInterval(20*time.Millisecond))
	pollCtx, cancel := context.WithCancel(ctx)
	go poller.Run(pollCtx, func(err error) { t.Logf("poller error: %v", err) })

	require.Eventually(t, func() bool {
		var publishedAt *time.Time
		err := pool.QueryRow(ctx, "SELECT published_at FROM auth.outbox LIMIT 1").Scan(&publishedAt)
		return err == nil && publishedAt != nil
	}, time.Second, 10*time.Millisecond)

	// give the poller several more ticks, then stop it and check nothing extra arrived
	time.Sleep(150 * time.Millisecond)
	cancel()
	require.NoError(t, nc.Flush())
	require.Equal(t, int32(1), deliveries.Load(), "an already-published row must not be redelivered by a later poll")
}
