package sortkey

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestBetweenProducesKeyWithinBounds(t *testing.T) {
	tests := []struct {
		name       string
		prev, next string
		wantExact  string
		checkLower bool
		checkUpper bool
	}{
		{name: "empty bounds returns mid-alphabet key", prev: "", next: "", wantExact: "i"},
		{name: "no lower bound", prev: "", next: "i", checkLower: true, checkUpper: true},
		{name: "no upper bound", prev: "i", next: "", checkLower: true},
		{name: "two keys with room", prev: "a1", next: "a3", checkLower: true, checkUpper: true},
		{name: "adjacent single-char keys", prev: "i", next: "j", checkLower: true, checkUpper: true},
		// "0i" legitimately occurs (e.g. Between("", "1") can produce it) —
		// leading '0' alone isn't the floor; only a next that's ALL zeros is
		// (see the ErrNoRoom cases below).
		{name: "succeeds when next has room past leading zeros", prev: "", next: "0i", checkLower: true, checkUpper: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, err := Between(tc.prev, tc.next)
			require.NoError(t, err)
			if tc.wantExact != "" {
				assert.Equal(t, tc.wantExact, key)
			}
			if tc.checkLower {
				assert.Greater(t, key, tc.prev)
			}
			if tc.checkUpper {
				assert.Less(t, key, tc.next)
			}
		})
	}
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
	tests := []struct {
		name       string
		prev, next string
	}{
		{name: "prev after next", prev: "b", next: "a"},
		{name: "prev equal to next", prev: "a", next: "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Between(tc.prev, tc.next)
			assert.Error(t, err)
		})
	}
}

// TestBetweenReturnsErrNoRoomAtTheGenuineFloor covers a next made entirely
// of the alphabet minimum, with no lower bound: the one case nothing can
// be inserted below (see ErrNoRoom's doc comment).
func TestBetweenReturnsErrNoRoomAtTheGenuineFloor(t *testing.T) {
	tests := []struct {
		name       string
		prev, next string
	}{
		{name: "no lower bound, next is the alphabet minimum", prev: "", next: "0"},
		{name: "next is prev plus the alphabet minimum", prev: "a", next: "a0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Between(tc.prev, tc.next)
			assert.ErrorIs(t, err, ErrNoRoom)
		})
	}
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
