// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionRouteUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "transaction-route.updated", events.TransactionRouteUpdatedDefinition.Key())
	assert.Equal(t, "transaction-route", events.TransactionRouteUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.TransactionRouteUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.TransactionRouteUpdatedDefinition.SchemaVersion)
}

func TestNewTransactionRouteUpdated_MapsMinimalTransactionRoute(t *testing.T) {
	tr := minimalTransactionRoute()

	payload := events.NewTransactionRouteUpdated(tr)

	assert.Equal(t, transactionRouteID.String(), payload.ID)
	assert.Equal(t, transactionRouteOrg.String(), payload.OrganizationID)
	assert.Equal(t, transactionRouteLed.String(), payload.LedgerID)
	assert.Equal(t, "Charge Settlement", payload.Title)
	assert.Empty(t, payload.Description)
	assert.Equal(t, []string{transactionRouteOR1.String(), transactionRouteOR2.String()}, payload.OperationRouteIDs)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestTransactionRouteUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewTransactionRouteUpdated(minimalTransactionRoute())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.TransactionRouteUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.TransactionRouteUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestTransactionRouteUpdatedPayload_JSONShape_OmitsCreatedAt(t *testing.T) {
	payload := events.NewTransactionRouteUpdated(minimalTransactionRoute())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "title", "operationRouteIds", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasCreatedAt := generic["createdAt"]
	assert.False(t, hasCreatedAt, "createdAt must NOT appear on transaction-route.updated")

	_, hasDescription := generic["description"]
	assert.False(t, hasDescription, "description must omitempty when empty")

	assert.Lenf(t, generic, 6, "expected 6 top-level fields when description omitted, got %d (drift?)", len(generic))
}
