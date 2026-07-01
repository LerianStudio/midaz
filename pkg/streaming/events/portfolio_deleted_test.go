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

func TestPortfolioDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "portfolio.deleted", events.PortfolioDeletedDefinition.Key())
	assert.Equal(t, "portfolio", events.PortfolioDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.PortfolioDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.PortfolioDeletedDefinition.SchemaVersion)
}

func TestNewPortfolioDeleted_MapsIdentity(t *testing.T) {
	payload := events.NewPortfolioDeleted(
		"01J7K8FN5W8R0R2S7Q1V4H6P00",
		"01J7K8FN5W8R0R2S7Q1V4H6J01",
		"01J7K8FN5W8R0R2S7Q1V4H6J02",
		fixedTime,
	)

	assert.Equal(t, "01J7K8FN5W8R0R2S7Q1V4H6P00", payload.ID)
	assert.Equal(t, "01J7K8FN5W8R0R2S7Q1V4H6J01", payload.OrganizationID)
	assert.Equal(t, "01J7K8FN5W8R0R2S7Q1V4H6J02", payload.LedgerID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestPortfolioDeletedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewPortfolioDeleted("port-123", "org-456", "led-789", fixedTime)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.PortfolioDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.PortfolioDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestPortfolioDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewPortfolioDeleted("port-123", "org-456", "led-789", fixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "deletedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	assert.Lenf(t, generic, 4, "expected 4 top-level fields, got %d (drift?)", len(generic))
}
