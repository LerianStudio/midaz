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

func TestSegmentUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "segment.updated", events.SegmentUpdatedDefinition.Key())
	assert.Equal(t, "segment", events.SegmentUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.SegmentUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.SegmentUpdatedDefinition.SchemaVersion)
}

func TestNewSegmentUpdated_MapsMinimalSegment(t *testing.T) {
	s := minimalSegment()

	payload := events.NewSegmentUpdated(s)

	assert.Equal(t, s.ID, payload.ID)
	assert.Equal(t, s.OrganizationID, payload.OrganizationID)
	assert.Equal(t, s.LedgerID, payload.LedgerID)
	assert.Equal(t, s.Name, payload.Name)
	assert.Equal(t, "ACTIVE", payload.Status.Code)
	assert.Nil(t, payload.Status.Description)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewSegmentUpdated_MapsStatusDescription(t *testing.T) {
	statusDesc := "Active segment"

	s := minimalSegment()
	s.Status.Description = &statusDesc

	payload := events.NewSegmentUpdated(s)

	require.NotNil(t, payload.Status.Description)
	assert.Equal(t, statusDesc, *payload.Status.Description)
}

func TestSegmentUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewSegmentUpdated(minimalSegment())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.SegmentUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.SegmentUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestSegmentUpdatedPayload_JSONShape(t *testing.T) {
	payload := events.NewSegmentUpdated(minimalSegment())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "name", "status", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasCreatedAt := generic["createdAt"]
	assert.False(t, hasCreatedAt, "createdAt must NOT appear on segment.updated")

	status, ok := generic["status"].(map[string]any)
	require.True(t, ok, "status must serialize as an object")
	_, hasStatusDesc := status["description"]
	assert.False(t, hasStatusDesc, "status.description must omitempty when nil")

	assert.Lenf(t, generic, 6, "expected 6 top-level fields, got %d (drift?)", len(generic))
}
