package wal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func walPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.wal")
}

func TestAppendAndRecoverRoundTrip(t *testing.T) {
	path := walPath(t)
	w, err := OpenWriter(path)
	require.NoError(t, err)

	records := [][]byte{[]byte("first"), []byte("second"), []byte("third")}
	for _, r := range records {
		require.NoError(t, w.Append(r))
	}
	require.NoError(t, w.Close())

	var got [][]byte
	validUpTo, err := Recover(path, func(record []byte) error {
		got = append(got, append([]byte(nil), record...))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, records, got)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), validUpTo, "no torn tail: validUpTo covers the whole file")
}

func TestRecoverOnMissingFileIsEmptyNotError(t *testing.T) {
	got, err := Recover(filepath.Join(t.TempDir(), "does-not-exist.wal"), func([]byte) error {
		t.Fatal("fn must not be called for a file that never existed")
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), got)
}

// TestRecoverResyncsPastATornTailRecord is the scenario RFC-002 §6 names
// explicitly: a SIGKILL mid-write leaves a partial record at the end of
// the file. Recovery must replay every complete record before it and stop
// cleanly there, not fail the whole replay.
func TestRecoverResyncsPastATornTailRecord(t *testing.T) {
	path := walPath(t)
	w, err := OpenWriter(path)
	require.NoError(t, err)
	require.NoError(t, w.Append([]byte("complete-one")))
	require.NoError(t, w.Append([]byte("complete-two")))
	require.NoError(t, w.Close())

	info, err := os.Stat(path)
	require.NoError(t, err)
	completeSize := info.Size()

	// Simulate a crash mid-write of a third record: append a length
	// prefix claiming more bytes than actually follow, then nothing else.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.Write([]byte{0, 0, 1, 0}) // claims a 256-byte record
	require.NoError(t, err)
	_, err = f.Write([]byte("only a few bytes")) // far short of 256
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var got [][]byte
	validUpTo, err := Recover(path, func(record []byte) error {
		got = append(got, append([]byte(nil), record...))
		return nil
	})
	require.NoError(t, err, "a torn tail must not surface as an error")
	assert.Equal(t, [][]byte{[]byte("complete-one"), []byte("complete-two")}, got)
	assert.Equal(t, completeSize, validUpTo, "validUpTo must point right after the last complete record")

	require.NoError(t, Truncate(path, validUpTo))
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, completeSize, info.Size())

	// A fresh Writer resuming after Truncate must produce a clean,
	// fully-recoverable file — proves the torn tail is really gone, not
	// just skipped over.
	w2, err := OpenWriter(path)
	require.NoError(t, err)
	require.NoError(t, w2.Append([]byte("complete-three")))
	require.NoError(t, w2.Close())

	got = nil
	_, err = Recover(path, func(record []byte) error {
		got = append(got, append([]byte(nil), record...))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, [][]byte{[]byte("complete-one"), []byte("complete-two"), []byte("complete-three")}, got)
}

func TestRecoverDetectsChecksumMismatchAsCorruptionNotATornWrite(t *testing.T) {
	path := walPath(t)
	w, err := OpenWriter(path)
	require.NoError(t, err)
	require.NoError(t, w.Append([]byte("hello")))
	require.NoError(t, w.Close())

	// Flip a byte inside the record body without touching the length or
	// checksum — a complete, correctly-sized frame whose content
	// disagrees with its own checksum: corruption, not a crash mid-write.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data[4] ^= 0xFF
	require.NoError(t, os.WriteFile(path, data, 0o600))

	_, err = Recover(path, func([]byte) error { return nil })
	assert.ErrorIs(t, err, ErrChecksum)
}

func TestAppendRejectsOversizedRecord(t *testing.T) {
	w, err := OpenWriter(walPath(t))
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	err = w.Append(make([]byte, maxRecordLen+1))
	assert.ErrorIs(t, err, ErrRecordTooLarge)
}

func TestRecoverPropagatesReplayFnError(t *testing.T) {
	path := walPath(t)
	w, err := OpenWriter(path)
	require.NoError(t, err)
	require.NoError(t, w.Append([]byte("one")))
	require.NoError(t, w.Append([]byte("two")))
	require.NoError(t, w.Close())

	callCount := 0
	sentinel := assertError("replay stopped here")
	_, err = Recover(path, func([]byte) error {
		callCount++
		if callCount == 1 {
			return sentinel
		}
		return nil
	})
	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, callCount, "recovery must stop at the first replay error, not continue past it")
}

type assertError string

func (e assertError) Error() string { return string(e) }
