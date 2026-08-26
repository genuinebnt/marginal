// Package oplog is RFC-002 §4's permanent wire format: the envelope every
// op — either ISA tier, internal/pageop's union — is wrapped in before it
// touches the WAL or collab.ops. This encoding can never break — it's
// replayed for history and read by every future release, so once a
// field's meaning ships it's fixed forever; only additive change is
// allowed (RFC-002 §4 rules 1-4).
package oplog

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"marginal/collaboration-service/internal/pageop"
)

// Version is the current encoding version, stamped on every record from
// op #1 (RFC-002 §4 rule 1) — never inferred, never defaulted after the
// fact, so a decoder always knows which format a stored row is in.
const Version uint16 = 1

var (
	ErrUnsupportedVersion = errors.New("oplog: unsupported encoding version")
	ErrInvalidActorKind   = errors.New("oplog: invalid actor kind")
)

// ActorKind matches DATA_MODEL.md's collab.ops.actor_kind CHECK constraint
// exactly — kept as a Go type (not a bare string) so a typo is a compile
// error in Go code, even though the wire/DB representation stays plain
// text (extending a CHECK is ordinary DDL; extending an enum is not, so the
// column itself is deliberately TEXT — DATA_MODEL.md's note on this column).
type ActorKind string

const (
	ActorUser   ActorKind = "user"
	ActorAgent  ActorKind = "agent"
	ActorPlugin ActorKind = "plugin"
	ActorSystem ActorKind = "system"
)

func (k ActorKind) Valid() bool {
	switch k {
	case ActorUser, ActorAgent, ActorPlugin, ActorSystem:
		return true
	default:
		return false
	}
}

// VectorClock is this actor's own monotonic counter per page, keyed by
// actor id (as a string) — sufficient for the dedup/causal-ordering needs
// of a single doc-actor process (RFC-002 §9's Merkle-diff reconciliation
// and §10's dedup are a later increment; this is the field they'll read,
// not their implementation).
type VectorClock map[string]uint64

// LoggedOp is RFC-002 §4's LoggedOp struct, ported field-for-field:
// id/version/actor/clock/op. PageID, ActorKind, and UndoGroup are this
// package's own additions, needed because collab.ops persists more than
// the RFC's minimal struct names (DATA_MODEL.md's two "must exist from op
// #1" columns — see that doc's note on why they're cheap now and not
// later).
type LoggedOp struct {
	ID          uuid.UUID // UUIDv7 — time-ordered, the dedup key (RFC-002 §4 rule 5)
	Version     uint16
	PageID      uuid.UUID
	ActorID     uuid.UUID
	ActorKind   ActorKind
	UndoGroup   *uuid.UUID // nil = a group of one (DATA_MODEL.md)
	VectorClock VectorClock
	Op          pageop.Op
	CreatedAt   time.Time
}

// New stamps a fresh LoggedOp with the current Version and a UUIDv7 id —
// the one place a LoggedOp gets constructed for a brand-new op, so every
// caller gets the versioning rule for free instead of having to remember it.
func New(pageID, actorID uuid.UUID, actorKind ActorKind, undoGroup *uuid.UUID, clock VectorClock, op pageop.Op) (LoggedOp, error) {
	if !actorKind.Valid() {
		return LoggedOp{}, ErrInvalidActorKind
	}
	id, err := uuid.NewV7()
	if err != nil {
		return LoggedOp{}, fmt.Errorf("oplog: generating id: %w", err)
	}
	return LoggedOp{
		ID:          id,
		Version:     Version,
		PageID:      pageID,
		ActorID:     actorID,
		ActorKind:   actorKind,
		UndoGroup:   undoGroup,
		VectorClock: clock,
		Op:          op,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// wireLoggedOp is the actual JSON shape — Op is boxed through pageop.Marshal
// (its own "type"-tagged envelope) rather than json.Marshal's default
// interface handling, since encoding/json can't dispatch an interface field
// back to a concrete type on decode without help.
type wireLoggedOp struct {
	ID          uuid.UUID       `json:"id"`
	Version     uint16          `json:"version"`
	PageID      uuid.UUID       `json:"page_id"`
	ActorID     uuid.UUID       `json:"actor_id"`
	ActorKind   ActorKind       `json:"actor_kind"`
	UndoGroup   *uuid.UUID      `json:"undo_group,omitempty"`
	VectorClock VectorClock     `json:"vector_clock"`
	Op          json.RawMessage `json:"op"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Marshal encodes l as the permanent wire format. Every record written this
// way must decode forever (RFC-002 §4 rule 4) — Unmarshal is the other half
// of that promise.
func Marshal(l LoggedOp) ([]byte, error) {
	opJSON, err := pageop.Marshal(l.Op)
	if err != nil {
		return nil, fmt.Errorf("oplog: marshaling op: %w", err)
	}
	return json.Marshal(wireLoggedOp{
		ID:          l.ID,
		Version:     l.Version,
		PageID:      l.PageID,
		ActorID:     l.ActorID,
		ActorKind:   l.ActorKind,
		UndoGroup:   l.UndoGroup,
		VectorClock: l.VectorClock,
		Op:          opJSON,
		CreatedAt:   l.CreatedAt,
	})
}

// Unmarshal decodes data written by Marshal at any past Version. Today
// there is only Version 1, so this rejects anything else outright — the
// day a Version 2 ships, this function grows a branch per version rather
// than replacing the Version-1 branch (RFC-002 §4 rule 4: old versions
// decode forever).
func Unmarshal(data []byte) (LoggedOp, error) {
	var wire wireLoggedOp
	if err := json.Unmarshal(data, &wire); err != nil {
		return LoggedOp{}, err
	}
	if wire.Version != Version {
		return LoggedOp{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, wire.Version)
	}
	op, err := pageop.Unmarshal(wire.Op)
	if err != nil {
		return LoggedOp{}, fmt.Errorf("oplog: unmarshaling op: %w", err)
	}
	return LoggedOp{
		ID:          wire.ID,
		Version:     wire.Version,
		PageID:      wire.PageID,
		ActorID:     wire.ActorID,
		ActorKind:   wire.ActorKind,
		UndoGroup:   wire.UndoGroup,
		VectorClock: wire.VectorClock,
		Op:          op,
		CreatedAt:   wire.CreatedAt,
	}, nil
}

// MarshalJSON/UnmarshalJSON delegate to Marshal/Unmarshal, so LoggedOp
// composes naturally as a field inside another JSON structure (e.g.
// internal/wsapi's server frames) via encoding/json's normal interface
// dispatch, instead of every caller needing to know to call Marshal
// itself and nest the resulting raw bytes by hand.
func (l LoggedOp) MarshalJSON() ([]byte, error) { return Marshal(l) }

func (l *LoggedOp) UnmarshalJSON(data []byte) error {
	v, err := Unmarshal(data)
	if err != nil {
		return err
	}
	*l = v
	return nil
}
