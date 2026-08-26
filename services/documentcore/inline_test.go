package documentcore

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests run testdata/document-core/marks.json — a behavior spec
// shared with the TypeScript suite (web/src/document-core) and, later, the
// Rust port. See .agents/agents.md's testing-philosophy section: the goal
// is that a port can reuse the test *cases*, not the test code.

type markVector struct {
	Name        string          `json:"name"`
	Text        string          `json:"text"`
	Ops         []markOp        `json:"ops"`
	ExpectError *markVectorErr  `json:"expect_error"`
	ExpectMarks []markVectorRef `json:"expect_marks"`
	Queries     []markQuery     `json:"queries"`
}

type markOp struct {
	Op    string   `json:"op"` // "add" | "remove"
	Kind  MarkKind `json:"kind"`
	Start int      `json:"start"`
	End   int      `json:"end"`
}

type markVectorRef struct {
	Kind  MarkKind `json:"kind"`
	Start int      `json:"start"`
	End   int      `json:"end"`
}

type markVectorErr struct {
	Type   string `json:"type"`
	Start  *int   `json:"start"`
	End    *int   `json:"end"`
	Offset *int   `json:"offset"`
	Len    *int   `json:"len"`
}

type markQuery struct {
	Offset int        `json:"offset"`
	Expect []MarkKind `json:"expect"`
}

func loadMarkVectors(t *testing.T) []markVector {
	t.Helper()
	data, err := os.ReadFile("../../testdata/document-core/marks.json")
	require.NoError(t, err)
	var vectors []markVector
	require.NoError(t, json.Unmarshal(data, &vectors))
	return vectors
}

func assertMarkError(t *testing.T, want *markVectorErr, got error) {
	t.Helper()
	switch want.Type {
	case "InvertedRange":
		var e *InvertedRangeError
		require.True(t, errors.As(got, &e), "want InvertedRangeError, got %v", got)
		require.Equal(t, *want.Start, e.Start)
		require.Equal(t, *want.End, e.End)
	case "NotCharBoundary":
		var e *NotCharBoundaryError
		require.True(t, errors.As(got, &e), "want NotCharBoundaryError, got %v", got)
		require.Equal(t, *want.Offset, e.Offset)
	case "OutOfBounds":
		var e *OutOfBoundsError
		require.True(t, errors.As(got, &e), "want OutOfBoundsError, got %v", got)
		require.Equal(t, *want.Offset, e.Offset)
		require.Equal(t, *want.Len, e.Len)
	default:
		t.Fatalf("unknown expected error type in test vector: %s", want.Type)
	}
}

func TestContentMarkVectors(t *testing.T) {
	for _, v := range loadMarkVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			c := PlainContent(v.Text)

			var lastErr error
			for _, op := range v.Ops {
				switch op.Op {
				case "add":
					lastErr = c.AddMark(op.Kind, op.Start, op.End)
				case "remove":
					lastErr = c.RemoveMark(op.Kind, op.Start, op.End)
				default:
					t.Fatalf("unknown op %q in test vector", op.Op)
				}
				if lastErr != nil {
					break
				}
			}

			if v.ExpectError != nil {
				assertMarkError(t, v.ExpectError, lastErr)
			} else {
				require.NoError(t, lastErr)
			}

			require.Len(t, c.Marks, len(v.ExpectMarks))
			for i, want := range v.ExpectMarks {
				require.Equal(t, want.Kind, c.Marks[i].Kind, "mark %d kind", i)
				require.Equal(t, want.Start, c.Marks[i].Start, "mark %d start", i)
				require.Equal(t, want.End, c.Marks[i].End, "mark %d end", i)
			}

			for _, q := range v.Queries {
				got := c.MarksAt(q.Offset)
				require.Len(t, got, len(q.Expect), "marks_at(%d)", q.Offset)
				for i, want := range q.Expect {
					require.Equal(t, want, got[i], "marks_at(%d)[%d]", q.Offset, i)
				}
			}
		})
	}
}
