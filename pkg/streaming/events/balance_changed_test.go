//go:build unit

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalanceChangedDefinition_Key(t *testing.T) {
	assert.Equal(t, "balance.changed", BalanceChangedDefinition.Key())
	assert.Equal(t, "balance", BalanceChangedDefinition.ResourceType)
	assert.Equal(t, "changed", BalanceChangedDefinition.EventType)
	assert.Equal(t, "1.0.0", BalanceChangedDefinition.SchemaVersion)
}

func TestNewBalanceChanged_MapsAllFields(t *testing.T) {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	src := BalanceChangedSource{
		OrganizationID: "org-1",
		LedgerID:       "led-1",
		AccountID:      "acc-1",
		BalanceID:      "bal-1",
		Alias:          "@person1",
		AssetCode:      "BRL",
		BalanceKey:     "default",
		Available:      decimal.NewFromInt(1500),
		OnHold:         decimal.NewFromInt(500),
		Version:        42,
		Reason:         BalanceChangeReasonCredit,
		OperationType:  "CREDIT",
		Direction:      "credit",
		Amount:         decimal.NewFromInt(100),
		TransactionID:  "txn-1",
		OperationID:    "op-1",
		OccurredAt:     ts,
	}

	p := NewBalanceChanged(src)

	assert.Equal(t, "acc-1", p.AccountID)
	assert.Equal(t, "bal-1", p.BalanceID)
	assert.Equal(t, "credit", p.Reason)
	assert.Equal(t, "CREDIT", p.OperationType)
	assert.True(t, decimal.NewFromInt(1500).Equal(p.Available))
	assert.Equal(t, int64(42), p.Version)
	assert.Equal(t, "2026-07-03T12:00:00Z", p.OccurredAt)
}

// Minimal-domain mapping: only the required identifiers are set. Optional
// fields (Alias, Direction) are left zero and must stay empty after mapping,
// and their omitempty JSON tags must drop them from the wire payload.
func TestNewBalanceChanged_MinimalDomain(t *testing.T) {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	src := BalanceChangedSource{
		OrganizationID: "org-1",
		LedgerID:       "led-1",
		AccountID:      "acc-1",
		BalanceID:      "bal-1",
		AssetCode:      "BRL",
		BalanceKey:     "default",
		Reason:         BalanceChangeReasonAdjust,
		OperationType:  "CREDIT",
		TransactionID:  "txn-1",
		OperationID:    "op-1",
		OccurredAt:     ts,
	}

	p := NewBalanceChanged(src)

	assert.Empty(t, p.Alias)
	assert.Empty(t, p.Direction)
	assert.True(t, p.Available.IsZero())
	assert.True(t, p.OnHold.IsZero())
	assert.True(t, p.Amount.IsZero())
	assert.Equal(t, int64(0), p.Version)

	raw, err := json.Marshal(p)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	assert.NotContains(t, m, "alias", "empty alias must be dropped by omitempty")
	assert.NotContains(t, m, "direction", "empty direction must be dropped by omitempty")
	// Required fields stay on the wire even when zero-valued.
	assert.Contains(t, m, "available")
	assert.Contains(t, m, "onHold")
	assert.Contains(t, m, "version")
}

func TestBalanceChangedPayload_ToEmitRequest(t *testing.T) {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	p := NewBalanceChanged(BalanceChangedSource{
		TransactionID: "txn-1",
		OperationID:   "op-1",
		Reason:        BalanceChangeReasonDebit,
		OccurredAt:    ts,
	})

	req, err := p.ToEmitRequest("tenant-1", ts)
	require.NoError(t, err)
	assert.Equal(t, "balance.changed", req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, "txn-1:op-1", req.Subject)
	assert.Equal(t, ts, req.Timestamp)
	assert.NotEmpty(t, req.Payload)
}

// JSONShape locks the wire field count and names (v1.0.0 contract).
func TestBalanceChangedPayload_JSONShape(t *testing.T) {
	p := NewBalanceChanged(BalanceChangedSource{
		OrganizationID: "org-1",
		LedgerID:       "led-1",
		AccountID:      "acc-1",
		BalanceID:      "bal-1",
		Alias:          "@person1",
		AssetCode:      "BRL",
		BalanceKey:     "default",
		Available:      decimal.NewFromInt(1500),
		OnHold:         decimal.NewFromInt(500),
		Version:        42,
		Reason:         BalanceChangeReasonCredit,
		OperationType:  "CREDIT",
		Direction:      "credit",
		Amount:         decimal.NewFromInt(100),
		TransactionID:  "txn-1",
		OperationID:    "op-1",
		OccurredAt:     time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC),
	})

	raw, err := json.Marshal(p)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	// 17 wire fields (alias and direction are omitempty but present here,
	// since every field is set in this test).
	for _, k := range []string{
		"organizationId", "ledgerId", "accountId", "balanceId", "alias",
		"assetCode", "balanceKey", "available", "onHold", "version",
		"reason", "operationType", "direction", "amount", "transactionId",
		"operationId", "occurredAt",
	} {
		_, ok := m[k]
		assert.Truef(t, ok, "missing wire field: %s", k)
	}
	assert.Len(t, m, 17, "payload must have exactly 17 wire fields")
	assert.NotContains(t, m, "scale", "scale is asset-level; it must not be in the payload")
}
