// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	transactionRouteID  = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	transactionRouteOrg = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")
	transactionRouteLed = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0c")
	transactionRouteOR1 = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0d")
	transactionRouteOR2 = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0e")
)

func minimalTransactionRoute() *mmodel.TransactionRoute {
	return &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: transactionRouteOrg,
		LedgerID:       transactionRouteLed,
		Title:          "Charge Settlement",
		OperationRoutes: []mmodel.OperationRoute{
			{ID: transactionRouteOR1, OperationType: "source"},
			{ID: transactionRouteOR2, OperationType: "destination"},
		},
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
}

func TestTransactionRouteCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "transaction-route.created", events.TransactionRouteCreatedDefinition.Key())
	assert.Equal(t, "transaction-route", events.TransactionRouteCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.TransactionRouteCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.TransactionRouteCreatedDefinition.SchemaVersion)
}

func TestNewTransactionRouteCreated_MapsMinimalTransactionRoute(t *testing.T) {
	tr := minimalTransactionRoute()

	payload := events.NewTransactionRouteCreated(tr)

	assert.Equal(t, transactionRouteID.String(), payload.ID)
	assert.Equal(t, transactionRouteOrg.String(), payload.OrganizationID)
	assert.Equal(t, transactionRouteLed.String(), payload.LedgerID)
	assert.Equal(t, "Charge Settlement", payload.Title)
	assert.Empty(t, payload.Description)
	assert.Equal(t, []string{transactionRouteOR1.String(), transactionRouteOR2.String()}, payload.OperationRouteIDs)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewTransactionRouteCreated_MapsDescription(t *testing.T) {
	tr := minimalTransactionRoute()
	tr.Description = "Settlement route for service charges"

	payload := events.NewTransactionRouteCreated(tr)

	assert.Equal(t, "Settlement route for service charges", payload.Description)
}

func TestNewTransactionRouteCreated_EmptyOperationRoutesProducesEmptySlice(t *testing.T) {
	tr := minimalTransactionRoute()
	tr.OperationRoutes = nil

	payload := events.NewTransactionRouteCreated(tr)

	// Non-nil empty slice — `omitempty` drops it from the wire, but
	// callers reading the struct don't need to nil-check.
	assert.NotNil(t, payload.OperationRouteIDs)
	assert.Len(t, payload.OperationRouteIDs, 0)
}

func TestTransactionRouteCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewTransactionRouteCreated(minimalTransactionRoute())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.TransactionRouteCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.TransactionRouteCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestTransactionRouteCreatedPayload_JSONShape_MinimalOmitsDescription(t *testing.T) {
	payload := events.NewTransactionRouteCreated(minimalTransactionRoute())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "title", "operationRouteIds", "createdAt", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasDescription := generic["description"]
	assert.False(t, hasDescription, "description must omitempty when empty")

	assert.Lenf(t, generic, 7, "expected 7 top-level fields when description omitted, got %d (drift?)", len(generic))
}

func TestTransactionRouteCreatedPayload_JSONShape_WithDescriptionIncludesIt(t *testing.T) {
	tr := minimalTransactionRoute()
	tr.Description = "Long-form description"

	payload := events.NewTransactionRouteCreated(tr)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	assert.Lenf(t, generic, 8, "expected 8 top-level fields when description present, got %d (drift?)", len(generic))
}
