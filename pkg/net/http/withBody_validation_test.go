package http

import (
	"encoding/json"
	"testing"
)

type metaPayload struct {
	Metadata map[string]any `json:"metadata"`
}

func TestValidateNoNullBytes_AllowsCleanStrings(t *testing.T) {
	p := &metaPayload{Metadata: map[string]any{"a": "abc", "n": 123}}
	if v := validateNoNullBytes(p); v != nil {
		b, _ := json.Marshal(v)
		t.Fatalf("expected no violations, got %s", string(b))
	}
}

func TestValidateNoNullBytes_DetectsNullByte(t *testing.T) {
	p := &metaPayload{Metadata: map[string]any{"a": "bad\x00value"}}
	v := validateNoNullBytes(p)
	if len(v) == 0 {
		t.Fatalf("expected violation for null byte, got none")
	}
}
