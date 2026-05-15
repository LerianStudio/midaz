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

func TestOrganizationDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "organization.deleted", events.OrganizationDeletedDefinition.Key())
	assert.Equal(t, "organization", events.OrganizationDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.OrganizationDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.OrganizationDeletedDefinition.SchemaVersion)
}

func TestNewOrganizationDeleted_MapsMinimalOrganization(t *testing.T) {
	payload := events.NewOrganizationDeleted("01J7K8FN5W8R0R2S7Q1V4H6J0M", fixedTime)

	assert.Equal(t, "01J7K8FN5W8R0R2S7Q1V4H6J0M", payload.ID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestNewOrganizationDeleted_MapsOptionalFields(t *testing.T) {
	payload := events.NewOrganizationDeleted("org-123", fixedTime)

	assert.Equal(t, "org-123", payload.ID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestOrganizationDeletedPayload_ToEvent_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewOrganizationDeleted("org-123", fixedTime)

	evt, err := payload.ToEvent("tenant-1", "lerian.midaz.ledger", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.OrganizationDeletedDefinition.ResourceType, evt.ResourceType)
	assert.Equal(t, events.OrganizationDeletedDefinition.EventType, evt.EventType)
	assert.Equal(t, events.OrganizationDeletedDefinition.SchemaVersion, evt.SchemaVersion)
	assert.Equal(t, "tenant-1", evt.TenantID)
	assert.Equal(t, "lerian.midaz.ledger", evt.Source)
	assert.Equal(t, payload.ID, evt.Subject)
	assert.Equal(t, fixedTime, evt.Timestamp)

	var roundTrip events.OrganizationDeletedPayload
	require.NoError(t, json.Unmarshal(evt.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestOrganizationDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewOrganizationDeleted("org-123", fixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "deletedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	assert.Lenf(t, generic, 2, "expected 2 top-level fields, got %d (drift?)", len(generic))
}
