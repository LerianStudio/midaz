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

func TestLimitDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "limit.deleted", events.LimitDeletedDefinition.Key())
	assert.Equal(t, "limit", events.LimitDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.LimitDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.LimitDeletedDefinition.SchemaVersion)
}

func TestNewLimitDeleted_MapsDomain(t *testing.T) {
	limit := minimalLimit()
	deletedAt := fixedTime
	limit.Status = model.LimitStatusDeleted
	limit.DeletedAt = &deletedAt

	payload := events.NewLimitDeleted(limit)

	assert.Equal(t, fixedLimitUUID.String(), payload.ID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestNewLimitDeleted_NilDeletedAtDoesNotPanic(t *testing.T) {
	limit := minimalLimit()
	limit.DeletedAt = nil

	payload := events.NewLimitDeleted(limit)

	assert.Equal(t, fixedLimitUUID.String(), payload.ID)
	assert.Equal(t, "", payload.DeletedAt)
}

func TestLimitDeletedPayload_ToEmitRequest(t *testing.T) {
	limit := minimalLimit()
	deletedAt := fixedTime
	limit.DeletedAt = &deletedAt

	payload := events.NewLimitDeleted(limit)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.LimitDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.LimitDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestLimitDeletedPayload_JSONShape(t *testing.T) {
	limit := minimalLimit()
	deletedAt := fixedTime
	limit.DeletedAt = &deletedAt

	data, err := json.Marshal(events.NewLimitDeleted(limit))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":        {},
		"deletedAt": {},
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
		"status", "name", "description", "maxAmount", "limitType", "currency", "scopes",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "field %q must NOT appear", forbidden)
	}

	assert.Lenf(t, generic, 2, "expected 2 top-level fields, got %d (drift?)", len(generic))
}
