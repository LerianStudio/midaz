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

// TestHolderDeletedDefinition_Key locks the canonical event key.
func TestHolderDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "holder.deleted", events.HolderDeletedDefinition.Key())
	assert.Equal(t, "holder", events.HolderDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.HolderDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.HolderDeletedDefinition.SchemaVersion)
}

// TestNewHolderDeleted_SoftDelete verifies the soft-delete path: deletionType
// derives to "soft" when hardDelete is false.
func TestNewHolderDeleted_SoftDelete(t *testing.T) {
	payload := events.NewHolderDeleted(holderFixedID.String(), holderTestOrgID, false, holderFixedTime)

	assert.Equal(t, holderFixedID.String(), payload.ID)
	assert.Equal(t, holderTestOrgID, payload.OrganizationID)
	assert.Equal(t, "soft", payload.DeletionType)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

// TestNewHolderDeleted_HardDelete verifies the hard-delete path: deletionType
// derives to "hard" when hardDelete is true.
func TestNewHolderDeleted_HardDelete(t *testing.T) {
	payload := events.NewHolderDeleted(holderFixedID.String(), holderTestOrgID, true, holderFixedTime)

	assert.Equal(t, holderFixedID.String(), payload.ID)
	assert.Equal(t, holderTestOrgID, payload.OrganizationID)
	assert.Equal(t, "hard", payload.DeletionType)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

// TestHolderDeletedPayload_ToEmitRequest_AssemblesStreamingEvent verifies the
// ToEmitRequest helper composes a fully-populated EmitRequest.
func TestHolderDeletedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewHolderDeleted(holderFixedID.String(), holderTestOrgID, false, holderFixedTime)

	req, err := payload.ToEmitRequest("tenant-1", holderFixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.HolderDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, holderFixedTime, req.Timestamp)

	var roundTrip events.HolderDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestHolderDeletedPayload_JSONShape locks the wire JSON layout and asserts PII
// absence.
func TestHolderDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewHolderDeleted(holderFixedID.String(), holderTestOrgID, true, holderFixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"id", "organizationId", "deletionType", "deletedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"document", "cpf", "cnpj", "name", "type", "externalId",
		"contact", "addresses", "address",
		"naturalPerson", "legalPerson", "representative", "metadata",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include PII key %q", forbidden)
	}

	assert.Lenf(t, generic, 4, "expected 4 top-level fields, got %d (drift?)", len(generic))
}
