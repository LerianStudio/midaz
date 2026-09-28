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
	"github.com/vmihailenco/msgpack/v5"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func crossLedgerTestEntry() *AccountingEntry {
	return &AccountingEntry{
		Debit:  &AccountingRubric{Code: "1900", Description: "Arriving from another ledger"},
		Credit: &AccountingRubric{Code: "2900", Description: "Leaving to another ledger"},
	}
}

func TestAccountingEntries_CrossLedgerIsAnEntryOnlyAction(t *testing.T) {
	t.Parallel()

	entries := &AccountingEntries{CrossLedger: crossLedgerTestEntry()}

	assert.Equal(t, []string{constant.ActionCrossLedger}, entries.Actions())
	assert.NotContains(t, constant.ValidActions, constant.ActionCrossLedger,
		"crossLedger is an accounting-entry key, never a transaction-route action")
}

func TestAccountingEntries_CrossLedgerJSONKeyIsOptional(t *testing.T) {
	t.Parallel()

	without, err := json.Marshal(AccountingEntries{Direct: &AccountingEntry{}})
	require.NoError(t, err)
	assert.NotContains(t, string(without), "crossLedger")

	with, err := json.Marshal(AccountingEntries{CrossLedger: crossLedgerTestEntry()})
	require.NoError(t, err)

	var decoded AccountingEntries
	require.NoError(t, json.Unmarshal(with, &decoded))
	require.NotNil(t, decoded.CrossLedger)
	assert.Equal(t, "1900", decoded.CrossLedger.Debit.Code)
	assert.Equal(t, "2900", decoded.CrossLedger.Credit.Code)
}

func TestTransactionRoute_ToCache_BridgeRouteAppearsOnlyUnderCrossLedger(t *testing.T) {
	t.Parallel()

	sourceID := uuid.New()
	destinationID := uuid.New()
	bridgeID := uuid.New()

	route := &TransactionRoute{
		ID: uuid.New(),
		OperationRoutes: []OperationRoute{
			{ID: sourceID, OperationType: constant.OperationRouteTypeSource, AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}}},
			{ID: destinationID, OperationType: constant.OperationRouteTypeDestination, AccountingEntries: &AccountingEntries{Direct: &AccountingEntry{}}},
			{ID: bridgeID, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &AccountingEntries{CrossLedger: crossLedgerTestEntry()}},
		},
	}

	cache := route.ToCache()

	bridgeCache, ok := cache.Actions[constant.ActionCrossLedger]
	require.True(t, ok, "the bridge route must be cached under the crossLedger action")
	assert.Contains(t, bridgeCache.Bidirectional, bridgeID.String())
	assert.Len(t, bridgeCache.Bidirectional, 1)
	assert.Empty(t, bridgeCache.Source)
	assert.Empty(t, bridgeCache.Destination)

	direct := cache.Actions[constant.ActionDirect]
	_, inDirect := direct.FindRoute(bridgeID.String())
	assert.False(t, inDirect, "a pure bridge route must never count in the direct template")
	assert.Len(t, direct.Source, 1)
	assert.Len(t, direct.Destination, 1)
	assert.Empty(t, direct.Bidirectional)
}

func TestTransactionRouteCache_CrossLedgerEntrySurvivesMsgpackRoundTrip(t *testing.T) {
	t.Parallel()

	bridgeID := uuid.New()
	route := &TransactionRoute{
		ID: uuid.New(),
		OperationRoutes: []OperationRoute{
			{ID: bridgeID, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &AccountingEntries{CrossLedger: crossLedgerTestEntry()}},
		},
	}

	raw, err := route.ToCache().ToMsgpack()
	require.NoError(t, err)

	var decoded TransactionRouteCache
	require.NoError(t, decoded.FromMsgpack(raw))

	cached, ok := decoded.Actions[constant.ActionCrossLedger].FindRoute(bridgeID.String())
	require.True(t, ok)
	require.NotNil(t, cached.AccountingEntries)
	require.NotNil(t, cached.AccountingEntries.CrossLedger)
	assert.Equal(t, "1900", cached.AccountingEntries.CrossLedger.Debit.Code)
	assert.Equal(t, "2900", cached.AccountingEntries.CrossLedger.Credit.Code)
}

