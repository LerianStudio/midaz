// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operationroute

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitMergePatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		raw              json.RawMessage
		expectMergeKeys  []string
		expectRemoveKeys []string
		expectMergeNil   bool
	}{
		{
			name:             "only_non_null_keys_go_to_merge",
			raw:              json.RawMessage(`{"direct":{"debit":{"code":"1001","description":"Cash"},"credit":{"code":"2001","description":"Revenue"}}}`),
			expectMergeKeys:  []string{"direct"},
			expectRemoveKeys: nil,
			expectMergeNil:   false,
		},
		{
			name:             "only_null_keys_go_to_remove",
			raw:              json.RawMessage(`{"hold":null,"cancel":null}`),
			expectMergeKeys:  nil,
			expectRemoveKeys: []string{"cancel", "hold"},
			expectMergeNil:   true,
		},
		{
			name:             "mix_of_merge_and_remove",
			raw:              json.RawMessage(`{"direct":{"debit":{"code":"1001"},"credit":{"code":"2001"}},"hold":null}`),
			expectMergeKeys:  []string{"direct"},
			expectRemoveKeys: []string{"hold"},
			expectMergeNil:   false,
		},
		{
			name:             "empty_object_returns_nil_for_both",
			raw:              json.RawMessage(`{}`),
			expectMergeKeys:  nil,
			expectRemoveKeys: nil,
			expectMergeNil:   true,
		},
		{
			name:             "multiple_non_null_entries",
			raw:              json.RawMessage(`{"direct":{"debit":{"code":"1001"}},"hold":{"debit":{"code":"1002"}},"commit":{"debit":{"code":"1003"}}}`),
			expectMergeKeys:  []string{"commit", "direct", "hold"},
			expectRemoveKeys: nil,
			expectMergeNil:   false,
		},
		{
			name:             "all_keys_null",
			raw:              json.RawMessage(`{"direct":null,"hold":null,"commit":null,"cancel":null,"revert":null}`),
			expectMergeKeys:  nil,
			expectRemoveKeys: []string{"cancel", "commit", "direct", "hold", "revert"},
			expectMergeNil:   true,
		},
		{
			name:             "update_and_remove_simultaneously",
			raw:              json.RawMessage(`{"direct":{"debit":{"code":"9999"},"credit":{"code":"8888"}},"hold":null,"commit":{"debit":{"code":"7777"}}}`),
			expectMergeKeys:  []string{"commit", "direct"},
			expectRemoveKeys: []string{"hold"},
			expectMergeNil:   false,
		},
		{
			name:             "invalid_json_returns_raw_as_merge",
			raw:              json.RawMessage(`{invalid}`),
			expectMergeKeys:  nil, // can't parse, returned as-is
			expectRemoveKeys: nil,
			expectMergeNil:   false, // raw is returned as mergeJSON
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mergeJSON, removeKeys := splitMergePatch(tt.raw)

			// Verify removeKeys
			sort.Strings(removeKeys)
			sort.Strings(tt.expectRemoveKeys)
			assert.Equal(t, tt.expectRemoveKeys, removeKeys, "removeKeys mismatch")

			// Verify mergeJSON
			if tt.expectMergeNil {
				assert.Nil(t, mergeJSON, "mergeJSON should be nil")
			} else {
				require.NotNil(t, mergeJSON, "mergeJSON should not be nil")

				if tt.name == "invalid_json_returns_raw_as_merge" {
					// For invalid JSON, the raw input is returned as-is
					assert.Equal(t, []byte(tt.raw), mergeJSON)
				} else {
					// Parse and verify keys
					var parsed map[string]json.RawMessage
					err := json.Unmarshal(mergeJSON, &parsed)
					require.NoError(t, err, "mergeJSON should be valid JSON")

					var actualKeys []string
					for k := range parsed {
						actualKeys = append(actualKeys, k)
					}
					sort.Strings(actualKeys)

					assert.Equal(t, tt.expectMergeKeys, actualKeys, "merge keys mismatch")

					// Verify no null values in merge output
					for k, v := range parsed {
						assert.NotEqual(t, "null", string(v), "key %q should not have null value in mergeJSON", k)
					}
				}
			}
		})
	}
}

func TestSplitMergePatch_PreservesNestedStructure(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"direct":{"debit":{"code":"1001","description":"Cash"},"credit":{"code":"2001","description":"Revenue"}}}`)

	mergeJSON, removeKeys := splitMergePatch(raw)

	assert.Nil(t, removeKeys)
	require.NotNil(t, mergeJSON)

	// Verify the nested structure is preserved exactly
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(mergeJSON, &parsed))

	directRaw, ok := parsed["direct"]
	require.True(t, ok, "direct key should exist")

	var entry map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(directRaw, &entry))

	_, hasDebit := entry["debit"]
	_, hasCredit := entry["credit"]
	assert.True(t, hasDebit, "debit should be preserved")
	assert.True(t, hasCredit, "credit should be preserved")
}
