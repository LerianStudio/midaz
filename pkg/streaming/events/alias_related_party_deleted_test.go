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

// TestAliasRelatedPartyDeletedDefinition_Key locks the canonical event key. The
// HYPHEN in "related-party-deleted" is the sensitive point: the route-key
// validator rejects underscores, so this must never silently become
// "related_party_deleted".
func TestAliasRelatedPartyDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "alias.related-party-deleted", events.AliasRelatedPartyDeletedDefinition.Key())
	assert.Equal(t, "alias", events.AliasRelatedPartyDeletedDefinition.ResourceType)
	assert.Equal(t, "related-party-deleted", events.AliasRelatedPartyDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AliasRelatedPartyDeletedDefinition.SchemaVersion)
}

// TestNewAliasRelatedPartyDeleted_MapsFields verifies the 5-field mapping.
func TestNewAliasRelatedPartyDeleted_MapsFields(t *testing.T) {
	payload := events.NewAliasRelatedPartyDeleted(
		aliasFixedID.String(), aliasHolderID.String(), aliasTestOrgID,
		relatedPartyOneID.String(), aliasFixedTime,
	)

	assert.Equal(t, aliasFixedID.String(), payload.AliasID)
	assert.Equal(t, aliasHolderID.String(), payload.HolderID)
	assert.Equal(t, aliasTestOrgID, payload.OrganizationID)
	assert.Equal(t, relatedPartyOneID.String(), payload.RelatedPartyID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

// TestAliasRelatedPartyDeletedPayload_ToEmitRequest_SubjectIsAliasID verifies
// the ToEmitRequest helper composes a fully-populated EmitRequest whose Subject
// is the ALIAS ID (the aggregate), NOT the related-party ID.
func TestAliasRelatedPartyDeletedPayload_ToEmitRequest_SubjectIsAliasID(t *testing.T) {
	payload := events.NewAliasRelatedPartyDeleted(
		aliasFixedID.String(), aliasHolderID.String(), aliasTestOrgID,
		relatedPartyOneID.String(), aliasFixedTime,
	)

	req, err := payload.ToEmitRequest("tenant-1", aliasFixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.AliasRelatedPartyDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)

	// The aggregate is the alias, so ce-subject is the alias ID, not the
	// related-party ID.
	assert.Equal(t, aliasFixedID.String(), req.Subject)
	assert.NotEqual(t, relatedPartyOneID.String(), req.Subject)

	assert.Equal(t, aliasFixedTime, req.Timestamp)

	var roundTrip events.AliasRelatedPartyDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAliasRelatedPartyDeletedPayload_JSONShape locks the 5-key wire layout and
// asserts PII absence. Note deletionType is DELIBERATELY absent (removing a
// related party is always a pointwise removal, no soft/hard distinction).
func TestAliasRelatedPartyDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewAliasRelatedPartyDeleted(
		aliasFixedID.String(), aliasHolderID.String(), aliasTestOrgID,
		relatedPartyOneID.String(), aliasFixedTime,
	)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"aliasId", "holderId", "organizationId", "relatedPartyId", "deletedAt",
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
