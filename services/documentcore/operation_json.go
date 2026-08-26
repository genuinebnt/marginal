package documentcore

import (
	"encoding/json"
	"fmt"
)

// TypeName is op's variant name — the same string MarshalOp tags its JSON
// envelope with, and what collab.ops.kind (DATA_MODEL.md) stores when a
// documentcore.Op is logged directly by collaboration-service's block-op
// tier (internal/pageop there), matching the ops package's identical
// TypeName contract for character-granular ops.
func TypeName(op Op) (string, error) {
	switch op.(type) {
	case InsertBlock:
		return "InsertBlock", nil
	case DeleteBlock:
		return "DeleteBlock", nil
	case SetBlockKind:
		return "SetBlockKind", nil
	case SetBlockContent:
		return "SetBlockContent", nil
	case SetTitle:
		return "SetTitle", nil
	case MoveBlock:
		return "MoveBlock", nil
	default:
		return "", fmt.Errorf("documentcore: unknown op type %T", op)
	}
}

// MarshalOp and UnmarshalOp are Op's wire encoding: each variant's own JSON
// tags, plus a "type" field naming the variant — since Op is an interface,
// encoding/json can't dispatch on it by itself the way it can for
// BlockKind/MarkKind's tagged structs. This is the format the WASM/JS
// boundary uses, and the shape DATA_MODEL.md's collab.ops.kind column
// (stored as text) plus its op payload already assumes.
func MarshalOp(op Op) ([]byte, error) {
	typeName, err := TypeName(op)
	if err != nil {
		return nil, err
	}
	fields, err := json.Marshal(op)
	if err != nil {
		return nil, err
	}
	return spliceStringField(fields, "type", typeName)
}

// spliceStringField adds one string-valued field to the front of an
// already-marshaled JSON object, without decoding it — object is always
// a JSON object here (every Op variant is a struct), so this is a byte
// splice, not a decode-modify-reencode round trip through a
// map[string]json.RawMessage. An earlier version did exactly that:
// unmarshal into a map, add the field, marshal the whole map back out —
// on the op-marshaling path the WAL, collab.ops, and every WebSocket
// frame all go through, per op. Also changes nothing semantically: Go's
// own encoding/json sorts map keys when marshaling one, so the old
// version silently re-ordered every other field alphabetically as a
// side effect of the round trip; encoding/json's Unmarshal never cared
// about field order, so nothing downstream depended on that ordering
// either way.
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
	if len(object) > 2 { // object isn't just "{}" — it has fields of its own
		out = append(out, ',')
	}
	out = append(out, object[1:]...) // object sans its own leading '{'
	return out, nil
}

func UnmarshalOp(data []byte) (Op, error) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}

	switch envelope.Type {
	case "InsertBlock":
		var op InsertBlock
		err := json.Unmarshal(data, &op)
		return op, err
	case "DeleteBlock":
		var op DeleteBlock
		err := json.Unmarshal(data, &op)
		return op, err
	case "SetBlockKind":
		var op SetBlockKind
		err := json.Unmarshal(data, &op)
		return op, err
	case "SetBlockContent":
		var op SetBlockContent
		err := json.Unmarshal(data, &op)
		return op, err
	case "SetTitle":
		var op SetTitle
		err := json.Unmarshal(data, &op)
		return op, err
	case "MoveBlock":
		var op MoveBlock
		err := json.Unmarshal(data, &op)
		return op, err
	default:
		return nil, fmt.Errorf("documentcore: unknown op type %q", envelope.Type)
	}
}
