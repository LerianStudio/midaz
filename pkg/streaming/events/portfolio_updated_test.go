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

func TestPortfolioUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "portfolio.updated", events.PortfolioUpdatedDefinition.Key())
	assert.Equal(t, "portfolio", events.PortfolioUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.PortfolioUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.PortfolioUpdatedDefinition.SchemaVersion)
}

func TestNewPortfolioUpdated_MapsMinimalPortfolio(t *testing.T) {
	p := minimalPortfolio()

	payload := events.NewPortfolioUpdated(p)

	assert.Equal(t, p.ID, payload.ID)
	assert.Equal(t, p.OrganizationID, payload.OrganizationID)
	assert.Equal(t, p.LedgerID, payload.LedgerID)
	assert.Equal(t, p.Name, payload.Name)
	assert.Empty(t, payload.EntityID)
	assert.Equal(t, "ACTIVE", payload.Status.Code)
	assert.Nil(t, payload.Status.Description)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewPortfolioUpdated_MapsAllOptionalFields(t *testing.T) {
	statusDesc := "Active portfolio"

	p := minimalPortfolio()
	p.EntityID = "ext-entity-42"
	p.Status.Description = &statusDesc

	payload := events.NewPortfolioUpdated(p)

	assert.Equal(t, "ext-entity-42", payload.EntityID)
	require.NotNil(t, payload.Status.Description)
	assert.Equal(t, statusDesc, *payload.Status.Description)
}

func TestPortfolioUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewPortfolioUpdated(minimalPortfolio())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.PortfolioUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.PortfolioUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestPortfolioUpdatedPayload_JSONShape_MinimalOmitsEntityID(t *testing.T) {
	payload := events.NewPortfolioUpdated(minimalPortfolio())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "name", "status", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasEntityID := generic["entityId"]
	assert.False(t, hasEntityID, "entityId must omitempty when empty")

	_, hasCreatedAt := generic["createdAt"]
	assert.False(t, hasCreatedAt, "createdAt must NOT appear on portfolio.updated")

	status, ok := generic["status"].(map[string]any)
	require.True(t, ok, "status must serialize as an object")
	_, hasStatusDesc := status["description"]
	assert.False(t, hasStatusDesc, "status.description must omitempty when nil")

	assert.Lenf(t, generic, 6, "expected 6 top-level fields when entityId omitted, got %d (drift?)", len(generic))
}
