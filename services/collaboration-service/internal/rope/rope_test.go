package rope

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestNewAndStringRoundTrip(t *testing.T) {
	r := New("hello world")
	assert.Equal(t, "hello world", r.String())
	assert.Equal(t, 11, r.Len())
}

func TestEmptyRope(t *testing.T) {
	var r Rope
	assert.Equal(t, "", r.String())
	assert.Equal(t, 0, r.Len())
}

func TestInsertAtStartMiddleEnd(t *testing.T) {
	r := New("hello")

	start, err := r.Insert(0, ">>")
	require.NoError(t, err)
	assert.Equal(t, ">>hello", start.String())

	middle, err := r.Insert(2, "-")
	require.NoError(t, err)
	assert.Equal(t, "he-llo", middle.String())

	end, err := r.Insert(5, "!")
	require.NoError(t, err)
	assert.Equal(t, "hello!", end.String())
}

func TestInsertDoesNotMutateTheOriginal(t *testing.T) {
	r := New("hello")
	_, err := r.Insert(0, "X")
	require.NoError(t, err)
	assert.Equal(t, "hello", r.String(), "Rope is immutable — Insert must not affect the receiver")
}

func TestDeleteRange(t *testing.T) {
	r := New("hello world")
	got, err := r.Delete(5, 11)
	require.NoError(t, err)
	assert.Equal(t, "hello", got.String())
}

func TestDeleteEntireRope(t *testing.T) {
	r := New("hello")
	got, err := r.Delete(0, 5)
	require.NoError(t, err)
	assert.Equal(t, "", got.String())
	assert.Equal(t, 0, got.Len())
}

func TestSlice(t *testing.T) {
	r := New("hello world")
	got, err := r.Slice(6, 11)
	require.NoError(t, err)
	assert.Equal(t, "world", got)
}

func TestInsertRejectsOutOfBounds(t *testing.T) {
	r := New("hello")
	_, err := r.Insert(99, "x")
	assert.ErrorIs(t, err, ErrOutOfBounds)
	_, err = r.Insert(-1, "x")
	assert.ErrorIs(t, err, ErrOutOfBounds)
}

func TestDeleteRejectsInvertedRange(t *testing.T) {
	r := New("hello")
	_, err := r.Delete(4, 1)
	assert.ErrorIs(t, err, ErrInvertedRange)
}

func TestRejectsSplittingAMultibyteCharacter(t *testing.T) {
	r := New("héllo") // 'é' is 2 bytes: h(1) é(2) l l o
	_, err := r.Insert(2, "x")
	assert.ErrorIs(t, err, ErrNotCharBoundary)

	_, err = r.Delete(0, 2)
	assert.ErrorIs(t, err, ErrNotCharBoundary)
}

func TestAcceptsBoundariesAroundAMultibyteCharacter(t *testing.T) {
	r := New("héllo")
	got, err := r.Insert(1, "X") // right before 'é', a valid boundary
	require.NoError(t, err)
	assert.Equal(t, "hXéllo", got.String())

	got, err = r.Delete(1, 3) // exactly the 'é'
	require.NoError(t, err)
	assert.Equal(t, "hllo", got.String())
}

func TestBuildSplitsLargeInputIntoABalancedTree(t *testing.T) {
	big := make([]byte, maxLeafBytes*50)
	for i := range big {
		big[i] = 'a'
	}
	r := New(string(big))
	assert.Equal(t, len(big), r.Len())
	assert.Equal(t, string(big), r.String())
	// log2(50) ~= 5.6; a reasonably balanced tree over ~50 leaves should
	// have a depth in the same ballpark, nowhere near 50.
	assert.LessOrEqual(t, r.Depth(), 12, "a freshly built rope over many leaves must be balanced, not a linked list")
}

func TestManyInsertsStayBalanced(t *testing.T) {
	r := New("")
	for i := 0; i < 2000; i++ {
		var err error
		r, err = r.Insert(r.Len(), "x")
		require.NoError(t, err)
	}
	require.Equal(t, 2000, r.Len())
	// Repeated append-at-end is exactly the pattern that degrades an
	// unbalanced tree to a linked list (depth == count); the rebalancing
	// threshold must keep this near log2(2000) ~= 11, not 2000.
	assert.LessOrEqual(t, r.Depth(), 40, "2000 sequential appends must not degrade the rope to near-linear depth")
}

// TestPropertyMatchesNaiveStringReference is the standard way to test a
// rope: apply the same sequence of inserts/deletes to a Rope and to a
// plain Go string, and require they always produce the same result. If
// the rope's split/concat/rebalance logic is wrong, this is what catches
// it — unlike a handful of example-based tests, which only catch the
// cases someone thought to write down (Skiena ch. 1 on why differential
// testing against an obviously-correct reference is worth doing whenever
// one exists).
func TestPropertyMatchesNaiveStringReference(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		reference := ""
		r := New("")
		steps := rapid.IntRange(1, 60).Draw(t, "steps")

		for i := 0; i < steps; i++ {
			if len(reference) == 0 || rapid.Bool().Draw(t, "insert") {
				at := rapid.IntRange(0, len(reference)).Draw(t, "at")
				// Keep offsets on rune boundaries so the reference and
				// the rope are validating the same well-formed input;
				// off-boundary rejection is already covered by dedicated
				// tests above.
				at = runeBoundaryAtOrBefore(reference, at)
				text := rapid.StringN(0, 8, -1).Draw(t, "text")

				newReference := reference[:at] + text + reference[at:]
				newRope, err := r.Insert(at, text)
				if err != nil {
					t.Fatalf("Insert(%d, %q) on %q failed: %v", at, text, reference, err)
				}
				reference, r = newReference, newRope
			} else {
				a := runeBoundaryAtOrBefore(reference, rapid.IntRange(0, len(reference)).Draw(t, "a"))
				b := runeBoundaryAtOrBefore(reference, rapid.IntRange(0, len(reference)).Draw(t, "b"))
				if a > b {
					a, b = b, a
				}
				newReference := reference[:a] + reference[b:]
				newRope, err := r.Delete(a, b)
				if err != nil {
					t.Fatalf("Delete(%d, %d) on %q failed: %v", a, b, reference, err)
				}
				reference, r = newReference, newRope
			}

			if r.String() != reference {
				t.Fatalf("rope diverged from reference:\nrope:      %q\nreference: %q", r.String(), reference)
			}
			if r.Len() != len(reference) {
				t.Fatalf("rope length %d != reference length %d", r.Len(), len(reference))
			}
		}
	})
}

func runeBoundaryAtOrBefore(s string, at int) int {
	for at > 0 && at <= len(s) && !isBoundary(s, at) {
		at--
	}
	return at
}

func isBoundary(s string, at int) bool {
	if at == 0 || at == len(s) {
		return true
	}
	return s[at]&0xC0 != 0x80
}
