// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToCache_PropagatesCodeAndAccountingEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		code              string
		accountingEntries *AccountingEntries
		wantCode          string
		wantEntries       *AccountingEntries
	}{
		{
			name: "full accounting entries and code propagated",
			code: "EXT-001",
			accountingEntries: &AccountingEntries{
				Direct: &AccountingEntry{
					Debit:  &AccountingRubric{Code: "1001", Description: "Cash"},
					Credit: &AccountingRubric{Code: "2001", Description: "Revenue"},
				},
				Hold: &AccountingEntry{
					Debit:  &AccountingRubric{Code: "1002", Description: "Held Funds"},
					Credit: &AccountingRubric{Code: "2002", Description: "Liability"},
				},
			},
			wantCode: "EXT-001",
			wantEntries: &AccountingEntries{
				Direct: &AccountingEntry{
					Debit:  &AccountingRubric{Code: "1001", Description: "Cash"},
					Credit: &AccountingRubric{Code: "2001", Description: "Revenue"},
				},
				Hold: &AccountingEntry{
					Debit:  &AccountingRubric{Code: "1002", Description: "Held Funds"},
					Credit: &AccountingRubric{Code: "2002", Description: "Liability"},
				},
			},
		},
		{
			name: "minimal accounting entries with code still propagated",
			code: "EXT-002",
			accountingEntries: &AccountingEntries{
				Direct: &AccountingEntry{},
			},
			wantCode: "EXT-002",
			wantEntries: &AccountingEntries{
				Direct: &AccountingEntry{},
			},
		},
		{
			name: "empty code with minimal entries",
			code: "",
			accountingEntries: &AccountingEntries{
				Direct: &AccountingEntry{},
			},
			wantCode: "",
			wantEntries: &AccountingEntries{
				Direct: &AccountingEntry{},
			},
		},
		{
			name: "partial accounting entries — only direct set",
			code: "EXT-003",
			accountingEntries: &AccountingEntries{
				Direct: &AccountingEntry{
					Debit:  &AccountingRubric{Code: "3001", Description: "Accounts Receivable"},
					Credit: &AccountingRubric{Code: "3002", Description: "Sales"},
				},
			},
			wantCode: "EXT-003",
			wantEntries: &AccountingEntries{
				Direct: &AccountingEntry{
					Debit:  &AccountingRubric{Code: "3001", Description: "Accounts Receivable"},
					Credit: &AccountingRubric{Code: "3002", Description: "Sales"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			routeID := uuid.New()
			tr := TransactionRoute{
				OperationRoutes: []OperationRoute{
					{
						ID:                routeID,
						Code:              tt.code,
						OperationType:     "source",
						Action:            "direct",
						AccountingEntries: tt.accountingEntries,
					},
				},
			}

			cache := tr.ToCache()

			actionCache, exists := cache.Actions["direct"]
			require.True(t, exists, "expected action 'direct' to exist in cache")

			routeCache, exists := actionCache.Source[routeID.String()]
			require.True(t, exists, "expected route to exist in source map")

			assert.Equal(t, tt.wantCode, routeCache.Code)
			assert.Equal(t, tt.wantEntries, routeCache.AccountingEntries)
		})
	}
}

func TestToCache_MsgpackRoundTrip_PreservesRubrics(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()
	tr := TransactionRoute{
		OperationRoutes: []OperationRoute{
			{
				ID:            routeID,
				Code:          "EXT-RT",
				OperationType: "source",
				Action:        "direct",
				AccountingEntries: &AccountingEntries{
					Direct: &AccountingEntry{
						Debit:  &AccountingRubric{Code: "1001", Description: "Cash"},
						Credit: &AccountingRubric{Code: "2001", Description: "Revenue"},
					},
				},
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash",
				},
			},
		},
	}

	original := tr.ToCache()

	// Serialize to msgpack
	data, err := original.ToMsgpack()
	require.NoError(t, err, "ToMsgpack should not fail")

	// Deserialize from msgpack
	var restored TransactionRouteCache
	err = restored.FromMsgpack(data)
	require.NoError(t, err, "FromMsgpack should not fail")

	// Verify round-trip preserves all fields
	routeCache := restored.Actions["direct"].Source[routeID.String()]
	assert.Equal(t, "source", routeCache.OperationType)
	assert.Equal(t, "EXT-RT", routeCache.Code)
	require.NotNil(t, routeCache.AccountingEntries)
	require.NotNil(t, routeCache.AccountingEntries.Direct)
	assert.Equal(t, "1001", routeCache.AccountingEntries.Direct.Debit.Code)
	assert.Equal(t, "Cash", routeCache.AccountingEntries.Direct.Debit.Description)
	assert.Equal(t, "2001", routeCache.AccountingEntries.Direct.Credit.Code)
	assert.Equal(t, "Revenue", routeCache.AccountingEntries.Direct.Credit.Description)
	assert.Nil(t, routeCache.AccountingEntries.Hold)
	assert.Nil(t, routeCache.AccountingEntries.Cancel)
}

func TestToCache_MsgpackRoundTrip_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	// Simulate legacy data: a cache serialized WITHOUT Code and AccountingEntries fields.
	// This uses the old struct shape (only Account + OperationType).
	routeID := uuid.New()
	legacyCache := TransactionRouteCache{
		Actions: map[string]ActionRouteCache{
			"direct": {
				Source: map[string]OperationRouteCache{
					routeID.String(): {
						OperationType: "source",
						Account: &AccountCache{
							RuleType: "alias",
							ValidIf:  "@legacy",
						},
					},
				},
				Destination:   make(map[string]OperationRouteCache),
				Bidirectional: make(map[string]OperationRouteCache),
			},
		},
	}

	// Serialize legacy data
	data, err := legacyCache.ToMsgpack()
	require.NoError(t, err)

	// Deserialize into current struct (which now has Code and AccountingEntries)
	var restored TransactionRouteCache
	err = restored.FromMsgpack(data)
	require.NoError(t, err, "deserializing legacy data should not fail")

	routeCache := restored.Actions["direct"].Source[routeID.String()]
	assert.Equal(t, "source", routeCache.OperationType)
	assert.Equal(t, "", routeCache.Code, "missing Code field should default to empty string")
	assert.Nil(t, routeCache.AccountingEntries, "missing AccountingEntries should default to nil")
	require.NotNil(t, routeCache.Account)
	assert.Equal(t, "alias", routeCache.Account.RuleType)
}

func TestToCache_MsgpackRoundTrip_MinimalAccountingEntries(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()
	tr := TransactionRoute{
		OperationRoutes: []OperationRoute{
			{
				ID:                routeID,
				Code:              "MIN-ENT",
				OperationType:     "destination",
				Action:            "hold",
				AccountingEntries: &AccountingEntries{Hold: &AccountingEntry{}},
			},
		},
	}

	original := tr.ToCache()

	data, err := original.ToMsgpack()
	require.NoError(t, err)

	var restored TransactionRouteCache
	err = restored.FromMsgpack(data)
	require.NoError(t, err)

	routeCache := restored.Actions["hold"].Destination[routeID.String()]
	assert.Equal(t, "MIN-ENT", routeCache.Code)
	require.NotNil(t, routeCache.AccountingEntries)
	assert.NotNil(t, routeCache.AccountingEntries.Hold)
}
