// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalUpdatedHolder returns the smallest mmodel.Holder suitable for the
// holder.updated contract. PII fields are populated so the JSONShape test can
// prove they never reach the wire.
func minimalUpdatedHolder() *mmodel.Holder {
	id := holderFixedID
	personType := "LEGAL_PERSON"
	name := "Acme Ltd"
	document := "11444777000161"

	return &mmodel.Holder{
		ID:        &id,
		Type:      &personType,
		Name:      &name,
		Document:  &document,
		CreatedAt: holderFixedTime,
		UpdatedAt: holderFixedTime,
	}
}

// TestHolderUpdatedDefinition_Key locks the canonical event key.
func TestHolderUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "holder.updated", events.HolderUpdatedDefinition.Key())
	assert.Equal(t, "holder", events.HolderUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.HolderUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.HolderUpdatedDefinition.SchemaVersion)
}

// TestNewHolderUpdated_MapsMinimalHolder verifies the happy-path mapping for
// the simplest holder: identity, type, timestamps, no externalId.
func TestNewHolderUpdated_MapsMinimalHolder(t *testing.T) {
	h := minimalUpdatedHolder()

	payload := events.NewHolderUpdated(h, holderTestOrgID)

	assert.Equal(t, holderFixedID.String(), payload.ID)
	assert.Equal(t, holderTestOrgID, payload.OrganizationID)
	assert.Equal(t, "LEGAL_PERSON", payload.Type)
	assert.Nil(t, payload.ExternalID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

// TestNewHolderUpdated_MapsAllOptionalFields covers the path where the
// nullable externalId is set.
func TestNewHolderUpdated_MapsAllOptionalFields(t *testing.T) {
	externalID := "G4K7N8M2"

	h := minimalUpdatedHolder()
	h.ExternalID = &externalID

	payload := events.NewHolderUpdated(h, holderTestOrgID)

	require.NotNil(t, payload.ExternalID)
	assert.Equal(t, externalID, *payload.ExternalID)
}

// TestHolderUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent verifies the
// ToEmitRequest helper composes a fully-populated EmitRequest.
func TestHolderUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewHolderUpdated(minimalUpdatedHolder(), holderTestOrgID)

	req, err := payload.ToEmitRequest("tenant-1", holderFixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.HolderUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, holderFixedTime, req.Timestamp)

	var roundTrip events.HolderUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestHolderUpdatedPayload_JSONShape locks the wire JSON layout and asserts PII
// absence.
func TestHolderUpdatedPayload_JSONShape(t *testing.T) {
	payload := events.NewHolderUpdated(minimalUpdatedHolder(), holderTestOrgID)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"id", "organizationId", "type", "externalId",
		"createdAt", "updatedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"document", "cpf", "cnpj", "name",
		"contact", "addresses", "address",
		"naturalPerson", "legalPerson", "representative",
		"metadata", "deletedAt",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include PII key %q", forbidden)
	}

	assert.Lenf(t, generic, 6, "expected 6 top-level fields, got %d (drift?)", len(generic))
}
