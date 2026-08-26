//go:build js && wasm

package main

import (
	"encoding/json"

	"github.com/google/uuid"
	"marginal/document-service/internal/documentcore"
)

func marshal(v any) ([]byte, error) { return json.Marshal(v) }

func unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func parsePageID(s string) (documentcore.PageID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return documentcore.PageID{}, err
	}
	return documentcore.PageID(id), nil
}
