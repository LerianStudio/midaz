// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

func TestLimitUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "limit.updated", events.LimitUpdatedDefinition.Key())
	assert.Equal(t, "limit", events.LimitUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.LimitUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.LimitUpdatedDefinition.SchemaVersion)
}

func TestNewLimitUpdated_MapsMinimalLimit(t *testing.T) {
	payload := events.NewLimitUpdated(minimalLimit())

	assert.Equal(t, fixedLimitUUID.String(), payload.ID)
	assert.Equal(t, "DRAFT", payload.Status)
	assert.Equal(t, "DAILY", payload.LimitType)
	assert.Equal(t, "USD", payload.Currency)
	require.NotNil(t, payload.Scopes)
	assert.Len(t, payload.Scopes, 0)
	assert.Nil(t, payload.ActiveTimeStart)
	assert.Nil(t, payload.ResetAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewLimitUpdated_MapsAllOptionalFields(t *testing.T) {
	payload := events.NewLimitUpdated(fullLimit(t))

	assert.Equal(t, "ACTIVE", payload.Status)
	assert.Equal(t, "CUSTOM", payload.LimitType)
	require.NotNil(t, payload.ActiveTimeStart)
	assert.Equal(t, "09:00", *payload.ActiveTimeStart)
	require.NotNil(t, payload.ResetAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", *payload.ResetAt)
	require.Len(t, payload.Scopes, 1)
}

func TestLimitUpdatedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewLimitUpdated(fullLimit(t))

	req, err := payload.ToEmitRequest("tenant-2", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.LimitUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-2", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.LimitUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestLimitUpdatedPayload_JSONShape(t *testing.T) {
	data, err := json.Marshal(events.NewLimitUpdated(fullLimit(t)))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":              {},
		"status":          {},
		"limitType":       {},
		"currency":        {},
		"scopes":          {},
		"activeTimeStart": {},
		"activeTimeEnd":   {},
		"customStartDate": {},
		"customEndDate":   {},
		"resetAt":         {},
		"createdAt":       {},
		"updatedAt":       {},
	}

	for key := range generic {
		_, ok := expectedKeys[key]
		assert.Truef(t, ok, "wire payload has unexpected top-level key %q (drift?)", key)
	}

	for key := range expectedKeys {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{"name", "description", "maxAmount"} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "fenced field %q must NOT appear on the wire", forbidden)
	}

	assert.Lenf(t, generic, 12, "expected 12 top-level fields, got %d (drift?)", len(generic))
}
