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
	"github.com/vmihailenco/msgpack/v5"
)

// legacyOperationRouteCache mirrors OperationRouteCache as it was serialized
// before links carried optionality, so the tests can prove what a pod that
// predates the flag writes and reads.
type legacyOperationRouteCache struct {
	Account           *AccountCache      `msgpack:"account"`
	OperationType     string             `msgpack:"operationType"`
	Code              string             `msgpack:"code"`
	Description       string             `msgpack:"description"`
	AccountingEntries *AccountingEntries `msgpack:"accountingEntries"`
}

type legacyActionRouteCache struct {
	Source        map[string]legacyOperationRouteCache `msgpack:"source"`
	Destination   map[string]legacyOperationRouteCache `msgpack:"destination"`
	Bidirectional map[string]legacyOperationRouteCache `msgpack:"bidirectional"`
}

type legacyTransactionRouteCache struct {
	Actions map[string]legacyActionRouteCache `msgpack:"actions"`
}

var optionalLinksFixedTime = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func TestTransactionRoute_ToCache_MarksOptionalLinksInEveryAction(t *testing.T) {
	t.Parallel()

	sourceID, destinationID, feeID := uuid.New(), uuid.New(), uuid.New()

	route := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		Title:          "Transfer with optional fee",
		OperationRoutes: []OperationRoute{
			{ID: sourceID, OperationType: "source", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}, Revert: &AccountingEntry{}}},
			{ID: destinationID, OperationType: "destination", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}, Commit: &AccountingEntry{}}},
			{ID: feeID, OperationType: "destination", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}, Commit: &AccountingEntry{}, Revert: &AccountingEntry{}}},
		},
		OptionalOperationRouteIDs: []uuid.UUID{feeID},
		CreatedAt:                 optionalLinksFixedTime,
		UpdatedAt:                 optionalLinksFixedTime,
	}

	assert.True(t, route.IsOptional(feeID))
	assert.False(t, route.IsOptional(sourceID))
	assert.False(t, route.IsOptional(destinationID))

	cache := route.ToCache()

	for _, action := range []string{"direct", "commit", "revert"} {
		fee, ok := cache.Actions[action].Destination[feeID.String()]
		require.Truef(t, ok, "fee route must be cached under %s", action)
		assert.Truef(t, fee.Optional, "fee route must be optional under %s", action)
	}

	assert.False(t, cache.Actions["direct"].Source[sourceID.String()].Optional, "source route stays required")
	assert.False(t, cache.Actions["direct"].Destination[destinationID.String()].Optional, "destination route stays required")
	assert.False(t, cache.Actions["commit"].Destination[destinationID.String()].Optional, "destination route stays required under commit")
}

// A route with no optional links must serialize exactly as before the flag
// existed, so its cache entry is byte-identical whichever pod writes it.
func TestTransactionRouteCache_WithoutOptionalLinks_SerializesLikeLegacyShape(t *testing.T) {
	t.Parallel()

	sourceID, destinationID := uuid.New(), uuid.New()

	route := &TransactionRoute{
		ID: uuid.New(), OrganizationID: uuid.New(), Title: "Required only",
		OperationRoutes: []OperationRoute{
			{ID: sourceID, OperationType: "source", Code: "SRC", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}}},
			{ID: destinationID, OperationType: "destination", Code: "DST", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}}},
		},
	}

	current, err := route.ToCache().ToMsgpack()
	require.NoError(t, err)

	legacy, err := msgpack.Marshal(legacyTransactionRouteCache{Actions: map[string]legacyActionRouteCache{
		"direct": {
			Source:        map[string]legacyOperationRouteCache{sourceID.String(): {OperationType: "source", Code: "SRC", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}}}},
			Destination:   map[string]legacyOperationRouteCache{destinationID.String(): {OperationType: "destination", Code: "DST", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}}}},
			Bidirectional: map[string]legacyOperationRouteCache{},
		},
	}})
	require.NoError(t, err)

	assert.Equal(t, legacy, current)
}

// Cache entries never expire, so entries written before the flag existed are
// read by every new pod. A missing flag must mean required: the strict reading
// is the zero value, never the lenient one.
func TestTransactionRouteCache_FromMsgpack_LegacyEntryReadsEveryRouteAsRequired(t *testing.T) {
	t.Parallel()

	sourceID, destinationID := uuid.New().String(), uuid.New().String()

	legacy, err := msgpack.Marshal(legacyTransactionRouteCache{Actions: map[string]legacyActionRouteCache{
		"direct": {
			Source:      map[string]legacyOperationRouteCache{sourceID: {OperationType: "source"}},
			Destination: map[string]legacyOperationRouteCache{destinationID: {OperationType: "destination"}},
		},
	}})
	require.NoError(t, err)

	var cache TransactionRouteCache
	require.NoError(t, cache.FromMsgpack(legacy))

	assert.False(t, cache.Actions["direct"].Source[sourceID].Optional)
	assert.False(t, cache.Actions["direct"].Destination[destinationID].Optional)
}

// A pod that predates the flag must still read an entry written by a newer
// pod; it ignores the flag and treats the route as required.
func TestTransactionRouteCache_OptionalEntryDecodesIntoLegacyShape(t *testing.T) {
	t.Parallel()

	sourceID, feeID := uuid.New(), uuid.New()

	route := &TransactionRoute{
		ID: uuid.New(), OrganizationID: uuid.New(), Title: "With optional fee",
		OperationRoutes: []OperationRoute{
			{ID: sourceID, OperationType: "source", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}}},
			{ID: feeID, OperationType: "destination", Code: "FEE", AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}}},
		},
		OptionalOperationRouteIDs: []uuid.UUID{feeID},
	}

	current, err := route.ToCache().ToMsgpack()
	require.NoError(t, err)

	var legacy legacyTransactionRouteCache
	require.NoError(t, msgpack.Unmarshal(current, &legacy))

	fee, ok := legacy.Actions["direct"].Destination[feeID.String()]
	require.True(t, ok)
	assert.Equal(t, "FEE", fee.Code)
}
