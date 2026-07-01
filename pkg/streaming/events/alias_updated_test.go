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

// TestAliasUpdatedDefinition_Key locks the canonical event key.
func TestAliasUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "alias.updated", events.AliasUpdatedDefinition.Key())
	assert.Equal(t, "alias", events.AliasUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.AliasUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AliasUpdatedDefinition.SchemaVersion)
}

// TestNewAliasUpdated_MapsMinimalAlias verifies the happy-path mapping for the
// simplest alias.
func TestNewAliasUpdated_MapsMinimalAlias(t *testing.T) {
	a := minimalAlias()

	payload := events.NewAliasUpdated(a, aliasTestOrgID)

	assert.Equal(t, aliasFixedID.String(), payload.ID)
	assert.Equal(t, aliasHolderID.String(), payload.HolderID)
	assert.Equal(t, aliasTestOrgID, payload.OrganizationID)
	assert.Equal(t, "01J7K7XB9C2D3E4F5G6H7LEDGR", payload.LedgerID)
	assert.Equal(t, "01J7K7XB9C2D3E4F5G6H7ACCNT", payload.AccountID)
	assert.Equal(t, "LEGAL_PERSON", payload.Type)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
	assert.Nil(t, payload.RelatedParties)
}

// TestNewAliasUpdated_MapsRelatedParties covers the multi-entry path and PROVES
// only relatedPartyId + role cross the wire.
func TestNewAliasUpdated_MapsRelatedParties(t *testing.T) {
	a := aliasWithRelatedParties()

	payload := events.NewAliasUpdated(a, aliasTestOrgID)

	require.Len(t, payload.RelatedParties, 2)
	assert.Equal(t, relatedPartyOneID.String(), payload.RelatedParties[0].RelatedPartyID)
	assert.Equal(t, "PRIMARY_HOLDER", payload.RelatedParties[0].Role)
	assert.Equal(t, relatedPartyTwoID.String(), payload.RelatedParties[1].RelatedPartyID)
	assert.Equal(t, "LEGAL_REPRESENTATIVE", payload.RelatedParties[1].Role)
}

// TestAliasUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent verifies the
// ToEmitRequest helper composes a fully-populated EmitRequest.
func TestAliasUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAliasUpdated(aliasWithRelatedParties(), aliasTestOrgID)

	req, err := payload.ToEmitRequest("tenant-1", aliasFixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.AliasUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, aliasFixedTime, req.Timestamp)

	var roundTrip events.AliasUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAliasUpdatedPayload_JSONShape_NilRelatedPartiesIsNull locks the intended
// wire contract for an alias with no related parties: relatedParties MUST encode
// as JSON null, never as an empty array. A future switch to a non-nil empty
// slice would silently flip the wire to [] and break consumers that distinguish
// absent from empty.
func TestAliasUpdatedPayload_JSONShape_NilRelatedPartiesIsNull(t *testing.T) {
	payload := events.NewAliasUpdated(minimalAlias(), aliasTestOrgID)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &generic))

	raw, present := generic["relatedParties"]
	require.Truef(t, present, "wire payload must include %q", "relatedParties")

	assert.JSONEq(t, "null", string(raw), "empty relatedParties must be JSON null, not []")
	assert.NotEqual(t, "[]", string(raw), "empty relatedParties must NOT be an empty array")

	// Round-trip the minimal (nil-slice) payload to prove marshal->unmarshal
	// preserves equality, mirroring the populated-slice round-trip.
	var roundTrip events.AliasUpdatedPayload
	require.NoError(t, json.Unmarshal(data, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAliasUpdatedPayload_JSONShape locks the wire JSON layout and asserts PII
// absence at the top level and inside each relatedParties element.
func TestAliasUpdatedPayload_JSONShape(t *testing.T) {
	payload := events.NewAliasUpdated(aliasWithRelatedParties(), aliasTestOrgID)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"id", "holderId", "organizationId", "ledgerId", "accountId",
		"type", "createdAt", "updatedAt", "relatedParties",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"externalId", "document", "cpf", "cnpj", "name",
		"bankingDetails", "iban", "branch", "account",
		"regulatoryFields", "participantDocument", "metadata", "deletedAt",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include key %q", forbidden)
	}

	assert.Lenf(t, generic, 9, "expected 9 top-level fields, got %d (drift?)", len(generic))

	rawParties, ok := generic["relatedParties"].([]any)
	require.Truef(t, ok, "relatedParties must be a JSON array")
	require.NotEmpty(t, rawParties)

	elem, ok := rawParties[0].(map[string]any)
	require.Truef(t, ok, "relatedParties element must be a JSON object")

	for _, key := range []string{"relatedPartyId", "role"} {
		_, present := elem[key]
		assert.Truef(t, present, "relatedParties element must include %q", key)
	}

	for _, forbidden := range []string{
		"id", "document", "name", "startDate", "endDate",
	} {
		_, present := elem[forbidden]
		assert.Falsef(t, present, "relatedParties element must NOT include key %q", forbidden)
	}

	assert.Lenf(t, elem, 2, "expected 2 relatedParties element fields, got %d (drift?)", len(elem))
}
