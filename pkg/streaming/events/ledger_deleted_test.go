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

func TestLedgerDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "ledger.deleted", events.LedgerDeletedDefinition.Key())
	assert.Equal(t, "ledger", events.LedgerDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.LedgerDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.LedgerDeletedDefinition.SchemaVersion)
}

func TestNewLedgerDeleted_MapsMinimalLedger(t *testing.T) {
	payload := events.NewLedgerDeleted("01J7K8FN5W8R0R2S7Q1V4H6J0M", "01J7K8FN5W8R0R2S7Q1V4H6J01", fixedTime)

	assert.Equal(t, "01J7K8FN5W8R0R2S7Q1V4H6J0M", payload.ID)
	assert.Equal(t, "01J7K8FN5W8R0R2S7Q1V4H6J01", payload.OrganizationID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestLedgerDeletedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewLedgerDeleted("led-123", "org-456", fixedTime)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.LedgerDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.LedgerDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestLedgerDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewLedgerDeleted("led-123", "org-456", fixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "deletedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	assert.Lenf(t, generic, 3, "expected 3 top-level fields, got %d (drift?)", len(generic))
}
