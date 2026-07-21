// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

func TestLimitDeactivatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "limit.deactivated", events.LimitDeactivatedDefinition.Key())
	assert.Equal(t, "limit", events.LimitDeactivatedDefinition.ResourceType)
	assert.Equal(t, "deactivated", events.LimitDeactivatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.LimitDeactivatedDefinition.SchemaVersion)
}

func TestNewLimitDeactivated_MapsLimit(t *testing.T) {
	limit := minimalLimit()
	limit.Status = model.LimitStatusInactive

	payload := events.NewLimitDeactivated(limit)

	assert.Equal(t, fixedLimitUUID.String(), payload.ID)
	assert.Equal(t, "INACTIVE", payload.Status)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestLimitDeactivatedPayload_ToEmitRequest(t *testing.T) {
	limit := minimalLimit()
	limit.Status = model.LimitStatusInactive

	payload := events.NewLimitDeactivated(limit)

	req, err := payload.ToEmitRequest("tenant-3", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.LimitDeactivatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-3", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.LimitDeactivatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestLimitDeactivatedPayload_JSONShape(t *testing.T) {
	limit := minimalLimit()
	limit.Status = model.LimitStatusInactive

	data, err := json.Marshal(events.NewLimitDeactivated(limit))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":        {},
		"status":    {},
		"updatedAt": {},
	}

	for key := range generic {
		_, ok := expectedKeys[key]
		assert.Truef(t, ok, "unexpected top-level key %q (drift?)", key)
	}

	for key := range expectedKeys {
		_, ok := generic[key]
		assert.Truef(t, ok, "must include %q", key)
	}

	for _, forbidden := range []string{
		"activatedAt", "deactivatedAt", "limitType", "currency", "scopes", "maxAmount", "name", "description",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "field %q must NOT appear", forbidden)
	}

	assert.Lenf(t, generic, 3, "expected 3 top-level fields, got %d (drift?)", len(generic))
}
