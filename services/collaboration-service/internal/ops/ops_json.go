package ops

import (
	"encoding/json"
	"fmt"
)

// MarshalOp and UnmarshalOp are Op's wire encoding — the same "type"-tagged
// envelope convention documentcore's MarshalOp/UnmarshalOp uses, since Op is
// an interface and encoding/json can't dispatch on it alone. This is the
// format internal/oplog and internal/wal frame into collab.ops.payload
// (DATA_MODEL.md) and the local WAL record body.

// TypeName is op's variant name — the same string MarshalOp tags its JSON
// envelope with, and what collab.ops.kind (DATA_MODEL.md) stores, so there
// is exactly one name for a given op variant rather than two schemes that
// could drift apart.
func TypeName(op Op) (string, error) {
	switch op.(type) {
	case InsertText:
		return "InsertText", nil
	case DeleteText:
		return "DeleteText", nil
	case NoOp:
		return "NoOp", nil
	default:
		return "", fmt.Errorf("ops: unknown op type %T", op)
	}
}

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
// already-marshaled JSON object without decoding it — see
// documentcore.MarshalOp's identical helper (same fix, same reason: this
// package's own equivalent op-marshaling path used to round-trip through
// a map[string]json.RawMessage just to add one "type" field). Not shared
// across the module boundary between documentcore and this package for
// ~15 lines of code; duplicated deliberately rather than forcing an
// import neither package's own domain actually needs.
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

func UnmarshalOp(data []byte) (Op, error) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}

	switch envelope.Type {
	case "InsertText":
		var op InsertText
		err := json.Unmarshal(data, &op)
		return op, err
	case "DeleteText":
		var op DeleteText
		err := json.Unmarshal(data, &op)
		return op, err
	case "NoOp":
		var op NoOp
		err := json.Unmarshal(data, &op)
		return op, err
	default:
		return nil, fmt.Errorf("ops: unknown op type %q", envelope.Type)
	}
}
