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

func minimalLedger() *mmodel.Ledger {
	return &mmodel.Ledger{
		ID:             "01J7K8FN5W8R0R2S7Q1V4H6J0M",
		OrganizationID: "01J7K8FN5W8R0R2S7Q1V4H6J01",
		Name:           "Treasury Operations",
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

func TestLedgerCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "ledger.created", events.LedgerCreatedDefinition.Key())
	assert.Equal(t, "ledger", events.LedgerCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.LedgerCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.LedgerCreatedDefinition.SchemaVersion)
}

func TestNewLedgerCreated_MapsMinimalLedger(t *testing.T) {
	led := minimalLedger()

	payload := events.NewLedgerCreated(led)

	assert.Equal(t, led.ID, payload.ID)
	assert.Equal(t, led.OrganizationID, payload.OrganizationID)
	assert.Equal(t, led.Name, payload.Name)
	assert.Equal(t, "ACTIVE", payload.Status.Code)
	assert.Nil(t, payload.Status.Description)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewLedgerCreated_MapsAllOptionalFields(t *testing.T) {
	statusDesc := "Active ledger"

	led := minimalLedger()
	led.Status.Description = &statusDesc

	payload := events.NewLedgerCreated(led)

	require.NotNil(t, payload.Status.Description)
	assert.Equal(t, statusDesc, *payload.Status.Description)
}

func TestLedgerCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewLedgerCreated(minimalLedger())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.LedgerCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.LedgerCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestLedgerCreatedPayload_JSONShape(t *testing.T) {
	payload := events.NewLedgerCreated(minimalLedger())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "name", "status", "createdAt", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	status, ok := generic["status"].(map[string]any)
	require.True(t, ok, "status must serialize as an object")
	_, hasStatusDesc := status["description"]
	assert.False(t, hasStatusDesc, "status.description must omitempty when nil")

	assert.Lenf(t, generic, 6, "expected 6 top-level fields, got %d (drift?)", len(generic))
}
