// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	tranID       = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ac10").String()
	tranOrg      = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ac11").String()
	tranLed      = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ac12").String()
	tranParent   = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ac13").String()
	tranOpID     = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ac14").String()
	tranAmount   = decimal.NewFromInt(1500)
	approvedCode = constant.APPROVED
	canceledCode = constant.CANCELED
)

// minimalTransactionSource returns a TransactionSource populated with
// the field set every per-event constructor consumes. Operations is a
// single pre-marshaled stub so the wire shape can be inspected without
// importing the internal operation type.
func minimalTransactionSource() events.TransactionSource {
	stubOp, _ := json.Marshal(map[string]any{
		"id":            tranOpID,
		"transactionId": tranID,
		"accountId":     uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ac20").String(),
		"direction":     constant.DirectionDebit,
		"assetCode":     "USD",
	})

	return events.TransactionSource{
		ID:                       tranID,
		OrganizationID:           tranOrg,
		LedgerID:                 tranLed,
		Status:                   mmodel.Status{Code: approvedCode, Description: &approvedCode},
		Amount:                   &tranAmount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "default",
		Description:              "test posting",
		Source:                   []string{"@external/cash"},
		Destination:              []string{"@person1"},
		Route:                    "default-route",
		Operations:               []json.RawMessage{stubOp},
		CreatedAt:                fixedTime,
		UpdatedAt:                fixedTime,
	}
}

func TestTransactionLifecycleDefinitions_Keys(t *testing.T) {
	// All four event_types are single-word and pass the lib-streaming
	// route-key regex (^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)+$).
	assert.Equal(t, "transaction.posted", events.TransactionPostedDefinition.Key())
	assert.Equal(t, "transaction.committed", events.TransactionCommittedDefinition.Key())
	assert.Equal(t, "transaction.canceled", events.TransactionCanceledDefinition.Key())
	assert.Equal(t, "transaction.reverted", events.TransactionRevertedDefinition.Key())

	for _, def := range []events.Definition{
		events.TransactionPostedDefinition,
		events.TransactionCommittedDefinition,
		events.TransactionCanceledDefinition,
		events.TransactionRevertedDefinition,
	} {
		assert.Equal(t, "transaction", def.ResourceType)
		assert.Equal(t, "1.0.0", def.SchemaVersion)
	}
}

func TestNewTransactionPosted_MapsAllSourceFields(t *testing.T) {
	src := minimalTransactionSource()
	payload := events.NewTransactionPosted(src)

	assert.Equal(t, src.ID, payload.ID)
	assert.Nil(t, payload.ParentTransactionID, "posted has no parent")
	assert.Equal(t, src.OrganizationID, payload.OrganizationID)
	assert.Equal(t, src.LedgerID, payload.LedgerID)
	assert.Equal(t, approvedCode, payload.Status.Code)
	require.NotNil(t, payload.Amount)
	assert.True(t, src.Amount.Equal(*payload.Amount))
	assert.Equal(t, "USD", payload.AssetCode)
	assert.Equal(t, "default", payload.ChartOfAccountsGroupName)
	assert.Equal(t, "test posting", payload.Description)
	assert.Equal(t, []string{"@external/cash"}, payload.Source)
	assert.Equal(t, []string{"@person1"}, payload.Destination)
	assert.Equal(t, "default-route", payload.Route)
	assert.Nil(t, payload.RouteID)
	require.Len(t, payload.Operations, 1)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewTransactionReverted_PopulatesParentTransactionID(t *testing.T) {
	src := minimalTransactionSource()
	src.ParentTransactionID = &tranParent

	payload := events.NewTransactionReverted(src)

	require.NotNil(t, payload.ParentTransactionID)
	assert.Equal(t, tranParent, *payload.ParentTransactionID)
}

func TestNewTransactionCommittedAndCanceled_ShareTheBasePayload(t *testing.T) {
	src := minimalTransactionSource()

	committed := events.NewTransactionCommitted(src)
	canceled := events.NewTransactionCanceled(src)

	// All non-discriminator fields agree — the four constructors share
	// newTransactionPayload by design.
	assert.Equal(t, committed.ID, canceled.ID)
	assert.Equal(t, committed.OrganizationID, canceled.OrganizationID)
	assert.Equal(t, committed.LedgerID, canceled.LedgerID)
	assert.Equal(t, committed.Status, canceled.Status)
	assert.Equal(t, len(committed.Operations), len(canceled.Operations))
}

func TestTransactionPayload_ToEmitRequest_AssemblesStreamingEvents(t *testing.T) {
	tests := []struct {
		name      string
		emit      func(events.TransactionPayload) (libStreaming.EmitRequest, error)
		expectKey string
	}{
		{
			name: "posted",
			emit: func(p events.TransactionPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestPosted("tenant-x", fixedTime)
			},
			expectKey: "transaction.posted",
		},
		{
			name: "committed",
			emit: func(p events.TransactionPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestCommitted("tenant-x", fixedTime)
			},
			expectKey: "transaction.committed",
		},
		{
			name: "canceled",
			emit: func(p events.TransactionPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestCanceled("tenant-x", fixedTime)
			},
			expectKey: "transaction.canceled",
		},
		{
			name: "reverted",
			emit: func(p events.TransactionPayload) (libStreaming.EmitRequest, error) {
				return p.ToEmitRequestReverted("tenant-x", fixedTime)
			},
			expectKey: "transaction.reverted",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			payload := events.NewTransactionPosted(minimalTransactionSource())

			req, err := tc.emit(payload)
			require.NoError(t, err)

			assert.Equal(t, tc.expectKey, req.DefinitionKey)
			assert.Equal(t, "tenant-x", req.TenantID)
			assert.Equal(t, tranID, req.Subject,
				"Subject must be the transaction id (consumer_idempotency_key_candidate)")
			assert.Equal(t, fixedTime, req.Timestamp)

			var roundTrip events.TransactionPayload
			require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
			assert.Equal(t, payload.ID, roundTrip.ID)
		})
	}
}

func TestTransactionPayload_JSONShape_OmitsScale(t *testing.T) {
	payload := events.NewTransactionPosted(minimalTransactionSource())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"id", "organizationId", "ledgerId", "status", "amount",
		"assetCode", "operations", "createdAt", "updatedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasParent := generic["parentTransactionId"]
	assert.False(t, hasParent, "parentTransactionId must omitempty when nil")

	_, hasScale := generic["scale"]
	assert.False(t, hasScale, "scale is intentionally omitted (asset-level property)")
}

func TestTransactionPayload_JSONShape_RevertCarriesParent(t *testing.T) {
	src := minimalTransactionSource()
	src.ParentTransactionID = &tranParent

	payload := events.NewTransactionReverted(src)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	parent, ok := generic["parentTransactionId"]
	require.True(t, ok, "reverted payload must include parentTransactionId")
	assert.Equal(t, tranParent, parent)
}

func TestTransactionPayload_CanceledStatusCarriesThroughWire(t *testing.T) {
	src := minimalTransactionSource()
	src.Status = mmodel.Status{Code: canceledCode, Description: &canceledCode}

	payload := events.NewTransactionCanceled(src)
	req, err := payload.ToEmitRequestCanceled("tenant-x", fixedTime)
	require.NoError(t, err)

	var roundTrip events.TransactionPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, canceledCode, roundTrip.Status.Code)
}
