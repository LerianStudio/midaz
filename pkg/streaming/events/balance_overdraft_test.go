// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	overdraftTxID = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab20").String()
	overdraftOpID = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab21").String()
)

func minimalOverdraftSource() events.BalanceOverdraftSource {
	return events.BalanceOverdraftSource{
		BalanceID:        balanceID,
		AccountID:        balanceAccount,
		OrganizationID:   balanceOrg,
		LedgerID:         balanceLed,
		AssetCode:        "USD",
		TransactionID:    overdraftTxID,
		OperationID:      overdraftOpID,
		Amount:           decimal.NewFromInt(150),
		OverdraftBalance: decimal.NewFromInt(50),
		OccurredAt:       fixedTime,
	}
}

func TestBalanceOverdraftDefinitions_Keys(t *testing.T) {
	// Hyphen-spelled event types satisfy the lib-streaming route-key regex.
	assert.Equal(t, "balance.overdraft-drawn", events.BalanceOverdraftDrawnDefinition.Key())
	assert.Equal(t, "balance.overdraft-repaid", events.BalanceOverdraftRepaidDefinition.Key())
	assert.Equal(t, "balance.overdraft-cleared", events.BalanceOverdraftClearedDefinition.Key())

	for _, def := range []events.Definition{
		events.BalanceOverdraftDrawnDefinition,
		events.BalanceOverdraftRepaidDefinition,
		events.BalanceOverdraftClearedDefinition,
	} {
		assert.Equal(t, "balance", def.ResourceType)
		assert.Equal(t, "1.0.0", def.SchemaVersion)
	}
}

func TestBalanceOverdraft_WireActionConstants(t *testing.T) {
	assert.Equal(t, "drawn", events.OverdraftWireActionDrawn)
	assert.Equal(t, "repaid", events.OverdraftWireActionRepaid)
	assert.Equal(t, "cleared", events.OverdraftWireActionCleared)
}

func TestNewBalanceOverdraft_ConstructorsStampMatchingAction(t *testing.T) {
	src := minimalOverdraftSource()

	drawn := events.NewBalanceOverdraftDrawn(src)
	repaid := events.NewBalanceOverdraftRepaid(src)
	cleared := events.NewBalanceOverdraftCleared(src)

	assert.Equal(t, "drawn", drawn.Action)
	assert.Equal(t, "repaid", repaid.Action)
	assert.Equal(t, "cleared", cleared.Action)

	// All three constructors must produce the same non-action fields
	// (the only difference between them is the discriminator).
	for _, p := range []events.BalanceOverdraftPayload{drawn, repaid, cleared} {
		assert.Equal(t, src.BalanceID, p.BalanceID)
		assert.Equal(t, src.AccountID, p.AccountID)
		assert.Equal(t, src.OrganizationID, p.OrganizationID)
		assert.Equal(t, src.LedgerID, p.LedgerID)
		assert.Equal(t, src.AssetCode, p.AssetCode)
		assert.Equal(t, src.TransactionID, p.TransactionID)
		assert.Equal(t, src.OperationID, p.OperationID)
		assert.True(t, src.Amount.Equal(p.Amount))
		assert.True(t, src.OverdraftBalance.Equal(p.OverdraftBalance))
		assert.Nil(t, p.OverdraftLimit, "OverdraftLimit must remain nil until T-010 lands")
		assert.Equal(t, "2026-05-13T12:34:56Z", p.OccurredAt)
	}
}

func TestNewBalanceOverdraft_AcceptsExplicitOverdraftLimit(t *testing.T) {
	src := minimalOverdraftSource()
	limit := decimal.NewFromInt(500)
	src.OverdraftLimit = &limit

	payload := events.NewBalanceOverdraftDrawn(src)

	require.NotNil(t, payload.OverdraftLimit)
	assert.True(t, payload.OverdraftLimit.Equal(limit))
}

func TestBalanceOverdraftPayload_ToEmitRequest_AssemblesStreamingEvents(t *testing.T) {
	tests := []struct {
		name      string
		payload   events.BalanceOverdraftPayload
		emit      func(events.BalanceOverdraftPayload) (libStreaming.EmitRequest, error)
		expectKey string
	}{
		{
			name:    "drawn",
			payload: events.NewBalanceOverdraftDrawn(minimalOverdraftSource()),
			emit: func(p events.BalanceOverdraftPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestDrawn("tenant-x", fixedTime)
			},
			expectKey: "balance.overdraft-drawn",
		},
		{
			name:    "repaid",
			payload: events.NewBalanceOverdraftRepaid(minimalOverdraftSource()),
			emit: func(p events.BalanceOverdraftPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestRepaid("tenant-x", fixedTime)
			},
			expectKey: "balance.overdraft-repaid",
		},
		{
			name:    "cleared",
			payload: events.NewBalanceOverdraftCleared(minimalOverdraftSource()),
			emit: func(p events.BalanceOverdraftPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestCleared("tenant-x", fixedTime)
			},
			expectKey: "balance.overdraft-cleared",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.emit(tc.payload)
			require.NoError(t, err)

			assert.Equal(t, tc.expectKey, req.DefinitionKey)
			assert.Equal(t, "tenant-x", req.TenantID)
			assert.Equal(t, overdraftTxID+":"+overdraftOpID, req.Subject,
				"Subject must combine transactionId+operationId for idempotency-friendly dedup")
			assert.Equal(t, fixedTime, req.Timestamp)

			var roundTrip events.BalanceOverdraftPayload
			require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
			assert.Equal(t, tc.payload.Action, roundTrip.Action)
		})
	}
}

func TestBalanceOverdraftPayload_JSONShape_IncludesAllRequiredFields(t *testing.T) {
	payload := events.NewBalanceOverdraftDrawn(minimalOverdraftSource())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"balanceId", "accountId", "organizationId", "ledgerId", "assetCode",
		"transactionId", "operationId", "action", "amount",
		"overdraftBalance", "occurredAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasLimit := generic["overdraftLimit"]
	assert.False(t, hasLimit, "overdraftLimit must omitempty until T-010 lands")

	_, hasScale := generic["scale"]
	assert.False(t, hasScale, "scale is intentionally omitted from the wire payload")
}
