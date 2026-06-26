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

func TestAssetUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "asset.updated", events.AssetUpdatedDefinition.Key())
	assert.Equal(t, "asset", events.AssetUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.AssetUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AssetUpdatedDefinition.SchemaVersion)
}

func TestNewAssetUpdated_MapsMinimalAsset(t *testing.T) {
	a := minimalAsset()

	payload := events.NewAssetUpdated(a)

	assert.Equal(t, a.ID, payload.ID)
	assert.Equal(t, a.OrganizationID, payload.OrganizationID)
	assert.Equal(t, a.LedgerID, payload.LedgerID)
	assert.Equal(t, a.Name, payload.Name)
	assert.Equal(t, a.Type, payload.Type)
	assert.Equal(t, a.Code, payload.Code)
	assert.Equal(t, "ACTIVE", payload.Status.Code)
	assert.Nil(t, payload.Status.Description)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewAssetUpdated_MapsAllOptionalFields(t *testing.T) {
	statusDesc := "Active asset"

	a := minimalAsset()
	a.Status.Description = &statusDesc

	payload := events.NewAssetUpdated(a)

	require.NotNil(t, payload.Status.Description)
	assert.Equal(t, statusDesc, *payload.Status.Description)
}

func TestAssetUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAssetUpdated(minimalAsset())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.AssetUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.AssetUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestAssetUpdatedPayload_JSONShape(t *testing.T) {
	payload := events.NewAssetUpdated(minimalAsset())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "name", "type", "code", "status", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	status, ok := generic["status"].(map[string]any)
	require.True(t, ok, "status must serialize as an object")
	_, hasStatusDesc := status["description"]
	assert.False(t, hasStatusDesc, "status.description must omitempty when nil")

	assert.Lenf(t, generic, 8, "expected 8 top-level fields, got %d (drift?)", len(generic))
}
