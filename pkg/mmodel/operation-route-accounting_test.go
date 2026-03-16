// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountingEntries_JSONMarshal(t *testing.T) {
	t.Parallel()

	directEntry := &AccountingEntry{
		Debit:  &AccountingRubric{Code: "1001", Description: "Cash"},
		Credit: &AccountingRubric{Code: "2001", Description: "Revenue"},
	}
	tests := []struct {
		name    string
		route   OperationRoute
		wantKey bool
	}{
		{
			name:    "nil accounting entries omitted from JSON",
			route:   OperationRoute{ID: uuid.New(), OperationType: "source"},
			wantKey: false,
		},
		{
			name:    "partial accounting entries with only direct action",
			route:   OperationRoute{ID: uuid.New(), OperationType: "source", AccountingEntries: &AccountingEntries{Direct: directEntry}},
			wantKey: true,
		},
		{
			name: "full accounting entries with all actions",
			route: OperationRoute{ID: uuid.New(), OperationType: "source", AccountingEntries: &AccountingEntries{
				Direct: directEntry,
				Hold:   &AccountingEntry{Debit: &AccountingRubric{Code: "1002", Description: "Held Cash"}, Credit: &AccountingRubric{Code: "2002", Description: "Held Revenue"}},
				Commit: &AccountingEntry{Debit: &AccountingRubric{Code: "1003", Description: "Committed"}, Credit: &AccountingRubric{Code: "2003", Description: "Committed Revenue"}},
				Cancel: &AccountingEntry{Debit: &AccountingRubric{Code: "1004", Description: "Cancelled"}, Credit: &AccountingRubric{Code: "2004", Description: "Cancelled Revenue"}},
				Revert: &AccountingEntry{Debit: &AccountingRubric{Code: "1005", Description: "Reverted"}, Credit: &AccountingRubric{Code: "2005", Description: "Reverted Revenue"}},
			}},
			wantKey: true,
		},
		{
			name:    "partial entry with only debit",
			route:   OperationRoute{ID: uuid.New(), OperationType: "source", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{Debit: &AccountingRubric{Code: "1001", Description: "Cash"}}}},
			wantKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.route)
			require.NoError(t, err)

			var raw map[string]any
			err = json.Unmarshal(data, &raw)
			require.NoError(t, err)

			_, hasKey := raw["accountingEntries"]
			assert.Equal(t, tt.wantKey, hasKey, "accountingEntries key presence mismatch")
		})
	}

	t.Run("full accounting entries values are correctly marshaled", func(t *testing.T) {
		t.Parallel()

		route := OperationRoute{
			ID:            uuid.New(),
			OperationType: "source",
			AccountingEntries: &AccountingEntries{
				Direct: &AccountingEntry{
					Debit:  &AccountingRubric{Code: "1001", Description: "Cash"},
					Credit: &AccountingRubric{Code: "2001", Description: "Revenue"},
				},
				Revert: &AccountingEntry{
					Debit:  &AccountingRubric{Code: "1005", Description: "Reverted"},
					Credit: &AccountingRubric{Code: "2005", Description: "Reverted Revenue"},
				},
			},
		}

		data, err := json.Marshal(route)
		require.NoError(t, err)

		var unmarshaled OperationRoute
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		require.NotNil(t, unmarshaled.AccountingEntries)
		require.NotNil(t, unmarshaled.AccountingEntries.Direct)
		assert.Equal(t, "1001", unmarshaled.AccountingEntries.Direct.Debit.Code)
		assert.Equal(t, "Cash", unmarshaled.AccountingEntries.Direct.Debit.Description)
		assert.Equal(t, "2001", unmarshaled.AccountingEntries.Direct.Credit.Code)
		assert.Equal(t, "Revenue", unmarshaled.AccountingEntries.Direct.Credit.Description)
		require.NotNil(t, unmarshaled.AccountingEntries.Revert)
		assert.Equal(t, "2005", unmarshaled.AccountingEntries.Revert.Credit.Code)
		assert.Equal(t, "Reverted Revenue", unmarshaled.AccountingEntries.Revert.Credit.Description)
	})
}

