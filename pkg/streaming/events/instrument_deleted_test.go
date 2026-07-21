// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInstrumentDeletedDefinition_Key locks the canonical event key.
func TestInstrumentDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "instrument.deleted", events.InstrumentDeletedDefinition.Key())
	assert.Equal(t, "instrument", events.InstrumentDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.InstrumentDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.InstrumentDeletedDefinition.SchemaVersion)
}

// TestNewInstrumentDeleted_SoftDelete verifies deletionType derives to "soft".
func TestNewInstrumentDeleted_SoftDelete(t *testing.T) {
	payload := events.NewInstrumentDeleted(instrumentFixedID.String(), instrumentHolderID.String(), instrumentTestOrgID, false, instrumentFixedTime)

	assert.Equal(t, instrumentFixedID.String(), payload.ID)
	assert.Equal(t, instrumentHolderID.String(), payload.HolderID)
	assert.Equal(t, instrumentTestOrgID, payload.OrganizationID)
	assert.Equal(t, "soft", payload.DeletionType)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

// TestNewInstrumentDeleted_HardDelete verifies deletionType derives to "hard".
func TestNewInstrumentDeleted_HardDelete(t *testing.T) {
	payload := events.NewInstrumentDeleted(instrumentFixedID.String(), instrumentHolderID.String(), instrumentTestOrgID, true, instrumentFixedTime)

	assert.Equal(t, "hard", payload.DeletionType)
}

// TestInstrumentDeletedPayload_ToEmitRequest_AssemblesStreamingEvent verifies
// the ToEmitRequest helper composes a fully-populated EmitRequest with Subject =
// instrument ID.
func TestInstrumentDeletedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewInstrumentDeleted(instrumentFixedID.String(), instrumentHolderID.String(), instrumentTestOrgID, false, instrumentFixedTime)

	req, err := payload.ToEmitRequest("tenant-1", instrumentFixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.InstrumentDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, instrumentFixedTime, req.Timestamp)

	var roundTrip events.InstrumentDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestInstrumentDeletedPayload_JSONShape locks the 5-key wire layout and asserts
// PII absence.
func TestInstrumentDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewInstrumentDeleted(instrumentFixedID.String(), instrumentHolderID.String(), instrumentTestOrgID, true, instrumentFixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"id", "holderId", "organizationId", "deletionType", "deletedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"type", "externalId", "ledgerId", "accountId",
		"document", "name", "bankingDetails", "iban",
		"regulatoryFields", "relatedParties", "metadata",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include key %q", forbidden)
	}

	assert.Lenf(t, generic, 5, "expected 5 top-level fields, got %d (drift?)", len(generic))
}
