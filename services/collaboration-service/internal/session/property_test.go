package session

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
)

// TestConcurrentApplyClientOpSerializesCorrectly is the property that
// actually matters for a doc-actor: many goroutines submitting ops
// concurrently must still end up with every op applied exactly once, the
// text containing exactly the expected total character count, and every
// subscriber receiving every op it didn't submit itself — proving the
// internal mutex genuinely serializes access rather than just happening
// not to crash. Run with -race.
func TestConcurrentApplyClientOpSerializesCorrectly(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	blockID := insertBlock(t, s, uuid.Must(uuid.NewV7()), "")

	const numGoroutines = 15
	const opsPerGoroutine = 10

	subs := make([]*recordingSubscriber, numGoroutines)
	subIDs := make([]uint64, numGoroutines)
	for i := range subs {
		subs[i] = &recordingSubscriber{}
		subIDs[i], _, _, _ = s.Subscribe(uuid.Must(uuid.NewV7()), subs[i])
	}

	var wg sync.WaitGroup
	var wantRunes int64
	allIDs := make([][]uuid.UUID, numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		g := g
		allIDs[g] = make([]uuid.UUID, opsPerGoroutine)
		wg.Add(1)
		go func() {
			defer wg.Done()
			actor := uuid.Must(uuid.NewV7())
			for i := 0; i < opsPerGoroutine; i++ {
				text := fmt.Sprintf("g%di%d;", g, i)
				result, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser,
					pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: text}}, subIDs[g])
				require.NoError(t, err)
				allIDs[g][i] = result.Op.ID
				atomic.AddInt64(&wantRunes, int64(len([]rune(text))))
			}
		}()
	}
	wg.Wait()

	total := numGoroutines * opsPerGoroutine
	assert.Equal(t, int(wantRunes), len([]rune(blockText(t, s, blockID))),
		"every op's text must have landed exactly once, none lost or duplicated")

	// Every subscriber must see every op except the ones its own
	// goroutine submitted.
	for g, sub := range subs {
		got := sub.snapshot()
		assert.Len(t, got, total-opsPerGoroutine, "subscriber %d must receive every op except its own", g)

		own := make(map[uuid.UUID]bool, opsPerGoroutine)
		for _, id := range allIDs[g] {
			own[id] = true
		}
		for _, l := range got {
			assert.False(t, own[l.ID], "subscriber %d must never receive its own submitted op via Deliver", g)
		}
	}
}
