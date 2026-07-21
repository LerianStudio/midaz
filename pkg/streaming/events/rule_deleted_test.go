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

func TestRuleDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "rule.deleted", events.RuleDeletedDefinition.Key())
	assert.Equal(t, "rule", events.RuleDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.RuleDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.RuleDeletedDefinition.SchemaVersion)
}

func TestNewRuleDeleted_MapsPrimitives(t *testing.T) {
	payload := events.NewRuleDeleted(fixedRuleUUID, fixedTime)

	assert.Equal(t, fixedRuleUUID.String(), payload.ID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestRuleDeletedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewRuleDeleted(fixedRuleUUID, fixedTime)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.RuleDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.RuleDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestRuleDeletedPayload_JSONShape(t *testing.T) {
	data, err := json.Marshal(events.NewRuleDeleted(fixedRuleUUID, fixedTime))
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

	for _, forbidden := range []string{"status", "name", "description", "expression", "scopes", "action"} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "field %q must NOT appear", forbidden)
	}

	assert.Lenf(t, generic, 2, "expected 2 top-level fields, got %d (drift?)", len(generic))
}
