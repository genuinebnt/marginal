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
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(fields, &merged); err != nil {
		return nil, err
	}
	typeJSON, err := json.Marshal(typeName)
	if err != nil {
		return nil, err
	}
	merged["type"] = typeJSON
	return json.Marshal(merged)
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