// The previous release's cache shapes, without the crossLedger entry. They stand
// in for a pod that has not been upgraded yet during a rolling deploy.
type previousAccountingEntries struct {
	Direct    *AccountingEntry `msgpack:"direct"`
	Hold      *AccountingEntry `msgpack:"hold"`
	Commit    *AccountingEntry `msgpack:"commit"`
	Cancel    *AccountingEntry `msgpack:"cancel"`
	Revert    *AccountingEntry `msgpack:"revert"`
	Overdraft *AccountingEntry `msgpack:"overdraft"`
	Block     *AccountingEntry `msgpack:"block"`
	Unblock   *AccountingEntry `msgpack:"unblock"`
}

type previousOperationRouteCache struct {
	Account           *AccountCache              `msgpack:"account"`
	OperationType     string                     `msgpack:"operationType"`
	Code              string                     `msgpack:"code"`
	Description       string                     `msgpack:"description"`
	AccountingEntries *previousAccountingEntries `msgpack:"accountingEntries"`
}

type previousActionRouteCache struct {
	Source        map[string]previousOperationRouteCache `msgpack:"source"`
	Destination   map[string]previousOperationRouteCache `msgpack:"destination"`
	Bidirectional map[string]previousOperationRouteCache `msgpack:"bidirectional"`
}

type previousTransactionRouteCache struct {
	Actions map[string]previousActionRouteCache `msgpack:"actions"`
}

func TestTransactionRouteCache_CrossLedgerEntryIsCompatibleAcrossVersions(t *testing.T) {
	t.Parallel()

	sourceID := uuid.New().String()
	bridgeID := uuid.New().String()
	direct := &AccountingEntry{Debit: &AccountingRubric{Code: "1000", Description: "Direct"}}

	t.Run("a previous-release decoder reads a cache that carries crossLedger", func(t *testing.T) {
		t.Parallel()

		current := TransactionRouteCache{Actions: map[string]ActionRouteCache{
			constant.ActionDirect: {Source: map[string]OperationRouteCache{
				sourceID: {OperationType: constant.OperationRouteTypeSource, AccountingEntries: &AccountingEntries{Direct: direct}},
			}},
			constant.ActionCrossLedger: {Bidirectional: map[string]OperationRouteCache{
				bridgeID: {OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &AccountingEntries{CrossLedger: crossLedgerTestEntry()}},
			}},
		}}

		raw, err := current.ToMsgpack()
		require.NoError(t, err)

		var previous previousTransactionRouteCache
		require.NoError(t, msgpack.Unmarshal(raw, &previous))
		assert.Equal(t, "1000", previous.Actions[constant.ActionDirect].Source[sourceID].AccountingEntries.Direct.Debit.Code)
	})

	t.Run("the current decoder reads a cache written by the previous release", func(t *testing.T) {
		t.Parallel()

		previous := previousTransactionRouteCache{Actions: map[string]previousActionRouteCache{
			constant.ActionDirect: {Source: map[string]previousOperationRouteCache{
				sourceID: {OperationType: constant.OperationRouteTypeSource, AccountingEntries: &previousAccountingEntries{Direct: direct}},
			}},
		}}

		raw, err := msgpack.Marshal(previous)
		require.NoError(t, err)

		var current TransactionRouteCache
		require.NoError(t, current.FromMsgpack(raw))

		cached, ok := current.Actions[constant.ActionDirect].FindRoute(sourceID)
		require.True(t, ok)
		assert.Equal(t, "1000", cached.AccountingEntries.Direct.Debit.Code)
		assert.Nil(t, cached.AccountingEntries.CrossLedger)
		_, hasBridge := current.Actions[constant.ActionCrossLedger]
		assert.False(t, hasBridge)
	})
}
