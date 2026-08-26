// Package wal is collaboration-service's local durability layer
// (ARCHITECTURE.md's collaboration-service sequence diagram, RFC-002 §6):
// a client's op is acknowledged only after it is fsync'd to this on-disk
// log, not after Postgres confirms — durability without paying database
// latency per keystroke. Postgres persistence (collab.ops) is a separate,
// batched flush that reads from this log later; that flush loop lives with
// the session layer (docs/porting/PROGRESS.md "Next"), not here.
//
// Record framing, byte-for-byte per RFC-002 §6 and §8 (deliberately the
// same framing on both the WAL and the wire — one encoder, one decoder,
// one set of tests):
//
//	[4-byte big-endian length][record bytes][4-byte crc32 (IEEE) of the record bytes]
//
// what a "record" holds is opaque to this package — the caller passes
// already-encoded bytes (an oplog.Marshal result, in practice) and gets
// them back unchanged on replay. RFC-002 §8's rkyv zero-copy decode is
// Rust-specific serialization machinery with no Go equivalent worth
// building for an MVP's write volume; this package only owns the
// durability framing, not the payload encoding.
//
// One Writer is one segment (one file, one page's session) — RFC-002 §6's
// 64MB rotation-then-delete-once-flushed scheme isn't implemented yet.
// That's a real, deliberate gap for this repo's scope: a demo session's
// single-page op log isn't expected to approach 64MB, so multi-segment
// bookkeeping (rotating, tracking which segments Postgres has confirmed,
// deleting the rest) would be complexity spent on a problem this MVP
// doesn't actually have yet. Add it if a page's WAL file is ever observed
// to grow unbounded in practice, not preemptively.
package wal

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// maxRecordLen guards against a corrupt length prefix causing an
// unbounded read — a torn write can leave any 4 bytes at a record
// boundary, and this package's recovery must never trust them blindly.
// 64 MiB is generous for a single op record; it's a sanity bound, not a
// tuned limit.
const maxRecordLen = 64 << 20

var (
	ErrRecordTooLarge = errors.New("wal: record exceeds maxRecordLen")
	ErrTornRecord     = errors.New("wal: torn record at end of file (expected after an unclean shutdown)")
	ErrChecksum       = errors.New("wal: checksum mismatch (corrupt record)")
)

// Writer appends framed records to one open segment file, syncing after
// every write — the whole point of a WAL is that a write isn't durable
// until this returns nil.
type Writer struct {
	f *os.File
}

// OpenWriter opens path for appending, creating it if it doesn't exist.
// Existing content (a prior session's segment) is left alone — recovery
// reads it via Recover before a fresh Writer starts appending past it.
func OpenWriter(path string) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("wal: opening %s: %w", path, err)
	}
	return &Writer{f: f}, nil
}

// Append writes one framed record and fsyncs before returning. The caller
// must not treat the op as durable — must not acknowledge the client —
// until this returns nil (RFC-002 §6).
func (w *Writer) Append(record []byte) error {
	if len(record) > maxRecordLen {
		return ErrRecordTooLarge
	}

	frame := make([]byte, 4+len(record)+4)
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(record)))
	copy(frame[4:], record)
	sum := crc32.ChecksumIEEE(record)
	binary.BigEndian.PutUint32(frame[4+len(record):], sum)

	if _, err := w.f.Write(frame); err != nil {
		return fmt.Errorf("wal: writing record: %w", err)
	}
	// Sync, not just flush an in-process buffer — durability means the
	// bytes survived a crash, and only the OS/disk confirming a sync can
	// promise that. Measuring sync_data() (fdatasync, skips inode metadata
	// like mtime) against a full sync is the RFC-002 §6 tradeoff this
	// leaves as a later optimization; Go's os.File.Sync() maps to fsync,
	// the safe default to start correct from.
	if err := w.f.Sync(); err != nil {
		return fmt.Errorf("wal: syncing: %w", err)
	}
	return nil
}

// Close closes the underlying file. It does not sync — the last Append
// already did.
func (w *Writer) Close() error { return w.f.Close() }

// Recover replays every valid record in path in order, calling fn for
// each. This is a recovering parser, not a strict one (RFC-002 §6): a
// torn tail — a partial record left by a crash mid-write — is expected,
// not an error, and recovery stops cleanly at the first one rather than
// failing the whole replay. A checksum mismatch on a record that claims a
// complete, correctly-sized frame is different: that's corruption, not a
// torn write, and IS reported.
//
// Recover returns the byte offset immediately after the last valid
// record, and everything replayed decoded cleanly. session.open is the
// one caller: it reconciles any record recovered here against what's
// already confirmed in Postgres, reflushes the difference, then deletes
// this whole segment file outright and opens a fresh one — simpler than
// truncating this file in place to validUpTo and resuming appends to it,
// since a session only ever recovers a segment once, at open, not
// repeatedly.
func Recover(path string, fn func(record []byte) error) (validUpTo int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("wal: opening %s for recovery: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := bufio.NewReader(f)
	var offset int64

	for {
		lenBuf := make([]byte, 4)
		n, readErr := io.ReadFull(r, lenBuf)
		if readErr != nil {
			if isTornBoundary(readErr, n) {
				return offset, nil
			}
			return offset, fmt.Errorf("wal: reading length prefix: %w", readErr)
		}
		recordLen := binary.BigEndian.Uint32(lenBuf)
		if recordLen > maxRecordLen {
			// A length this large can only be a torn/corrupt boundary —
			// no real record is ever written that big (Append rejects it
			// before it reaches disk) — so treat it as the torn tail, not
			// a fatal error.
			return offset, nil
		}

		body := make([]byte, recordLen)
		n, readErr = io.ReadFull(r, body)
		if readErr != nil {
			if isTornBoundary(readErr, n) {
				return offset, nil
			}
			return offset, fmt.Errorf("wal: reading record body: %w", readErr)
		}

		sumBuf := make([]byte, 4)
		n, readErr = io.ReadFull(r, sumBuf)
		if readErr != nil {
			if isTornBoundary(readErr, n) {
				return offset, nil
			}
			return offset, fmt.Errorf("wal: reading checksum: %w", readErr)
		}

		wantSum := binary.BigEndian.Uint32(sumBuf)
		if crc32.ChecksumIEEE(body) != wantSum {
			return offset, ErrChecksum
		}

		if err := fn(body); err != nil {
			return offset, fmt.Errorf("wal: replaying record at offset %d: %w", offset, err)
		}
		offset += int64(4 + recordLen + 4)
	}
}

// isTornBoundary reports whether readErr from io.ReadFull represents a
// clean or partial end-of-file — both are what an unclean shutdown leaves
// behind, never a real error to surface. n > 0 with io.ErrUnexpectedEOF is
// the partial case: some bytes of this frame's next field made it to
// disk, but not all of them.
func isTornBoundary(readErr error, n int) bool {
	if errors.Is(readErr, io.EOF) && n == 0 {
		return true
	}
	return errors.Is(readErr, io.ErrUnexpectedEOF)
}
