// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

var (
	groupEventID       = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ad10").String()
	groupEventReverted = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ad11").String()
	groupEventOrg      = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ad12").String()
	groupEventLedgerA  = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ad13").String()
	groupEventLedgerB  = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ad14").String()
	groupEventOrigin   = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ad15").String()
	groupEventDest     = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ad16").String()
)

func minimalTransactionGroupSource() events.TransactionGroupSource {
	return events.TransactionGroupSource{
		GroupID:   groupEventID,
		Status:    constant.APPROVED,
		AssetCode: "BRL",
		Parts: []events.TransactionGroupPartSource{
			{
				TransactionID:  groupEventOrigin,
				OrganizationID: groupEventOrg,
				LedgerID:       groupEventLedgerA,
				Role:           events.TransactionGroupRoleOrigin,
				Status:         constant.APPROVED,
			},
			{
				TransactionID:  groupEventDest,
				OrganizationID: groupEventOrg,
				LedgerID:       groupEventLedgerB,
				Role:           events.TransactionGroupRoleDestination,
				Status:         constant.APPROVED,
			},
		},
		OccurredAt: fixedTime,
	}
}

func TestTransactionGroupLifecycleDefinitions_Keys(t *testing.T) {
	assert.Equal(t, "transaction_group.posted", events.TransactionGroupPostedDefinition.Key())
	assert.Equal(t, "transaction_group.committed", events.TransactionGroupCommittedDefinition.Key())
	assert.Equal(t, "transaction_group.canceled", events.TransactionGroupCanceledDefinition.Key())
	assert.Equal(t, "transaction_group.reverted", events.TransactionGroupRevertedDefinition.Key())

	for _, def := range []events.Definition{
		events.TransactionGroupPostedDefinition,
		events.TransactionGroupCommittedDefinition,
		events.TransactionGroupCanceledDefinition,
		events.TransactionGroupRevertedDefinition,
	} {
		assert.Equal(t, "transaction_group", def.ResourceType)
		assert.Equal(t, "1.0.0", def.SchemaVersion)
	}

	assert.Equal(t, "origin", events.TransactionGroupRoleOrigin)
	assert.Equal(t, "destination", events.TransactionGroupRoleDestination)
}

func TestNewTransactionGroup_MapsMinimalSource(t *testing.T) {
	payload := events.NewTransactionGroup(minimalTransactionGroupSource())

	assert.Equal(t, groupEventID, payload.GroupID)
	assert.Nil(t, payload.RevertedGroupID)
	assert.Equal(t, constant.APPROVED, payload.Status)
	assert.Equal(t, "BRL", payload.AssetCode)
	assert.Equal(t, 2, payload.LedgerCount)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.OccurredAt)
	require.Len(t, payload.Parts, 2)
	assert.Equal(t, events.TransactionGroupPart{
		TransactionID:  groupEventOrigin,
		OrganizationID: groupEventOrg,
		LedgerID:       groupEventLedgerA,
		Role:           events.TransactionGroupRoleOrigin,
		Status:         constant.APPROVED,
	}, payload.Parts[0])
	assert.Equal(t, events.TransactionGroupRoleDestination, payload.Parts[1].Role)
}

func TestNewTransactionGroup_MapsRevertedGroupAndCountsDistinctLedgers(t *testing.T) {
	src := minimalTransactionGroupSource()
	src.RevertedGroupID = &groupEventReverted
	src.Parts = append(src.Parts, events.TransactionGroupPartSource{
		TransactionID:  uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ad17").String(),
		OrganizationID: groupEventOrg,
		LedgerID:       groupEventLedgerA,
		Role:           events.TransactionGroupRoleOrigin,
		Status:         constant.APPROVED,
	})

	payload := events.NewTransactionGroup(src)

	require.NotNil(t, payload.RevertedGroupID)
	assert.Equal(t, groupEventReverted, *payload.RevertedGroupID)
	assert.Len(t, payload.Parts, 3)
	assert.Equal(t, 2, payload.LedgerCount, "ledgerCount counts distinct ledgers, not parts")
}

func TestTransactionGroupPayload_ToEmitRequest_AssemblesStreamingEvents(t *testing.T) {
	tests := []struct {
		name      string
		emit      func(events.TransactionGroupPayload) (libStreaming.EmitRequest, error)
		expectKey string
	}{
		{
			name: "posted",
			emit: func(p events.TransactionGroupPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestPosted("tenant-x", fixedTime)
			},
			expectKey: "transaction_group.posted",
		},
		{
			name: "committed",
			emit: func(p events.TransactionGroupPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestCommitted("tenant-x", fixedTime)
			},
			expectKey: "transaction_group.committed",
		},
		{
			name: "canceled",
			emit: func(p events.TransactionGroupPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestCanceled("tenant-x", fixedTime)
			},
			expectKey: "transaction_group.canceled",
		},
		{
			name: "reverted",
			emit: func(p events.TransactionGroupPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestReverted("tenant-x", fixedTime)
			},
			expectKey: "transaction_group.reverted",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := events.NewTransactionGroup(minimalTransactionGroupSource())

			req, err := tc.emit(payload)
			require.NoError(t, err)

			assert.Equal(t, tc.expectKey, req.DefinitionKey)
			assert.Equal(t, "tenant-x", req.TenantID)
			assert.Equal(t, groupEventID, req.Subject, "Subject must be the group id")
			assert.Equal(t, fixedTime, req.Timestamp)

			var roundTrip events.TransactionGroupPayload
			require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
			assert.Equal(t, payload, roundTrip)
		})
	}
}

func TestTransactionGroupPayload_JSONShape(t *testing.T) {
	payload := events.NewTransactionGroup(minimalTransactionGroupSource())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := []string{"groupId", "status", "assetCode", "ledgerCount", "parts", "occurredAt"}
	for _, key := range expectedKeys {
		assert.Containsf(t, generic, key, "wire payload must include %q", key)
	}

	assert.Lenf(t, generic, len(expectedKeys), "expected %d top-level fields, got %d (drift?)", len(expectedKeys), len(generic))
	assert.NotContains(t, generic, "revertedGroupId", "revertedGroupId must omitempty when nil")

	parts, ok := generic["parts"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 2)

	part, ok := parts[0].(map[string]any)
	require.True(t, ok)

	expectedPartKeys := []string{"transactionId", "organizationId", "ledgerId", "role", "status"}
	for _, key := range expectedPartKeys {
		assert.Containsf(t, part, key, "part must include %q", key)
	}

	assert.Lenf(t, part, len(expectedPartKeys), "expected %d part fields, got %d (drift?)", len(expectedPartKeys), len(part))

	src := minimalTransactionGroupSource()
	src.RevertedGroupID = &groupEventReverted

	data, err = json.Marshal(events.NewTransactionGroup(src))
	require.NoError(t, err)

	generic = map[string]any{}
	require.NoError(t, json.Unmarshal(data, &generic))
	assert.Equal(t, groupEventReverted, generic["revertedGroupId"])
	assert.Len(t, generic, len(expectedKeys)+1)
}
