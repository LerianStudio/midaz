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

// TestInstrumentRelatedPartyDeletedDefinition_Key locks the canonical event key.
// The HYPHEN in "related-party-deleted" is the sensitive point: the route-key
// validator rejects underscores, so this must never silently become
// "related_party_deleted".
func TestInstrumentRelatedPartyDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "instrument.related-party-deleted", events.InstrumentRelatedPartyDeletedDefinition.Key())
	assert.Equal(t, "instrument", events.InstrumentRelatedPartyDeletedDefinition.ResourceType)
	assert.Equal(t, "related-party-deleted", events.InstrumentRelatedPartyDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.InstrumentRelatedPartyDeletedDefinition.SchemaVersion)
}

// TestNewInstrumentRelatedPartyDeleted_MapsFields verifies the 5-field mapping.
func TestNewInstrumentRelatedPartyDeleted_MapsFields(t *testing.T) {
	payload := events.NewInstrumentRelatedPartyDeleted(
		instrumentFixedID.String(), instrumentHolderID.String(), instrumentTestOrgID,
		relatedPartyOneID.String(), instrumentFixedTime,
	)

	assert.Equal(t, instrumentFixedID.String(), payload.InstrumentID)
	assert.Equal(t, instrumentHolderID.String(), payload.HolderID)
	assert.Equal(t, instrumentTestOrgID, payload.OrganizationID)
	assert.Equal(t, relatedPartyOneID.String(), payload.RelatedPartyID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

// TestInstrumentRelatedPartyDeletedPayload_ToEmitRequest_SubjectIsInstrumentID
// verifies the ToEmitRequest helper composes a fully-populated EmitRequest whose
// Subject is the INSTRUMENT ID (the aggregate), NOT the related-party ID.
func TestInstrumentRelatedPartyDeletedPayload_ToEmitRequest_SubjectIsInstrumentID(t *testing.T) {
	payload := events.NewInstrumentRelatedPartyDeleted(
		instrumentFixedID.String(), instrumentHolderID.String(), instrumentTestOrgID,
		relatedPartyOneID.String(), instrumentFixedTime,
	)

	req, err := payload.ToEmitRequest("tenant-1", instrumentFixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.InstrumentRelatedPartyDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)

	// The aggregate is the instrument, so ce-subject is the instrument ID, not
	// the related-party ID.
	assert.Equal(t, instrumentFixedID.String(), req.Subject)
	assert.NotEqual(t, relatedPartyOneID.String(), req.Subject)

	assert.Equal(t, instrumentFixedTime, req.Timestamp)

	var roundTrip events.InstrumentRelatedPartyDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestInstrumentRelatedPartyDeletedPayload_JSONShape locks the 5-key wire layout
// and asserts PII absence. Note deletionType is DELIBERATELY absent (removing a
// related party is always a pointwise removal, no soft/hard distinction).
func TestInstrumentRelatedPartyDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewInstrumentRelatedPartyDeleted(
		instrumentFixedID.String(), instrumentHolderID.String(), instrumentTestOrgID,
		relatedPartyOneID.String(), instrumentFixedTime,
	)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"instrumentId", "holderId", "organizationId", "relatedPartyId", "deletedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"deletionType", "document", "name", "role",
		"startDate", "endDate", "type",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include key %q", forbidden)
	}

	assert.Lenf(t, generic, 5, "expected 5 top-level fields, got %d (drift?)", len(generic))
}
