// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBalanceRedis_HasOverdraftFields verifies via reflection that the
// Redis cache projection exposes the overdraft fields with the right
// types and json tags that the Lua scripts depend on.
func TestBalanceRedis_HasOverdraftFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fieldName    string
		expectedType reflect.Type
		expectedJSON string
	}{
		{
			name:         "direction field",
			fieldName:    "Direction",
			expectedType: reflect.TypeOf(""),
			expectedJSON: "direction",
		},
		{
			name:         "overdraft_used field",
			fieldName:    "OverdraftUsed",
			expectedType: reflect.TypeOf(""),
			expectedJSON: "overdraftUsed",
		},
		{
			name:         "allow_overdraft field",
			fieldName:    "AllowOverdraft",
			expectedType: reflect.TypeOf(0),
			expectedJSON: "allowOverdraft",
		},
		{
			name:         "overdraft_limit_enabled field",
			fieldName:    "OverdraftLimitEnabled",
			expectedType: reflect.TypeOf(0),
			expectedJSON: "overdraftLimitEnabled",
		},
		{
			name:         "overdraft_limit field",
			fieldName:    "OverdraftLimit",
			expectedType: reflect.TypeOf(""),
			expectedJSON: "overdraftLimit",
		},
		{
			name:         "balance_scope field",
			fieldName:    "BalanceScope",
			expectedType: reflect.TypeOf(""),
			expectedJSON: "balanceScope",
		},
	}

	redisType := reflect.TypeOf(BalanceRedis{})

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, ok := redisType.FieldByName(tt.fieldName)
			require.True(t, ok, "expected field %q on BalanceRedis", tt.fieldName)
			assert.Equal(t, tt.expectedType, field.Type, "field %q type mismatch", tt.fieldName)

			// JSON tag may have options (e.g. "name,omitempty"); take the name.
			jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
			assert.Equal(t, tt.expectedJSON, jsonTag, "field %q json tag mismatch", tt.fieldName)
		})
	}
}

// TestBalanceRedis_UnmarshalJSON_OverdraftFields verifies that a fully
// populated JSON payload — matching what the Lua scripts write back — is
// correctly deserialized, including the overdraft fields.
func TestBalanceRedis_UnmarshalJSON_OverdraftFields(t *testing.T) {
	t.Parallel()

	payload := `{
		"id": "00000000-0000-0000-0000-000000000001",
		"alias": "@person1",
		"key": "default",
		"accountId": "00000000-0000-0000-0000-000000000002",
		"assetCode": "USD",
		"available": 1500,
		"onHold": 500,
		"version": 3,
		"accountType": "deposit",
		"allowSending": 1,
		"allowReceiving": 1,
		"direction": "debit",
		"overdraftUsed": "50.00",
		"allowOverdraft": 1,
		"overdraftLimitEnabled": 1,
		"overdraftLimit": "500.00",
		"balanceScope": "transactional"
	}`

	var br BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(payload), &br))

	assert.Equal(t, "debit", br.Direction)
	assert.Equal(t, "50.00", br.OverdraftUsed)
	assert.Equal(t, 1, br.AllowOverdraft)
	assert.Equal(t, 1, br.OverdraftLimitEnabled)
	assert.Equal(t, "500.00", br.OverdraftLimit)
	assert.Equal(t, "transactional", br.BalanceScope)
}

// TestBalanceRedis_UnmarshalJSON_OverdraftUsedFromFloat verifies that
// numeric OverdraftUsed values (common when Lua emits a plain number) are
// normalized into a decimal string, preserving the integer/decimal value.
func TestBalanceRedis_UnmarshalJSON_OverdraftUsedFromFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "decimal value", input: "50.5", expected: "50.5"},
		{name: "zero value", input: "0", expected: "0"},
		{name: "integer value", input: "100", expected: "100"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload := `{
				"available": 0,
				"onHold": 0,
				"overdraftUsed": ` + tt.input + `
			}`

			var br BalanceRedis
			require.NoError(t, json.Unmarshal([]byte(payload), &br))

			assert.Equal(t, tt.expected, br.OverdraftUsed)
		})
	}
}

// TestBalanceRedis_UnmarshalJSON_MissingOverdraftFields verifies legacy
// payloads (written before the overdraft feature) still decode cleanly,
// with sensible defaults applied to the overdraft fields.
func TestBalanceRedis_UnmarshalJSON_MissingOverdraftFields(t *testing.T) {
	t.Parallel()

	payload := `{
		"id": "00000000-0000-0000-0000-000000000001",
		"alias": "@legacy",
		"accountId": "00000000-0000-0000-0000-000000000002",
		"assetCode": "USD",
		"available": 100,
		"onHold": 0,
		"version": 1,
		"accountType": "deposit",
		"allowSending": 1,
		"allowReceiving": 1
	}`

	var br BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(payload), &br))

	assert.Equal(t, "", br.Direction, "legacy payload must leave Direction empty")
	assert.Equal(t, "0", br.OverdraftUsed, "missing overdraftUsed must default to \"0\"")
	assert.Equal(t, 0, br.AllowOverdraft)
	assert.Equal(t, 0, br.OverdraftLimitEnabled)
	assert.Equal(t, "0", br.OverdraftLimit, "missing overdraftLimit must default to \"0\"")
	assert.Equal(t, "", br.BalanceScope)
}
