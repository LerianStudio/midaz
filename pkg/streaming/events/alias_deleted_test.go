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

// TestAliasDeletedDefinition_Key locks the canonical event key.
func TestAliasDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "alias.deleted", events.AliasDeletedDefinition.Key())
	assert.Equal(t, "alias", events.AliasDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.AliasDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AliasDeletedDefinition.SchemaVersion)
}

// TestNewAliasDeleted_SoftDelete verifies deletionType derives to "soft".
func TestNewAliasDeleted_SoftDelete(t *testing.T) {
	payload := events.NewAliasDeleted(aliasFixedID.String(), aliasHolderID.String(), aliasTestOrgID, false, aliasFixedTime)

	assert.Equal(t, aliasFixedID.String(), payload.ID)
	assert.Equal(t, aliasHolderID.String(), payload.HolderID)
	assert.Equal(t, aliasTestOrgID, payload.OrganizationID)
	assert.Equal(t, "soft", payload.DeletionType)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

// TestNewAliasDeleted_HardDelete verifies deletionType derives to "hard".
func TestNewAliasDeleted_HardDelete(t *testing.T) {
	payload := events.NewAliasDeleted(aliasFixedID.String(), aliasHolderID.String(), aliasTestOrgID, true, aliasFixedTime)

	assert.Equal(t, "hard", payload.DeletionType)
}

// TestAliasDeletedPayload_ToEmitRequest_AssemblesStreamingEvent verifies the
// ToEmitRequest helper composes a fully-populated EmitRequest with Subject =
// alias ID.
func TestAliasDeletedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAliasDeleted(aliasFixedID.String(), aliasHolderID.String(), aliasTestOrgID, false, aliasFixedTime)

	req, err := payload.ToEmitRequest("tenant-1", aliasFixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.AliasDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, aliasFixedTime, req.Timestamp)

	var roundTrip events.AliasDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAliasDeletedPayload_JSONShape locks the 5-key wire layout and asserts PII
// absence.
func TestAliasDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewAliasDeleted(aliasFixedID.String(), aliasHolderID.String(), aliasTestOrgID, true, aliasFixedTime)

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
