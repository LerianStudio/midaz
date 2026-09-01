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

func minimalPortfolio() *mmodel.Portfolio {
	return &mmodel.Portfolio{
		ID:             "01J7K8FN5W8R0R2S7Q1V4H6P00",
		OrganizationID: "01J7K8FN5W8R0R2S7Q1V4H6J01",
		LedgerID:       "01J7K8FN5W8R0R2S7Q1V4H6J02",
		Name:           "Investment Portfolio",
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

func TestPortfolioCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "portfolio.created", events.PortfolioCreatedDefinition.Key())
	assert.Equal(t, "portfolio", events.PortfolioCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.PortfolioCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.PortfolioCreatedDefinition.SchemaVersion)
}

func TestNewPortfolioCreated_MapsMinimalPortfolio(t *testing.T) {
	p := minimalPortfolio()

	payload := events.NewPortfolioCreated(p)

	assert.Equal(t, p.ID, payload.ID)
	assert.Equal(t, p.OrganizationID, payload.OrganizationID)
	assert.Equal(t, p.LedgerID, payload.LedgerID)
	assert.Equal(t, p.Name, payload.Name)
	assert.Empty(t, payload.EntityID)
	assert.Equal(t, "ACTIVE", payload.Status.Code)
	assert.Nil(t, payload.Status.Description)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewPortfolioCreated_MapsAllOptionalFields(t *testing.T) {
	statusDesc := "Active portfolio"

	p := minimalPortfolio()
	p.EntityID = "ext-entity-42"
	p.Status.Description = &statusDesc

	payload := events.NewPortfolioCreated(p)

	assert.Equal(t, "ext-entity-42", payload.EntityID)
	require.NotNil(t, payload.Status.Description)
	assert.Equal(t, statusDesc, *payload.Status.Description)
}

func TestPortfolioCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewPortfolioCreated(minimalPortfolio())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.PortfolioCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.PortfolioCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestPortfolioCreatedPayload_JSONShape_MinimalOmitsEntityID(t *testing.T) {
	payload := events.NewPortfolioCreated(minimalPortfolio())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "name", "status", "createdAt", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasEntityID := generic["entityId"]
	assert.False(t, hasEntityID, "entityId must omitempty when empty")

	status, ok := generic["status"].(map[string]any)
	require.True(t, ok, "status must serialize as an object")
	_, hasStatusDesc := status["description"]
	assert.False(t, hasStatusDesc, "status.description must omitempty when nil")

	assert.Lenf(t, generic, 7, "expected 7 top-level fields when entityId omitted, got %d (drift?)", len(generic))
}

func TestPortfolioCreatedPayload_JSONShape_WithEntityIDIncludesIt(t *testing.T) {
	p := minimalPortfolio()
	p.EntityID = "ext-entity-7"

	payload := events.NewPortfolioCreated(p)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	entityID, ok := generic["entityId"]
	require.True(t, ok, "entityId must be present when non-empty")
	assert.Equal(t, "ext-entity-7", entityID)

	assert.Lenf(t, generic, 8, "expected 8 top-level fields when entityId present, got %d (drift?)", len(generic))
}
