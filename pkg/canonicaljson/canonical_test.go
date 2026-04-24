// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package canonicaljson

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeTwoFormsIntoMap parses both JSON forms into map[string]any. Using the native
// decoder proves that any key-ordering or whitespace differences in the raw JSON do NOT
// survive the decode — which is the precondition for our canonicalization guarantee.
func decodeTwoFormsIntoMap(t *testing.T, a, b string) (map[string]any, map[string]any) {
	t.Helper()

	var mapA, mapB map[string]any

	require.NoError(t, json.Unmarshal([]byte(a), &mapA), "form A must decode")
	require.NoError(t, json.Unmarshal([]byte(b), &mapB), "form B must decode")

	return mapA, mapB
}

func TestCanonicalize_SortsObjectKeys(t *testing.T) {
	// Same logical object, keys shuffled.
	a := `{"b": 1, "a": 2, "c": 3}`
	b := `{"a": 2, "b": 1, "c": 3}`
	c := `{"c": 3, "a": 2, "b": 1}`

	mapA, mapB := decodeTwoFormsIntoMap(t, a, b)
	_, mapC := decodeTwoFormsIntoMap(t, a, c)

	canA, err := Canonicalize(mapA)
	require.NoError(t, err)

	canB, err := Canonicalize(mapB)
	require.NoError(t, err)

	canC, err := Canonicalize(mapC)
	require.NoError(t, err)

	assert.Equal(t, string(canA), string(canB), "shuffled keys must produce identical canonical form")
	assert.Equal(t, string(canA), string(canC), "shuffled keys (third shuffle) must produce identical canonical form")
	assert.Equal(t, `{"a":2,"b":1,"c":3}`, string(canA), "canonical form must have sorted keys and no whitespace")
}

func TestCanonicalize_SortsNestedObjectKeys(t *testing.T) {
	a := `{"outer": {"z": 1, "a": 2}, "inner": {"y": 3, "b": 4}}`
	b := `{"inner": {"b": 4, "y": 3}, "outer": {"a": 2, "z": 1}}`

	mapA, mapB := decodeTwoFormsIntoMap(t, a, b)

	canA, err := Canonicalize(mapA)
	require.NoError(t, err)

	canB, err := Canonicalize(mapB)
	require.NoError(t, err)

	assert.Equal(t, string(canA), string(canB), "nested shuffled keys must canonicalize identically")
	assert.Equal(t, `{"inner":{"b":4,"y":3},"outer":{"a":2,"z":1}}`, string(canA))
}

func TestCanonicalize_StripsInsignificantWhitespace(t *testing.T) {
	cases := []string{
		`{"a":1,"b":2}`,
		`{ "a" : 1 , "b" : 2 }`,
		"{\n  \"a\": 1,\n  \"b\": 2\n}",
		"{\t\"a\":\t1,\t\"b\":\t2\t}",
	}

	var first []byte

	for i, raw := range cases {
		var m map[string]any

		require.NoError(t, json.Unmarshal([]byte(raw), &m), "case %d must decode", i)

		can, err := Canonicalize(m)
		require.NoError(t, err, "case %d must canonicalize", i)

		if first == nil {
			first = can
			continue
		}

		assert.Equal(t, string(first), string(can), "case %d must produce identical canonical form", i)
	}
}

func TestCanonicalize_NilAndEmptyPolicy(t *testing.T) {
	// Top-level nil encodes to JSON null.
	canNil, err := Canonicalize(nil)
	require.NoError(t, err)
	assert.Equal(t, "null", string(canNil))

	// Nil map encodes to null (matches encoding/json).
	var nilMap map[string]any

	canNilMap, err := Canonicalize(nilMap)
	require.NoError(t, err)
	assert.Equal(t, "null", string(canNilMap), "nil map encodes to null")

	// Empty (non-nil) map encodes to {}.
	canEmptyMap, err := Canonicalize(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "{}", string(canEmptyMap), "empty map encodes to {}")

	// Nil slice encodes to null.
	var nilSlice []any

	canNilSlice, err := Canonicalize(nilSlice)
	require.NoError(t, err)
	assert.Equal(t, "null", string(canNilSlice), "nil slice encodes to null")

	// Empty (non-nil) slice encodes to [].
	canEmptySlice, err := Canonicalize([]any{})
	require.NoError(t, err)
	assert.Equal(t, "[]", string(canEmptySlice), "empty slice encodes to []")

	// null vs {} vs [] must all hash differently.
	hashNull, err := Hash(nil)
	require.NoError(t, err)

	hashEmptyObj, err := Hash(map[string]any{})
	require.NoError(t, err)

	hashEmptyArr, err := Hash([]any{})
	require.NoError(t, err)

	assert.NotEqual(t, hashNull, hashEmptyObj, "null and {} must hash differently")
	assert.NotEqual(t, hashNull, hashEmptyArr, "null and [] must hash differently")
	assert.NotEqual(t, hashEmptyObj, hashEmptyArr, "{} and [] must hash differently")
}

