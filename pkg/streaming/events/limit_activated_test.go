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

func TestLimitActivatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "limit.activated", events.LimitActivatedDefinition.Key())
	assert.Equal(t, "limit", events.LimitActivatedDefinition.ResourceType)
	assert.Equal(t, "activated", events.LimitActivatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.LimitActivatedDefinition.SchemaVersion)
}

func TestNewLimitActivated_MapsLimit(t *testing.T) {
	limit := minimalLimit()
	limit.Status = model.LimitStatusActive

	payload := events.NewLimitActivated(limit)

	assert.Equal(t, fixedLimitUUID.String(), payload.ID)
	assert.Equal(t, "ACTIVE", payload.Status)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestLimitActivatedPayload_ToEmitRequest(t *testing.T) {
	limit := minimalLimit()
	limit.Status = model.LimitStatusActive

	payload := events.NewLimitActivated(limit)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.LimitActivatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.LimitActivatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestLimitActivatedPayload_JSONShape(t *testing.T) {
	limit := minimalLimit()
	limit.Status = model.LimitStatusActive

	data, err := json.Marshal(events.NewLimitActivated(limit))
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
