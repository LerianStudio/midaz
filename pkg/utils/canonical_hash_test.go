// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// samplePayload mirrors the shape used by Phase 3/4 — a typed request body with an
// arbitrary metadata bag whose key ordering is not under our control.
type samplePayload struct {
	Alias    string         `json:"alias"`
	Type     string         `json:"type"`
	AssetID  string         `json:"assetId"`
	Nickname *string        `json:"nickname,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func TestCanonicalHashJSON_MapKeyOrderInvariant(t *testing.T) {
	t.Parallel()

	// Two map[string]any values constructed in a way that can legitimately serialize with
	// different key ordering. We force the issue by marshaling each separately and confirming
	// that after canonicalization the hashes match.
	a := map[string]any{
		"alias":   "account-1",
		"type":    "deposit",
		"assetId": "USD",
	}
	b := map[string]any{
		"assetId": "USD",
		"type":    "deposit",
		"alias":   "account-1",
	}

	hashA, err := CanonicalHashJSON(a)
	require.NoError(t, err)

	hashB, err := CanonicalHashJSON(b)
	require.NoError(t, err)

	assert.Equal(t, hashA, hashB, "maps with identical content must produce identical hashes")
}

func TestCanonicalHashJSON_TypedStructFieldOrderInvariant(t *testing.T) {
	t.Parallel()

	// Structs with the same values should hash identically regardless of literal field order
	// in the source (the Go compiler already handles this — this is a sanity check).
	a := samplePayload{
		Alias:   "account-1",
		Type:    "deposit",
		AssetID: "USD",
	}
	b := samplePayload{
		AssetID: "USD",
		Alias:   "account-1",
		Type:    "deposit",
	}

	hashA, err := CanonicalHashJSON(a)
	require.NoError(t, err)

	hashB, err := CanonicalHashJSON(b)
	require.NoError(t, err)

	assert.Equal(t, hashA, hashB)
}

func TestCanonicalHashJSON_NilVsEmptyStringPointer(t *testing.T) {
	t.Parallel()

	// *string(nil) serializes as omitted (omitempty) or null; a pointer to ""
	// serializes as "". These must produce different hashes so Phase 3/4 can distinguish
	// "field not provided" from "field provided as empty".
	type payload struct {
		Nickname *string `json:"nickname"`
	}

	empty := ""
	nilPtr := payload{Nickname: nil}
	emptyPtr := payload{Nickname: &empty}

	hashNil, err := CanonicalHashJSON(nilPtr)
	require.NoError(t, err)

	hashEmpty, err := CanonicalHashJSON(emptyPtr)
	require.NoError(t, err)

	assert.NotEqual(t, hashNil, hashEmpty, "nil pointer and pointer-to-empty-string must hash differently")
}

func TestCanonicalHashJSON_DifferentBusinessValues(t *testing.T) {
	t.Parallel()

	a := samplePayload{Alias: "account-1", Type: "deposit", AssetID: "USD"}
	b := samplePayload{Alias: "account-2", Type: "deposit", AssetID: "USD"}

	hashA, err := CanonicalHashJSON(a)
	require.NoError(t, err)

	hashB, err := CanonicalHashJSON(b)
	require.NoError(t, err)

	assert.NotEqual(t, hashA, hashB)
}

func TestCanonicalHashJSON_NestedMapOrderInvariant(t *testing.T) {
	t.Parallel()

	a := samplePayload{
		Alias:   "account-1",
		Type:    "deposit",
		AssetID: "USD",
		Metadata: map[string]any{
			"region":   "us-east-1",
			"priority": "high",
			"tags": map[string]any{
				"env":    "prod",
				"team":   "platform",
				"origin": "crm",
			},
		},
	}
	b := samplePayload{
		Alias:   "account-1",
		Type:    "deposit",
		AssetID: "USD",
		Metadata: map[string]any{
			"tags": map[string]any{
				"origin": "crm",
				"team":   "platform",
				"env":    "prod",
			},
			"priority": "high",
			"region":   "us-east-1",
		},
	}

	hashA, err := CanonicalHashJSON(a)
	require.NoError(t, err)

	hashB, err := CanonicalHashJSON(b)
	require.NoError(t, err)

	assert.Equal(t, hashA, hashB, "nested maps with identical content must produce identical hashes")
}

func TestCanonicalHashJSON_SourceJSONKeyOrderInvariant(t *testing.T) {
	t.Parallel()

	// Simulate two wire-format payloads that arrive with different key orderings.
	// Decoding into map[string]any and re-canonicalizing must collide on the same hash.
	rawA := []byte(`{"alias":"account-1","type":"deposit","assetId":"USD","metadata":{"region":"us-east-1","priority":"high"}}`)
	rawB := []byte(`{"metadata":{"priority":"high","region":"us-east-1"},"assetId":"USD","type":"deposit","alias":"account-1"}`)

	var a, b map[string]any

	require.NoError(t, json.Unmarshal(rawA, &a))
	require.NoError(t, json.Unmarshal(rawB, &b))

	hashA, err := CanonicalHashJSON(a)
	require.NoError(t, err)

	hashB, err := CanonicalHashJSON(b)
	require.NoError(t, err)

	assert.Equal(t, hashA, hashB)
}

func TestCanonicalHashJSON_HashShape(t *testing.T) {
	t.Parallel()

	// SHA-256 hex is 64 lowercase hex chars. Cheap sanity check against accidental format drift.
	hash, err := CanonicalHashJSON(samplePayload{Alias: "a", Type: "t", AssetID: "x"})
	require.NoError(t, err)

	assert.Len(t, hash, 64)

	for _, r := range hash {
		assert.True(t,
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'),
			"hash must be lowercase hex, got %q", hash,
		)
	}
}
