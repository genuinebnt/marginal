package flush

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeRepo is a small, hand-written fake behind opstore.Repo — not
// "mocking infrastructure" (that rule is about integration tests skipping
// real Postgres); this is a unit test of Loop's own scheduling/retry logic,
// which shouldn't need a container to exercise.
type fakeRepo struct {
	mu      sync.Mutex
	calls   [][]oplog.LoggedOp
	failN   int // AppendBatch fails this many times before succeeding
	callCnt int
}

func (f *fakeRepo) Append(context.Context, oplog.LoggedOp) (bool, error) {
	panic("not used by flush.Loop")
}

func (f *fakeRepo) ListForPage(context.Context, uuid.UUID) ([]oplog.LoggedOp, error) {
	panic("not used by flush.Loop")
}

func (f *fakeRepo) AppendBatch(_ context.Context, ls []oplog.LoggedOp) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCnt++
	if f.callCnt <= f.failN {
		return 0, errors.New("fakeRepo: simulated transient failure")
	}
	cp := append([]oplog.LoggedOp(nil), ls...)
	f.calls = append(f.calls, cp)
	return len(ls), nil
}

func (f *fakeRepo) callsSnapshot() [][]oplog.LoggedOp {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]oplog.LoggedOp(nil), f.calls...)
}

func testOp(t *testing.T) oplog.LoggedOp {
	t.Helper()
	l, err := oplog.New(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), oplog.ActorUser, nil,
		nil, pageop.Text{
			BlockID: documentcore.BlockID(uuid.Must(uuid.NewV7())),
			Op:      ops.InsertText{At: nil, Text: "x"},
		})
	require.NoError(t, err)
	return l
}

func TestFlushesOnBatchSize(t *testing.T) {
	repo := &fakeRepo{}
	l := New(repo, WithBatchSize(3), WithInterval(time.Hour))
	l.Start(context.Background())
	defer l.Stop()

	for i := 0; i < 3; i++ {
		require.NoError(t, l.Enqueue(context.Background(), testOp(t)))
	}

	require.Eventually(t, func() bool { return len(repo.callsSnapshot()) == 1 }, time.Second, time.Millisecond,
		"a full batch must flush without waiting for the interval")
	assert.Len(t, repo.callsSnapshot()[0], 3)
}

func TestFlushesOnIntervalWhenBatchNeverFills(t *testing.T) {
	repo := &fakeRepo{}
	l := New(repo, WithBatchSize(100), WithInterval(20*time.Millisecond))
	l.Start(context.Background())
	defer l.Stop()

	require.NoError(t, l.Enqueue(context.Background(), testOp(t)))

	require.Eventually(t, func() bool { return len(repo.callsSnapshot()) == 1 }, time.Second, time.Millisecond,
		"the interval must flush a partial batch")
	assert.Len(t, repo.callsSnapshot()[0], 1)
}

func TestStopDrainsBufferedOpsBeforeReturning(t *testing.T) {
	repo := &fakeRepo{}
	l := New(repo, WithBatchSize(100), WithInterval(time.Hour), WithQueueSize(10))
	l.Start(context.Background())

	for i := 0; i < 4; i++ {
		require.NoError(t, l.Enqueue(context.Background(), testOp(t)))
	}

	l.Stop() // must not return until the 4 buffered ops are flushed

	calls := repo.callsSnapshot()
	require.Len(t, calls, 1)
	assert.Len(t, calls[0], 4)
}

func TestEnqueueAfterStopReturnsErrStopped(t *testing.T) {
	repo := &fakeRepo{}
	l := New(repo)
	l.Start(context.Background())
	l.Stop()

	err := l.Enqueue(context.Background(), testOp(t))
	assert.ErrorIs(t, err, ErrStopped)
}

func TestEnqueueRespectsCallerContextCancellation(t *testing.T) {
	repo := &fakeRepo{}
	// Queue size 0 plus no drain (interval effectively infinite, batch
	// size huge, and nobody calling Start) means Enqueue can never
	// succeed — proving it returns ctx.Err() instead of hanging forever.
	l := New(repo, WithQueueSize(0))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := l.Enqueue(ctx, testOp(t))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestFlushRetriesTransientFailureThenSucceeds(t *testing.T) {
	repo := &fakeRepo{failN: 2}
	l := New(repo, WithBatchSize(1), WithInterval(time.Hour), WithMaxAttempts(5))
	l.Start(context.Background())
	defer l.Stop()

	require.NoError(t, l.Enqueue(context.Background(), testOp(t)))

	require.Eventually(t, func() bool { return len(repo.callsSnapshot()) == 1 }, time.Second, time.Millisecond,
		"must eventually succeed after transient failures")
}

func TestFlushGivesUpAfterMaxAttemptsAndReportsViaOnError(t *testing.T) {
	repo := &fakeRepo{failN: 999} // always fails
	var mu sync.Mutex
	var gotErr error
	l := New(repo,
		WithBatchSize(1),
		WithInterval(time.Hour),
		WithMaxAttempts(2),
		WithOnError(func(err error) {
			mu.Lock()
			defer mu.Unlock()
			gotErr = err
		}),
	)
	l.Start(context.Background())
	defer l.Stop()

	require.NoError(t, l.Enqueue(context.Background(), testOp(t)))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return gotErr != nil
	}, time.Second, time.Millisecond, "onError must fire once retries are exhausted")
}

func TestContextCancellationAlsoDrainsAndStops(t *testing.T) {
	repo := &fakeRepo{}
	l := New(repo, WithBatchSize(100), WithInterval(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	l.Start(ctx)

	require.NoError(t, l.Enqueue(context.Background(), testOp(t)))
	cancel()
	l.wg.Wait() // run() must exit once ctx is done, same as Stop()

	assert.Len(t, repo.callsSnapshot(), 1)
}
