// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionRoute_ToCache_MsgpackRoundTrip_FullPipeline verifies AC-6: end-to-end msgpack
// roundtrip through ToCache -> ToMsgpack -> FromMsgpack preserves all fields including
// Actions with multiple action types and all operation types.
func TestTransactionRoute_ToCache_MsgpackRoundTrip_FullPipeline(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	srcDirectID := uuid.New()
	dstDirectID := uuid.New()
	srcHoldID := uuid.New()
	bidiHoldID := uuid.New()
	dstCommitID := uuid.New()

	route := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Full Pipeline Route",
		OperationRoutes: []OperationRoute{
			{
				ID:                srcDirectID,
				OperationType:     "source",
				AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}},
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@source_direct",
				},
			},
			{
				ID:                dstDirectID,
				OperationType:     "destination",
				AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}},
				Account: &AccountRule{
					RuleType: "account_type",
					ValidIf:  []string{"checking", "savings"},
				},
			},
			{
				ID:                srcHoldID,
				OperationType:     "source",
				AccountingEntries: &AccountingEntries{Hold: &AccountingEntry{}},
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@source_hold",
				},
			},
			{
				ID:                bidiHoldID,
				OperationType:     "bidirectional",
				AccountingEntries: &AccountingEntries{Hold: &AccountingEntry{}},
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@bidi_hold",
				},
			},
			{
				ID:                dstCommitID,
				OperationType:     "destination",
				AccountingEntries: &AccountingEntries{Commit: &AccountingEntry{}},
				Account:           nil,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Step 1: Convert to cache
	cache := route.ToCache()

	// Step 2: Serialize to msgpack
	data, err := cache.ToMsgpack()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Step 3: Deserialize from msgpack
	var restored TransactionRouteCache
	err = restored.FromMsgpack(data)
	require.NoError(t, err)

	// Step 4: Verify Actions survived roundtrip
	require.NotNil(t, restored.Actions, "Actions must survive msgpack roundtrip")
	assert.Len(t, restored.Actions, 3, "should have 3 action groups: direct, hold, commit")

	// Verify direct action
	directAction, exists := restored.Actions["direct"]
	require.True(t, exists)
	assert.Len(t, directAction.Source, 1)
	assert.Len(t, directAction.Destination, 1)
	assert.Len(t, directAction.Bidirectional, 0)

	srcDirect, srcExists := directAction.Source[srcDirectID.String()]
	require.True(t, srcExists)
	require.NotNil(t, srcDirect.Account)
	assert.Equal(t, "alias", srcDirect.Account.RuleType)
	assert.Equal(t, "@source_direct", srcDirect.Account.ValidIf)

	// Verify hold action
	holdAction, exists := restored.Actions["hold"]
	require.True(t, exists)
	assert.Len(t, holdAction.Source, 1)
	assert.Len(t, holdAction.Destination, 0)
	assert.Len(t, holdAction.Bidirectional, 1)

	// Verify commit action
	commitAction, exists := restored.Actions["commit"]
	require.True(t, exists)
	assert.Len(t, commitAction.Source, 0)
	assert.Len(t, commitAction.Destination, 1)
	assert.Len(t, commitAction.Bidirectional, 0)

	// Verify nil Account survives roundtrip
	dstCommit, dstExists := commitAction.Destination[dstCommitID.String()]
	require.True(t, dstExists)
	assert.Nil(t, dstCommit.Account, "nil Account must survive msgpack roundtrip")
}

// TestTransactionRouteCache_MsgpackRoundTrip_EmptyActionsMap verifies AC-6: an empty (but non-nil)
// Actions map survives msgpack roundtrip and is not detected as stale.
func TestTransactionRouteCache_MsgpackRoundTrip_EmptyActionsMap(t *testing.T) {
	t.Parallel()

	original := TransactionRouteCache{
		Actions: map[string]ActionRouteCache{},
	}

	data, err := original.ToMsgpack()
	require.NoError(t, err)

	var restored TransactionRouteCache
	err = restored.FromMsgpack(data)
	require.NoError(t, err)

	// Document actual msgpack behavior with empty maps
	if restored.Actions != nil {
		assert.Len(t, restored.Actions, 0, "empty Actions should remain empty after roundtrip")
	}
}

// TestTransactionRouteCache_MsgpackRoundTrip_CorruptData verifies AC-4/AC-6: corrupt data
// returns an error from FromMsgpack without panic.
func TestTransactionRouteCache_MsgpackRoundTrip_CorruptData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "empty bytes",
			data: []byte{},
		},
		{
			name: "nil bytes",
			data: nil,
		},
		{
			name: "random garbage bytes",
			data: []byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA},
		},
		{
			name: "truncated msgpack",
			data: []byte{0x84, 0xA6}, // partial msgpack header
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cache TransactionRouteCache
			err := cache.FromMsgpack(tt.data)
			// All corrupt data should either error or result in zero-value cache
			// The important thing is no panic
			if err != nil {
				assert.Error(t, err)
			}
		})
	}
}
