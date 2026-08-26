// Package pageop is a live session's op union: RFC-002 §2's two ISA tiers,
// reconciled into the one stream internal/session actually applies, logs,
// and broadcasts. A Block op is documentcore.Op — whole-page-scope,
// structural (InsertBlock, DeleteBlock, SetBlockKind, SetBlockContent,
// SetTitle, MoveBlock) — applied against the session's single
// documentcore.Page. A Text op is collaboration-service's own ops.Op,
// scoped to exactly one block's own live rope (internal/doctext) — the
// character-granular tier a Page alone can't represent, per RFC-002 §2.1.
//
// Session no longer holds one rope for the whole page; it holds a
// documentcore.Page for structure plus one doctext.Text per block, keyed
// by BlockID. Every block-structure op and every character op inside any
// block passes through this same LoggedOp stream — one page, one serial
// order, one WAL, one flush pipeline, regardless of which tier an op
// belongs to (docs/architecture/DATA_MODEL.md § collab.ops → docs.blocks).
package pageop

import (
	"encoding/json"
	"fmt"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/ops"
)

// Op is one client-submitted operation against a live page session.
type Op interface{ isPageOp() }

// Block is a structural op — insert/delete/reorder a block, change its
// kind, replace its content wholesale, or rename the page. Applied against
// the session's documentcore.Page; documentcore.Op.Invert() computes its
// own inverse from its own fields, unlike Text below.
type Block struct{ Op documentcore.Op }

func (Block) isPageOp() {}

// Text is a character-granular op scoped to one block's own live rope —
// InsertText/DeleteText/NoOp, applied against session.blocks[BlockID].
type Text struct {
	BlockID documentcore.BlockID
	Op      ops.Op
}

func (Text) isPageOp() {}

// TypeName is op's variant name, prefixed by its tier ("block:"/"text:")
// so collab.ops.kind (DATA_MODEL.md) can tell the two tiers apart without
// decoding the payload first.
func TypeName(op Op) (string, error) {
	switch op := op.(type) {
	case Block:
		name, err := documentcore.TypeName(op.Op)
		if err != nil {
			return "", err
		}
		return "block:" + name, nil
	case Text:
		name, err := ops.TypeName(op.Op)
		if err != nil {
			return "", err
		}
		return "text:" + name, nil
	default:
		return "", fmt.Errorf("pageop: unknown op type %T", op)
	}
}

// wireText is Text's JSON shape: BlockID alongside the inner ops.Op's own
// type-tagged envelope (ops.MarshalOp/UnmarshalOp), not a generic
// re-encoding of it.
type wireText struct {
	Block documentcore.BlockID `json:"block"`
	Op    json.RawMessage      `json:"op"`
}

// Marshal and Unmarshal are pageop.Op's wire encoding — the envelope
// internal/oplog frames into the WAL and collab.ops.payload. Block ops
// pass straight through to documentcore.MarshalOp/UnmarshalOp (already a
// type-tagged envelope of its own); Text ops nest ops.MarshalOp/UnmarshalOp
// inside a small wrapper naming the block they apply to.
func Marshal(op Op) ([]byte, error) {
	switch op := op.(type) {
	case Block:
		inner, err := documentcore.MarshalOp(op.Op)
		if err != nil {
			return nil, err
		}
		// inner is already a JSON object (documentcore.MarshalOp's own
		// {"type": "...", ...} envelope) — splicing "scope" onto the
		// front of it is a byte-level insert, not the decode-modify-
		// reencode round trip through a map[string]json.RawMessage an
		// earlier version did (see documentcore.MarshalOp's own
		// spliceStringField, the same fix for the same reason).
		return spliceStringField(inner, "scope", "block")
	case Text:
		innerOp, err := ops.MarshalOp(op.Op)
		if err != nil {
			return nil, err
		}
		blockJSON, err := json.Marshal(op.BlockID)
		if err != nil {
			return nil, err
		}
		scopeJSON, err := json.Marshal("text")
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]json.RawMessage{
			"scope": scopeJSON,
			"block": blockJSON,
			"op":    innerOp,
		})
	default:
		return nil, fmt.Errorf("pageop: unknown op type %T", op)
	}
}

// spliceStringField adds one string-valued field to the front of an
// already-marshaled JSON object without decoding it — see
// documentcore.MarshalOp's identical helper for the full reasoning.
// Duplicated rather than shared across the module boundary, same call as
// ops.MarshalOp's own copy.
func spliceStringField(object []byte, key, value string) ([]byte, error) {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	keyJSON, err := json.Marshal(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(object)+len(keyJSON)+len(valueJSON)+2)
	out = append(out, '{')
	out = append(out, keyJSON...)
	out = append(out, ':')
	out = append(out, valueJSON...)
	if len(object) > 2 {
		out = append(out, ',')
	}
	out = append(out, object[1:]...)
	return out, nil
}

func Unmarshal(data []byte) (Op, error) {
	var envelope struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}

	switch envelope.Scope {
	case "block":
		inner, err := documentcore.UnmarshalOp(data)
		if err != nil {
			return nil, err
		}
		return Block{Op: inner}, nil
	case "text":
		var w wireText
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, err
		}
		inner, err := ops.UnmarshalOp(w.Op)
		if err != nil {
			return nil, err
		}
		return Text{BlockID: w.Block, Op: inner}, nil
	default:
		return nil, fmt.Errorf("pageop: unknown op scope %q", envelope.Scope)
	}
}
