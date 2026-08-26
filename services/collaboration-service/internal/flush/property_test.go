package flush

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentEnqueueLosesNothing is the property that actually matters
// for this package: however many goroutines enqueue concurrently, and
// however batching/interval/retry slices them up, every op that Enqueue
// accepted must eventually reach AppendBatch exactly once. Run with -race.
func TestConcurrentEnqueueLosesNothing(t *testing.T) {
	repo := &fakeRepo{}
	l := New(repo, WithBatchSize(7), WithInterval(5*time.Millisecond), WithQueueSize(50))
	l.Start(context.Background())

	const numGoroutines = 20
	const opsPerGoroutine = 15
	total := numGoroutines * opsPerGoroutine

	ids := make([][]string, numGoroutines)
	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		g := g
		ids[g] = make([]string, opsPerGoroutine)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				op := testOp(t)
				ids[g][i] = op.ID.String()
				require.NoError(t, l.Enqueue(context.Background(), op))
			}
		}()
	}
	wg.Wait()

	l.Stop() // flush everything still buffered before asserting

	seen := make(map[string]int)
	for _, batch := range repo.callsSnapshot() {
		for _, l := range batch {
			seen[l.ID.String()]++
		}
	}

	assert.Len(t, seen, total, "every enqueued op must show up exactly once across all flushed batches")
	for _, gids := range ids {
		for _, id := range gids {
			assert.Equal(t, 1, seen[id], "op %s must appear exactly once, not lost or duplicated", id)
		}
	}
}
