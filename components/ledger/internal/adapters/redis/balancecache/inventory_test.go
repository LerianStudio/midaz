// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyShape(t *testing.T) {
	dual, err := Encode(codecSnapshot(), FormatDual)
	require.NoError(t, err)
	newOnly, err := Encode(codecSnapshot(), FormatNewOnly)
	require.NoError(t, err)

	var legacyFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(dual, &legacyFields))
	delete(legacyFields, "SchemaVersion")
	for _, name := range fieldNames {
		delete(legacyFields, lowerName(name))
	}
	legacy := marshalCodecFields(t, legacyFields)

	var mixedFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(dual, &mixedFields))
	delete(mixedFields, "allowReceiving")
	mixed := marshalCodecFields(t, mixedFields)

	var conflictingFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(dual, &conflictingFields))
	conflictingFields["available"] = json.RawMessage(`"999"`)
	conflictingFields["version"] = json.RawMessage(`"8"`)
	conflictingFields["allowSending"] = json.RawMessage(`false`)
	conflicting := marshalCodecFields(t, conflictingFields)

	tests := []struct {
		name  string
		raw   []byte
		shape Shape
		err   bool
	}{
		{name: "legacy", raw: legacy, shape: ShapeLegacy},
		{name: "dual", raw: dual, shape: ShapeDual},
		{name: "new only", raw: newOnly, shape: ShapeNewOnly},
		{name: "mixed", raw: mixed, shape: ShapeMixed},
		{name: "conflicting dual", raw: conflicting, shape: ShapeInvalid, err: true},
		{name: "invalid JSON", raw: []byte(`{"id":`), shape: ShapeInvalid, err: true},
		{name: "invalid value", raw: []byte(`{"SchemaVersion":2,"id":"bad"}`), shape: ShapeInvalid, err: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shape, err := ClassifyShape(tt.raw)
			require.Equal(t, tt.shape, shape)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBuildInventoryIsDeterministicAndReadOnly(t *testing.T) {
	dual, err := Encode(codecSnapshot(), FormatDual)
	require.NoError(t, err)
	newOnly, err := Encode(codecSnapshot(), FormatNewOnly)
	require.NoError(t, err)
	before := append([]byte(nil), dual...)

	report, err := BuildInventory([]InventoryEntry{
		{Key: "balance:z", Value: []byte(`not-json`)},
		{Key: "balance:b", Value: newOnly},
		{Key: "balance:a", Value: dual},
	})
	require.NoError(t, err)
	require.Equal(t, InventoryReport{
		Total: 3, Dual: 1, NewOnly: 1, Invalid: 1,
		Issues: []InventoryIssue{{Key: "balance:z", Shape: ShapeInvalid, Error: "cached balance must be a JSON object"}},
	}, report)
	require.False(t, report.FormatReadyForNewOnly())
	require.Equal(t, before, dual)
}

func TestInventoryFormatReadiness(t *testing.T) {
	dual, err := Encode(codecSnapshot(), FormatDual)
	require.NoError(t, err)
	newOnly, err := Encode(codecSnapshot(), FormatNewOnly)
	require.NoError(t, err)

	report, err := BuildInventory([]InventoryEntry{{Key: "balance:a", Value: dual}, {Key: "balance:b", Value: newOnly}})
	require.NoError(t, err)
	require.True(t, report.FormatReadyForNewOnly())

	empty, err := BuildInventory(nil)
	require.NoError(t, err)
	require.False(t, empty.FormatReadyForNewOnly())
}

func TestBuildInventoryRejectsAmbiguousKeys(t *testing.T) {
	_, err := BuildInventory([]InventoryEntry{{Key: "balance:a"}, {Key: "balance:a"}})
	require.ErrorContains(t, err, "duplicate balance cache inventory key")

	_, err = BuildInventory([]InventoryEntry{{Key: ""}})
	require.ErrorContains(t, err, "empty key")
}