func TestCanonicalize_ArrayOrderIsPreserved(t *testing.T) {
	// Array order is semantic — it must NOT be canonicalized away.
	a := []any{float64(3), float64(1), float64(2)}
	b := []any{float64(1), float64(2), float64(3)}

	canA, err := Canonicalize(a)
	require.NoError(t, err)

	canB, err := Canonicalize(b)
	require.NoError(t, err)

	assert.NotEqual(t, string(canA), string(canB), "different array orders must produce different canonical forms")
	assert.Equal(t, "[3,1,2]", string(canA))
	assert.Equal(t, "[1,2,3]", string(canB))
}

func TestCanonicalize_PrimitiveValues(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  string
	}{
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"integer float", float64(42), "42"},
		{"negative integer float", float64(-7), "-7"},
		{"fractional float", float64(1.5), "1.5"},
		{"string simple", "hello", `"hello"`},
		{"string with quote", `hello"world`, `"hello\"world"`},
		{"string with backslash", `a\b`, `"a\\b"`},
		{"string with unicode", "café", `"café"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Canonicalize(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestHash_DeterministicForSameLogicalValue(t *testing.T) {
	a := `{"holderId":"abc","ledgerId":"xyz","name":"Alice"}`
	b := `{"name":"Alice","ledgerId":"xyz","holderId":"abc"}`
	c := "{\n\t\"name\":\t\"Alice\",\n\t\"holderId\":\t\"abc\",\n\t\"ledgerId\":\t\"xyz\"\n}"

	var mapA, mapB, mapC map[string]any

	require.NoError(t, json.Unmarshal([]byte(a), &mapA))
	require.NoError(t, json.Unmarshal([]byte(b), &mapB))
	require.NoError(t, json.Unmarshal([]byte(c), &mapC))

	hashA, err := Hash(mapA)
	require.NoError(t, err)

	hashB, err := Hash(mapB)
	require.NoError(t, err)

	hashC, err := Hash(mapC)
	require.NoError(t, err)

	assert.Equal(t, hashA, hashB, "key-shuffled payloads must hash identically")
	assert.Equal(t, hashA, hashC, "whitespace-formatted payload must hash identically")
	assert.Len(t, hashA, 64, "SHA-256 hex must be 64 chars")
}

func TestHash_DiffersOnBusinessChange(t *testing.T) {
	base := map[string]any{
		"holderId":  "holder-1",
		"ledgerId":  "ledger-1",
		"name":      "Alice",
		"assetCode": "USD",
	}

	hashBase, err := Hash(base)
	require.NoError(t, err)

	// Change a single field value — hash must differ.
	renamed := map[string]any{
		"holderId":  "holder-1",
		"ledgerId":  "ledger-1",
		"name":      "Bob",
		"assetCode": "USD",
	}

	hashRenamed, err := Hash(renamed)
	require.NoError(t, err)

	assert.NotEqual(t, hashBase, hashRenamed, "different name must produce different hash")

	// Change the holder ID — hash must differ.
	differentHolder := map[string]any{
		"holderId":  "holder-2",
		"ledgerId":  "ledger-1",
		"name":      "Alice",
		"assetCode": "USD",
	}

	hashHolder, err := Hash(differentHolder)
	require.NoError(t, err)

	assert.NotEqual(t, hashBase, hashHolder, "different holderId must produce different hash")

	// Remove a field — hash must differ.
	missingAsset := map[string]any{
		"holderId": "holder-1",
		"ledgerId": "ledger-1",
		"name":     "Alice",
	}

	hashMissing, err := Hash(missingAsset)
	require.NoError(t, err)

	assert.NotEqual(t, hashBase, hashMissing, "missing field must produce different hash")
}

func TestHash_StructRoundTripsThroughJSON(t *testing.T) {
	// Test that a real struct (like a CreateAccount payload) hashes stably regardless of
	// the Go map iteration order for its metadata sub-map.
	type payload struct {
		HolderID string         `json:"holderId"`
		LedgerID string         `json:"ledgerId"`
		Metadata map[string]any `json:"metadata"`
	}

	p := payload{
		HolderID: "h-1",
		LedgerID: "l-1",
		Metadata: map[string]any{
			"tier":     "gold",
			"priority": float64(1),
			"tags":     []any{"a", "b"},
		},
	}

	// Hash the same struct 20 times; it must be identical every time, since map
	// iteration order is randomized by Go.
	first, err := Hash(p)
	require.NoError(t, err)

	for i := 0; i < 20; i++ {
		again, err := Hash(p)
		require.NoError(t, err)
		assert.Equal(t, first, again, "iteration %d must hash identically", i)
	}
}

func TestCanonicalize_ReturnsErrorOnUnmarshalable(t *testing.T) {
	// Functions are not json-marshalable; we expect a wrapped error.
	_, err := Canonicalize(func() {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canonicaljson")
}
