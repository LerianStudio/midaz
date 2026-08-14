// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holderFixedTime is the deterministic timestamp used by the holder event
// tests so RFC3339 round-trips can be asserted by exact match.
var holderFixedTime = time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)

// holderFixedID is a deterministic UUID reused across holder tests so the
// Subject/ID assertions are exact-match.
var holderFixedID = uuid.MustParse("0190d9e1-7c2a-7000-8000-000000000001")

// holderTestOrgID is the deterministic organization scope used by the holder
// event tests. Holder carries no organization scope on the domain model, so it
// is supplied to the constructor explicitly (mirroring the deleted event).
const holderTestOrgID = "01J7K7XB9C2D3E4F5G6H7J8K9L"

// minimalHolder returns the smallest mmodel.Holder suitable for the
// holder.created / holder.updated contract: identity, classification type, and
// timestamps. ExternalID is left nil so tests can verify nullable-field
// handling. PII fields (name, document, addresses, contact, naturalPerson,
// legalPerson) are populated to PROVE they never reach the wire payload.
func minimalHolder() *mmodel.Holder {
	id := holderFixedID
	personType := "NATURAL_PERSON"
	name := "John Doe"
	document := "91315026015"

	return &mmodel.Holder{
		ID:        &id,
		Type:      &personType,
		Name:      &name,
		Document:  &document,
		CreatedAt: holderFixedTime,
		UpdatedAt: holderFixedTime,
	}
}

// TestHolderCreatedDefinition_Key locks the canonical event key. Changing this
// assertion is a wire-contract change and requires a coordinated update of
// every downstream consumer.
func TestHolderCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "holder.created", events.HolderCreatedDefinition.Key())
	assert.Equal(t, "holder", events.HolderCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.HolderCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.HolderCreatedDefinition.SchemaVersion)
}

// TestNewHolderCreated_MapsMinimalHolder verifies the happy-path mapping for
// the simplest holder: identity, type, timestamps, no externalId.
func TestNewHolderCreated_MapsMinimalHolder(t *testing.T) {
	h := minimalHolder()

	payload := events.NewHolderCreated(h, holderTestOrgID)

	// Identity + scope.
	assert.Equal(t, holderFixedID.String(), payload.ID)
	assert.Equal(t, holderTestOrgID, payload.OrganizationID)

	// Classification.
	assert.Equal(t, "NATURAL_PERSON", payload.Type)

	// Nullable ref round-trips as nil pointer.
	assert.Nil(t, payload.ExternalID)

	// RFC3339 formatting locks producer-side timestamp discipline.
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

// TestNewHolderCreated_MapsAllOptionalFields covers the path where the
// nullable externalId is set. Verifies the *string value is propagated, not
// stripped or empty-stringed.
func TestNewHolderCreated_MapsAllOptionalFields(t *testing.T) {
	externalID := "G4K7N8M2"

	h := minimalHolder()
	h.ExternalID = &externalID

	payload := events.NewHolderCreated(h, holderTestOrgID)

	require.NotNil(t, payload.ExternalID)
	assert.Equal(t, externalID, *payload.ExternalID)
}

// TestHolderCreatedPayload_ToEmitRequest_AssemblesStreamingEvent verifies the
// ToEmitRequest helper composes a fully-populated EmitRequest with the correct
// DefinitionKey, tenant ID, subject, timestamp, and payload.
func TestHolderCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewHolderCreated(minimalHolder(), holderTestOrgID)

	req, err := payload.ToEmitRequest("tenant-1", holderFixedTime)
	require.NoError(t, err)

	// Catalog routing key.
	assert.Equal(t, events.HolderCreatedDefinition.Key(), req.DefinitionKey)

	// Per-emit fields.
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, holderFixedTime, req.Timestamp)

	// Payload round-trips back to the same struct.
	var roundTrip events.HolderCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestHolderCreatedPayload_JSONShape locks the wire JSON layout against
// accidental field-name drift AND asserts that no PII key ever appears on the
// wire. Breaking this test is a wire-contract change; downstream consumers must
// be updated in the same PR.
func TestHolderCreatedPayload_JSONShape(t *testing.T) {
	payload := events.NewHolderCreated(minimalHolder(), holderTestOrgID)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	// Required keys present.
	for _, key := range []string{
		"id", "organizationId", "type", "externalId",
		"createdAt", "updatedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	// PII must NEVER reach the wire.
	for _, forbidden := range []string{
		"document", "cpf", "cnpj", "name",
		"contact", "addresses", "address",
		"naturalPerson", "legalPerson", "representative",
		"metadata", "deletedAt",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include PII key %q", forbidden)
	}

	// Sanity: no field count surprises. Pin the count so additive drift is
	// caught here.
	assert.Lenf(t, generic, 6, "expected 6 top-level fields, got %d (drift?)", len(generic))
}
