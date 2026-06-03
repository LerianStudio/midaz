// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataSourceInfo_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info DataSourceInfo
	}{
		{
			name: "all fields populated",
			info: DataSourceInfo{
				ID:     "midaz_onboarding",
				Name:   "onboarding",
				Type:   "postgresql",
				Status: "available",
			},
		},
		{
			name: "minimal fields",
			info: DataSourceInfo{
				ID:   "test_db",
				Type: "mongodb",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.info)
			require.NoError(t, err, "marshalling DataSourceInfo should not fail")

			var decoded DataSourceInfo
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err, "unmarshalling DataSourceInfo should not fail")

			assert.Equal(t, tt.info, decoded, "round-trip should preserve all fields")
		})
	}
}

func TestDataSourceInfo_JSONFieldNames(t *testing.T) {
	t.Parallel()

	info := DataSourceInfo{
		ID:     "midaz_onboarding",
		Name:   "onboarding",
		Type:   "postgresql",
		Status: "available",
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	assert.Contains(t, raw, "id", "JSON should use 'id' field name")
	assert.Contains(t, raw, "name", "JSON should use 'name' field name")
	assert.Contains(t, raw, "type", "JSON should use 'type' field name")
	assert.Contains(t, raw, "status", "JSON should use 'status' field name")
}

func TestDataSourceSchema_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema DataSourceSchema
	}{
		{
			name: "schema with tables and fields",
			schema: DataSourceSchema{
				DataSourceID: "midaz_onboarding",
				Tables: []SchemaTable{
					{
						Name:   "account",
						Schema: "public",
						Fields: []SchemaField{
							{Name: "id", Type: "uuid"},
							{Name: "name", Type: "varchar"},
						},
					},
				},
			},
		},
		{
			name: "schema with empty tables",
			schema: DataSourceSchema{
				DataSourceID: "empty_db",
				Tables:       []SchemaTable{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.schema)
			require.NoError(t, err, "marshalling DataSourceSchema should not fail")

			var decoded DataSourceSchema
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err, "unmarshalling DataSourceSchema should not fail")

			assert.Equal(t, tt.schema, decoded, "round-trip should preserve all fields")
		})
	}
}

func TestValidationResult_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result ValidationResult
	}{
		{
			name: "valid result with no warnings",
			result: ValidationResult{
				Valid:    true,
				Warnings: []ValidationWarning{},
			},
		},
		{
			name: "invalid result with warnings",
			result: ValidationResult{
				Valid: false,
				Warnings: []ValidationWarning{
					{
						Field:   "nonexistent_field",
						Code:    "DATA_SOURCE_UNAVAILABLE",
						Message: "Field not found in data source schema",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.result)
			require.NoError(t, err, "marshalling ValidationResult should not fail")

			var decoded ValidationResult
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err, "unmarshalling ValidationResult should not fail")

			assert.Equal(t, tt.result, decoded, "round-trip should preserve all fields")
		})
	}
}
