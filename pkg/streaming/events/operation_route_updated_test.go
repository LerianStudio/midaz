// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationRouteUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "operation-route.updated", events.OperationRouteUpdatedDefinition.Key())
	assert.Equal(t, "operation-route", events.OperationRouteUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.OperationRouteUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.OperationRouteUpdatedDefinition.SchemaVersion)
}

func TestNewOperationRouteUpdated_MapsMinimalOperationRoute(t *testing.T) {
	o := minimalOperationRoute()

	payload := events.NewOperationRouteUpdated(o)

	assert.Equal(t, operationRouteID.String(), payload.ID)
	assert.Equal(t, operationRouteOrg.String(), payload.OrganizationID)
	assert.Equal(t, operationRouteLed.String(), payload.LedgerID)
	assert.Equal(t, "Cashin from service charge", payload.Title)
	assert.Empty(t, payload.Description)
	assert.Empty(t, payload.Code)
	assert.Equal(t, "source", payload.OperationType)
	assert.Nil(t, payload.Account)
	assert.Nil(t, payload.AccountingEntries)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewOperationRouteUpdated_PreservesPostCommitAccountRule(t *testing.T) {
	o := operationRouteWithAccountRule()

	payload := events.NewOperationRouteUpdated(o)

	require.NotNil(t, payload.Account)
	assert.Equal(t, "alias", payload.Account.RuleType)
	assert.Equal(t, "@cash_account", payload.Account.ValidIf)
}

func TestOperationRouteUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewOperationRouteUpdated(minimalOperationRoute())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.OperationRouteUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.OperationRouteUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestOperationRouteUpdatedPayload_JSONShape_OmitsCreatedAt(t *testing.T) {
	payload := events.NewOperationRouteUpdated(minimalOperationRoute())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "title", "operationType", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasCreatedAt := generic["createdAt"]
	assert.False(t, hasCreatedAt, "createdAt must NOT appear on operation-route.updated")

	for _, key := range []string{"description", "code", "account", "accountingEntries"} {
		_, has := generic[key]
		assert.Falsef(t, has, "%q must omitempty when zero/nil", key)
	}

	assert.Lenf(t, generic, 6, "expected 6 top-level fields when optionals omitted, got %d (drift?)", len(generic))
}

func TestOperationRouteUpdatedPayload_JSONShape_WithAccountingEntries(t *testing.T) {
	o := minimalOperationRoute()
	o.AccountingEntries = &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{
			Debit: &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
		},
	}

	payload := events.NewOperationRouteUpdated(o)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	_, hasEntries := generic["accountingEntries"]
	assert.True(t, hasEntries, "accountingEntries must be present when set")
}
