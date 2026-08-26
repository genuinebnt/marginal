package documentcore

import (
	"encoding/json"
	"fmt"
)

// MarshalOp and UnmarshalOp are Op's wire encoding: each variant's own JSON
// tags, plus a "type" field naming the variant — since Op is an interface,
// encoding/json can't dispatch on it by itself the way it can for
// BlockKind/MarkKind's tagged structs. This is the format the WASM/JS
// boundary uses, and the shape DATA_MODEL.md's collab.ops.kind column
// (stored as text) plus its op payload already assumes.
func MarshalOp(op Op) ([]byte, error) {
	var typeName string
	switch op.(type) {
	case InsertBlock:
		typeName = "InsertBlock"
	case DeleteBlock:
		typeName = "DeleteBlock"
	case SetBlockKind:
		typeName = "SetBlockKind"
	case SetBlockContent:
		typeName = "SetBlockContent"
	case SetTitle:
		typeName = "SetTitle"
	case MoveBlock:
		typeName = "MoveBlock"
	default:
		return nil, fmt.Errorf("documentcore: unknown op type %T", op)
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
