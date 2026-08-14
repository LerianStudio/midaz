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

func TestOrganizationUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "organization.updated", events.OrganizationUpdatedDefinition.Key())
	assert.Equal(t, "organization", events.OrganizationUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.OrganizationUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.OrganizationUpdatedDefinition.SchemaVersion)
}

func TestNewOrganizationUpdated_MapsMinimalOrganization(t *testing.T) {
	org := minimalOrganization()

	payload := events.NewOrganizationUpdated(org)

	assert.Equal(t, org.ID, payload.ID)
	assert.Nil(t, payload.ParentOrganizationID)
	assert.Equal(t, org.LegalName, payload.LegalName)
	assert.Nil(t, payload.DoingBusinessAs)
	assert.Equal(t, "ACTIVE", payload.Status.Code)
	assert.Nil(t, payload.Status.Description)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
	assert.Nil(t, payload.Address.Line2)
	assert.Nil(t, payload.Address.Description)
}

func TestNewOrganizationUpdated_MapsAllOptionalFields(t *testing.T) {
	parentID := "01J7K8FN5W8R0R2S7Q1V4H6J01"
	dba := "Lerian FS"
	line2 := "Suite 1500"
	addressDesc := "Headquarters"
	statusDesc := "Active organization"

	org := minimalOrganization()
	org.ParentOrganizationID = &parentID
	org.DoingBusinessAs = &dba
	org.Address = mmodel.Address{
		Line1:       "123 Financial Avenue",
		Line2:       &line2,
		ZipCode:     "10001",
		City:        "New York",
		State:       "NY",
		Country:     "US",
		Description: &addressDesc,
	}
	org.Status.Description = &statusDesc

	payload := events.NewOrganizationUpdated(org)

	require.NotNil(t, payload.ParentOrganizationID)
	assert.Equal(t, parentID, *payload.ParentOrganizationID)
	require.NotNil(t, payload.DoingBusinessAs)
	assert.Equal(t, dba, *payload.DoingBusinessAs)
	require.NotNil(t, payload.Address.Line2)
	assert.Equal(t, line2, *payload.Address.Line2)
	require.NotNil(t, payload.Address.Description)
	assert.Equal(t, addressDesc, *payload.Address.Description)
	require.NotNil(t, payload.Status.Description)
	assert.Equal(t, statusDesc, *payload.Status.Description)
}

func TestOrganizationUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewOrganizationUpdated(minimalOrganization())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.OrganizationUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.OrganizationUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestOrganizationUpdatedPayload_JSONShape(t *testing.T) {
	payload := events.NewOrganizationUpdated(minimalOrganization())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "parentOrganizationId", "legalName", "doingBusinessAs", "address", "status", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	address, ok := generic["address"].(map[string]any)
	require.True(t, ok, "address must serialize as an object")
	assert.Contains(t, address, "line2", "address.line2 must preserve non-omitempty JSON behavior")
	_, hasAddressDesc := address["description"]
	assert.False(t, hasAddressDesc, "address.description must omitempty when nil")

	status, ok := generic["status"].(map[string]any)
	require.True(t, ok, "status must serialize as an object")
	_, hasStatusDesc := status["description"]
	assert.False(t, hasStatusDesc, "status.description must omitempty when nil")

	assert.Lenf(t, generic, 7, "expected 7 top-level fields, got %d (drift?)", len(generic))
}
