// Package documentcore is the pure, in-memory document model: pages, blocks,
// the op ISA that mutates them, and per-actor undo/redo history. It has no
// database, network, or storage dependency — RFC-001 (document model) and
// RFC-002 (operation model) are the spec this package implements.
package documentcore

import "github.com/google/uuid"

// PageID identifies a page. DATA_MODEL.md fixes this as a UUID (v7 at
// generation time, for index locality) — documentcore never manufactures
// one itself, only ever receives it, the same reasoning CLOUD_PORTABILITY.md
// applies to Clock: randomness is ambient authority a pure model shouldn't
// reach for.
type PageID uuid.UUID

// BlockID identifies a block, scoped to its page. Same sourcing rule as
// PageID.
type BlockID uuid.UUID

func (id PageID) String() string  { return uuid.UUID(id).String() }
func (id BlockID) String() string { return uuid.UUID(id).String() }

// MarshalText/UnmarshalText (not inherited from uuid.UUID — a defined type
// doesn't inherit its underlying type's methods) give PageID/BlockID plain
// UUID-string JSON encoding, the same shape DATA_MODEL.md's JSONB columns
// and the WASM/JS boundary both expect.
func (id PageID) MarshalText() ([]byte, error)  { return uuid.UUID(id).MarshalText() }
func (id *PageID) UnmarshalText(b []byte) error { return (*uuid.UUID)(id).UnmarshalText(b) }

func (id BlockID) MarshalText() ([]byte, error)  { return uuid.UUID(id).MarshalText() }
func (id *BlockID) UnmarshalText(b []byte) error { return (*uuid.UUID)(id).UnmarshalText(b) }
