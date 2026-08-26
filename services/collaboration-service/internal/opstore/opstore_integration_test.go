//go:build integration

package opstore_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/migrate"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/opstore"
	"marginal/collaboration-service/internal/pageop"
)

// Real Postgres via testcontainers-go, never a mock — the standing rule
// for anything touching infrastructure (.agents/agents.md). Requires
// Docker; run with: go test -tags=integration ./internal/opstore/...
// Returns the pool too, so tests can independently verify the outbox side
// of Append's transaction (opstore.Repo has no outbox-reading method of
// its own — nothing in this repo's scope consumes it yet).
func newTestRepo(t *testing.T) (*opstore.PostgresRepo, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("collaboration_service_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sqlOpen(connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, migrate.Up(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return opstore.NewPostgresRepo(pool), pool
}

func countOutboxEvents(t *testing.T, pool *pgxpool.Pool, pageID uuid.UUID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM collab.outbox WHERE aggregate_id = $1`, pageID).Scan(&n)
	require.NoError(t, err)
	return n
}

func testLoggedOp(t *testing.T, pageID uuid.UUID) oplog.LoggedOp {
	t.Helper()
	l, err := oplog.New(pageID, uuid.Must(uuid.NewV7()), oplog.ActorUser, nil,
		oplog.VectorClock{"actor-1": 1}, pageop.Text{
			BlockID: documentcore.BlockID(uuid.Must(uuid.NewV7())),
			Op:      ops.InsertText{At: nil, Text: "hello"},
		})
	require.NoError(t, err)
	return l
}

func TestAppendAndListRoundTrip(t *testing.T) {
	repo, pool := newTestRepo(t)
	ctx := context.Background()
	pageID := uuid.Must(uuid.NewV7())

	l := testLoggedOp(t, pageID)
	inserted, err := repo.Append(ctx, l)
	require.NoError(t, err)
	assert.True(t, inserted)

	got, err := repo.ListForPage(ctx, pageID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, l.ID, got[0].ID)
	assert.Equal(t, l.PageID, got[0].PageID)
	assert.Equal(t, l.ActorID, got[0].ActorID)
	assert.Equal(t, l.ActorKind, got[0].ActorKind)
	assert.Equal(t, l.VectorClock, got[0].VectorClock)
	assert.Equal(t, l.Op, got[0].Op)

	assert.Equal(t, 1, countOutboxEvents(t, pool, pageID), "a successful append must write exactly one outbox event, in the same transaction")
}

func TestAppendIsIdempotentOnDuplicateID(t *testing.T) {
	repo, pool := newTestRepo(t)
	ctx := context.Background()
	pageID := uuid.Must(uuid.NewV7())
	l := testLoggedOp(t, pageID)

	inserted, err := repo.Append(ctx, l)
	require.NoError(t, err)
	assert.True(t, inserted)

	// A retried flush after a crash redelivers the same LoggedOp (same ID)
	// — must be a silent no-op, not an error and not a duplicate row.
	inserted, err = repo.Append(ctx, l)
	require.NoError(t, err)
	assert.False(t, inserted, "duplicate append must report inserted=false")

	got, err := repo.ListForPage(ctx, pageID)
	require.NoError(t, err)
	assert.Len(t, got, 1, "duplicate append must not create a second row")
	assert.Equal(t, 1, countOutboxEvents(t, pool, pageID), "a duplicate append must not write a second outbox event either")
}

func TestListForPageOrdersOldestFirst(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	pageID := uuid.Must(uuid.NewV7())

	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		l := testLoggedOp(t, pageID)
		ids = append(ids, l.ID)
		_, err := repo.Append(ctx, l)
		require.NoError(t, err)
	}

	got, err := repo.ListForPage(ctx, pageID)
	require.NoError(t, err)
	require.Len(t, got, 3)
	// UUIDv7 ids sort chronologically, matching insertion order — a
	// second, independent way (besides created_at) to see the ordering
	// held.
	for i, l := range got {
		assert.Equal(t, ids[i], l.ID)
	}
}

func TestListForPageScopesToOnePage(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	pageA, pageB := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	_, err := repo.Append(ctx, testLoggedOp(t, pageA))
	require.NoError(t, err)
	_, err = repo.Append(ctx, testLoggedOp(t, pageB))
	require.NoError(t, err)

	got, err := repo.ListForPage(ctx, pageA)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, pageA, got[0].PageID)
}

func TestAppendBatchInsertsAllAndCountsOnlyNew(t *testing.T) {
	repo, pool := newTestRepo(t)
	ctx := context.Background()
	pageID := uuid.Must(uuid.NewV7())

	batch := make([]oplog.LoggedOp, 5)
	for i := range batch {
		batch[i] = testLoggedOp(t, pageID)
	}

	inserted, err := repo.AppendBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, 5, inserted)

	got, err := repo.ListForPage(ctx, pageID)
	require.NoError(t, err)
	assert.Len(t, got, 5)
	assert.Equal(t, 5, countOutboxEvents(t, pool, pageID))
}

func TestAppendBatchSkipsAlreadyFlushedOpsWithinTheSameBatch(t *testing.T) {
	repo, pool := newTestRepo(t)
	ctx := context.Background()
	pageID := uuid.Must(uuid.NewV7())

	alreadyFlushed := testLoggedOp(t, pageID)
	inserted, err := repo.Append(ctx, alreadyFlushed)
	require.NoError(t, err)
	require.True(t, inserted)

	fresh := []oplog.LoggedOp{testLoggedOp(t, pageID), testLoggedOp(t, pageID)}
	// A redelivered batch mixing one already-committed op with two new
	// ones — the exact shape a crash-then-retry produces.
	mixed := append([]oplog.LoggedOp{alreadyFlushed}, fresh...)

	insertedCount, err := repo.AppendBatch(ctx, mixed)
	require.NoError(t, err)
	assert.Equal(t, 2, insertedCount, "only the two genuinely new ops count as inserted")

	got, err := repo.ListForPage(ctx, pageID)
	require.NoError(t, err)
	assert.Len(t, got, 3, "the already-flushed op must not be duplicated")
	assert.Equal(t, 3, countOutboxEvents(t, pool, pageID), "one outbox event per op ever actually inserted, never for a skipped duplicate")
}

func TestAppendBatchOnEmptySliceIsANoOp(t *testing.T) {
	repo, _ := newTestRepo(t)
	inserted, err := repo.AppendBatch(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, inserted)
}
