package rope

import (
	"strings"
	"testing"
)

// These benchmarks exist to demonstrate — and, over time, to catch a
// regression in — the whole reason a rope exists over a plain string:
// inserting into the middle of a large document is O(log n), not O(n).
// Run with: go test ./internal/rope/... -bench=. -benchmem -run=^$

func BenchmarkRopeInsertMiddle(b *testing.B) {
	text := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 20000) // ~900KB
	r := New(text)
	mid := r.Len() / 2

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Insert(mid, "x"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNaiveStringInsertMiddle is the same operation on a plain Go
// string, via slicing and concatenation — the thing a rope exists to
// beat. Expect this to scale with the document's size; BenchmarkRopeInsertMiddle
// should not.
func BenchmarkNaiveStringInsertMiddle(b *testing.B) {
	text := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 20000)
	mid := len(text) / 2

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = text[:mid] + "x" + text[mid:]
	}
}

func BenchmarkRopeStringManySequentialInserts(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := New("")
		for j := 0; j < 1000; j++ {
			var err error
			r, err = r.Insert(r.Len(), "x")
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
