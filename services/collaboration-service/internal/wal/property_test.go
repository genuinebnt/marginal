package wal

import (
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

// TestPropertyRecoverReplaysExactlyWhatWasAppended drives a Writer through a
// random sequence of variable-length records (including empty ones) and
// requires Recover to play them back byte-for-byte, in order — the
// baseline correctness property everything else (torn-tail handling,
// checksum detection) is a deliberate exception to.
func TestPropertyRecoverReplaysExactlyWhatWasAppended(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "wal-prop-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()
		path := filepath.Join(dir, "prop.wal")

		w, err := OpenWriter(path)
		if err != nil {
			t.Fatalf("OpenWriter: %v", err)
		}

		n := rapid.IntRange(0, 20).Draw(t, "n")
		var want [][]byte
		for i := 0; i < n; i++ {
			size := rapid.IntRange(0, 64).Draw(t, "size")
			data := rapid.SliceOfN(rapid.Byte(), size, size).Draw(t, "data")
			if err := w.Append(data); err != nil {
				t.Fatalf("Append: %v", err)
			}
			want = append(want, data)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		var got [][]byte
		validUpTo, err := Recover(path, func(record []byte) error {
			got = append(got, append([]byte(nil), record...))
			return nil
		})
		if err != nil {
			t.Fatalf("Recover: %v", err)
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("Stat: %v", statErr)
		}
		if validUpTo != info.Size() {
			t.Fatalf("validUpTo = %d, want %d (whole file, no torn tail)", validUpTo, info.Size())
		}

		if len(got) != len(want) {
			t.Fatalf("replayed %d records, want %d", len(got), len(want))
		}
		for i := range want {
			if string(got[i]) != string(want[i]) {
				t.Fatalf("record %d = %q, want %q", i, got[i], want[i])
			}
		}
	})
}
