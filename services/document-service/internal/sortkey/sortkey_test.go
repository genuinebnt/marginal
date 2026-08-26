package sortkey

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestBetweenEmptyBoundsReturnsMidAlphabetKey(t *testing.T) {
	key, err := Between("", "")
	require.NoError(t, err)
	assert.Equal(t, "i", key)
}

func TestBetweenNoLowerBound(t *testing.T) {
	key, err := Between("", "i")
	require.NoError(t, err)
	assert.Less(t, key, "i")
	assert.Greater(t, key, "")
}

func TestBetweenNoUpperBound(t *testing.T) {
	key, err := Between("i", "")
	require.NoError(t, err)
	assert.Greater(t, key, "i")
}

func TestBetweenTwoKeysWithRoom(t *testing.T) {
	key, err := Between("a1", "a3")
	require.NoError(t, err)
	assert.Greater(t, key, "a1")
	assert.Less(t, key, "a3")
}

func TestBetweenAdjacentSingleCharKeys(t *testing.T) {
	key, err := Between("i", "j")
	require.NoError(t, err)
	assert.Greater(t, key, "i")
	assert.Less(t, key, "j")
}

func TestBetweenRepeatedInsertsBeforeTheCurrentFirstKeepWorking(t *testing.T) {
	key := "i"
	for i := 0; i < 20; i++ {
		next, err := Between("", key)
		require.NoError(t, err, "iteration %d, key %q", i, key)
		assert.Less(t, next, key, "iteration %d", i)
		key = next
	}
}

func TestBetweenRepeatedInsertsAfterTheCurrentLastKeepWorking(t *testing.T) {
	key := "i"
	for i := 0; i < 20; i++ {
		next, err := Between(key, "")
		require.NoError(t, err, "iteration %d, key %q", i, key)
		assert.Greater(t, next, key, "iteration %d", i)
		key = next
	}
}

func TestBetweenRepeatedInsertsBetweenTheSameTwoKeysKeepWorking(t *testing.T) {
	lo, hi := "a", "b"
	for i := 0; i < 20; i++ {
		mid, err := Between(lo, hi)
		require.NoError(t, err, "iteration %d", i)
		assert.Greater(t, mid, lo)
		assert.Less(t, mid, hi)
		hi = mid // keep narrowing the same gap — the hardest case
	}
}

func TestBetweenRejectsPrevNotBeforeNext(t *testing.T) {
	_, err := Between("b", "a")
	assert.Error(t, err)

	_, err = Between("a", "a")
	assert.Error(t, err)
}

func TestBetweenReturnsErrNoRoomAtTheGenuineFloor(t *testing.T) {
	// next made entirely of the alphabet minimum, with no lower bound: the
	// one case nothing can be inserted below (see ErrNoRoom's doc comment).
	_, err := Between("", "0")
	assert.ErrorIs(t, err, ErrNoRoom)

	_, err = Between("a", "a0")
	assert.ErrorIs(t, err, ErrNoRoom)
}

func TestBetweenSucceedsWhenNextHasRoomPastLeadingZeros(t *testing.T) {
	// "0i" legitimately occurs (e.g. Between("", "1") can produce it) —
	// leading '0' alone isn't the floor; only an next that's ALL zeros is.
	key, err := Between("", "0i")
	require.NoError(t, err)
	assert.Greater(t, key, "")
	assert.Less(t, key, "0i")
}

// TestPropertyBetweenOrdering is the load-bearing check: for any prev, next
// this package itself generated (so ErrNoRoom's precondition can't have
// been violated), Between(prev, next) — when it succeeds — always returns
// a key strictly between them.
func TestPropertyBetweenOrdering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		keys := []string{}
		steps := rapid.IntRange(1, 30).Draw(t, "steps")

		for i := 0; i < steps; i++ {
			var prev, next string
			switch {
			case len(keys) == 0:
				prev, next = "", ""
			default:
				idx := rapid.IntRange(0, len(keys)-1).Draw(t, "idx")
				mode := rapid.IntRange(0, 2).Draw(t, "mode")
				switch mode {
				case 0:
					prev, next = "", keys[idx]
				case 1:
					prev, next = keys[idx], ""
				default:
					if idx == len(keys)-1 {
						prev, next = keys[idx], ""
					} else {
						prev, next = keys[idx], keys[idx+1]
					}
				}
			}

			key, err := Between(prev, next)
			if err != nil {
				continue // ErrNoRoom is a defensive, not a routine, path
			}
			if prev != "" && key <= prev {
				t.Fatalf("Between(%q, %q) = %q, not > prev", prev, next, key)
			}
			if next != "" && key >= next {
				t.Fatalf("Between(%q, %q) = %q, not < next", prev, next, key)
			}

			keys = insertSorted(keys, key)
		}
	})
}

func insertSorted(keys []string, key string) []string {
	i := 0
	for i < len(keys) && keys[i] < key {
		i++
	}
	out := make([]string, 0, len(keys)+1)
	out = append(out, keys[:i]...)
	out = append(out, key)
	out = append(out, keys[i:]...)
	return out
}