func TestAccountingEntries_JSONUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		jsonInput     string
		wantNil       bool
		wantDirect    bool
		wantHold      bool
		wantDebitCode string
	}{
		{
			name:      "unmarshal with no accounting entries field",
			jsonInput: `{"operationType":"source"}`,
			wantNil:   true,
		},
		{
			name:      "unmarshal with null accounting entries",
			jsonInput: `{"operationType":"source","accountingEntries":null}`,
			wantNil:   true,
		},
		{
			name: "unmarshal partial accounting entries",
			jsonInput: `{
				"operationType":"source",
				"accountingEntries":{
					"direct":{
						"debit":{"code":"1001","description":"Cash"}
					}
				}
			}`,
			wantNil:       false,
			wantDirect:    true,
			wantHold:      false,
			wantDebitCode: "1001",
		},
		{
			name: "unmarshal full accounting entries",
			jsonInput: `{
				"operationType":"source",
				"accountingEntries":{
					"direct":{"debit":{"code":"1001","description":"Cash"},"credit":{"code":"2001","description":"Revenue"}},
					"hold":{"debit":{"code":"1002","description":"Held"},"credit":{"code":"2002","description":"Held Revenue"}}
				}
			}`,
			wantNil:       false,
			wantDirect:    true,
			wantHold:      true,
			wantDebitCode: "1001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var route OperationRoute
			err := json.Unmarshal([]byte(tt.jsonInput), &route)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, route.AccountingEntries)
				return
			}

			require.NotNil(t, route.AccountingEntries)

			if tt.wantDirect {
				require.NotNil(t, route.AccountingEntries.Direct)
				require.NotNil(t, route.AccountingEntries.Direct.Debit)
				assert.Equal(t, tt.wantDebitCode, route.AccountingEntries.Direct.Debit.Code)
			}

			if tt.wantDirect && route.AccountingEntries.Direct.Credit != nil {
				assert.Equal(t, "2001", route.AccountingEntries.Direct.Credit.Code)
				assert.Equal(t, "Revenue", route.AccountingEntries.Direct.Credit.Description)
				assert.Equal(t, "Cash", route.AccountingEntries.Direct.Debit.Description)
			}

			if tt.wantHold {
				require.NotNil(t, route.AccountingEntries.Hold)
				if route.AccountingEntries.Hold.Credit != nil {
					assert.Equal(t, "2002", route.AccountingEntries.Hold.Credit.Code)
					assert.Equal(t, "Held Revenue", route.AccountingEntries.Hold.Credit.Description)
				}
			} else {
				assert.Nil(t, route.AccountingEntries.Hold)
			}
		})
	}
}

func TestAccountingEntries_InputTypes_JSONMarshal(t *testing.T) {
	t.Parallel()

	entries := &AccountingEntries{
		Direct: &AccountingEntry{
			Debit: &AccountingRubric{Code: "1001", Description: "Cash"},
		},
	}

	t.Run("create input nil omitted", func(t *testing.T) {
		t.Parallel()
		data, err := json.Marshal(CreateOperationRouteInput{Title: "T", OperationType: "source"})
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		_, hasKey := raw["accountingEntries"]
		assert.False(t, hasKey)
	})
	t.Run("create input with entries", func(t *testing.T) {
		t.Parallel()
		data, err := json.Marshal(CreateOperationRouteInput{Title: "T", OperationType: "source", AccountingEntries: entries})
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		_, hasKey := raw["accountingEntries"]
		assert.True(t, hasKey)
	})
	t.Run("update input nil omitted", func(t *testing.T) {
		t.Parallel()
		data, err := json.Marshal(UpdateOperationRouteInput{Title: "T"})
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		_, hasKey := raw["accountingEntries"]
		assert.False(t, hasKey)
	})
	t.Run("update input with entries", func(t *testing.T) {
		t.Parallel()
		data, err := json.Marshal(UpdateOperationRouteInput{Title: "T", AccountingEntries: entries})
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		_, hasKey := raw["accountingEntries"]
		assert.True(t, hasKey)
	})
}

func TestAccountingEntries_RoundTrip(t *testing.T) {
	t.Parallel()

	original := OperationRoute{
		ID:            uuid.New(),
		OperationType: "source",
		AccountingEntries: &AccountingEntries{
			Direct: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1001", Description: "Cash"},
				Credit: &AccountingRubric{Code: "2001", Description: "Revenue"},
			},
			Hold: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1002", Description: "Held Cash"},
				Credit: &AccountingRubric{Code: "2002", Description: "Held Revenue"},
			},
			Revert: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1005", Description: "Reverted"},
				Credit: &AccountingRubric{Code: "2005", Description: "Reverted Revenue"},
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var roundTripped OperationRoute
	require.NoError(t, json.Unmarshal(data, &roundTripped))

	ae := roundTripped.AccountingEntries
	require.NotNil(t, ae)

	assert.Equal(t, "1001", ae.Direct.Debit.Code)
	assert.Equal(t, "Cash", ae.Direct.Debit.Description)
	assert.Equal(t, "2001", ae.Direct.Credit.Code)
	assert.Equal(t, "Revenue", ae.Direct.Credit.Description)
	assert.Equal(t, "1002", ae.Hold.Debit.Code)
	assert.Equal(t, "2002", ae.Hold.Credit.Code)
	assert.Equal(t, "Held Revenue", ae.Hold.Credit.Description)
	assert.Equal(t, "1005", ae.Revert.Debit.Code)
	assert.Equal(t, "2005", ae.Revert.Credit.Code)
	assert.Equal(t, "Reverted Revenue", ae.Revert.Credit.Description)
	assert.Nil(t, ae.Commit)
	assert.Nil(t, ae.Cancel)
}
