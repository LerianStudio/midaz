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

func TestRuleActivatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "rule.activated", events.RuleActivatedDefinition.Key())
	assert.Equal(t, "rule", events.RuleActivatedDefinition.ResourceType)
	assert.Equal(t, "activated", events.RuleActivatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.RuleActivatedDefinition.SchemaVersion)
}

func TestNewRuleActivated_MapsWithTimestamp(t *testing.T) {
	activatedAt := fixedTime
	rule := minimalRule()
	rule.Status = model.RuleStatusActive
	rule.ActivatedAt = &activatedAt

	payload := events.NewRuleActivated(rule)

	assert.Equal(t, fixedRuleUUID.String(), payload.ID)
	assert.Equal(t, "ACTIVE", payload.Status)
	require.NotNil(t, payload.ActivatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", *payload.ActivatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewRuleActivated_NilTimestamp(t *testing.T) {
	rule := minimalRule()
	rule.Status = model.RuleStatusActive
	rule.ActivatedAt = nil

	payload := events.NewRuleActivated(rule)

	assert.Nil(t, payload.ActivatedAt)
}

func TestRuleActivatedPayload_ToEmitRequest(t *testing.T) {
	activatedAt := fixedTime
	rule := minimalRule()
	rule.ActivatedAt = &activatedAt

	payload := events.NewRuleActivated(rule)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.RuleActivatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.RuleActivatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestRuleActivatedPayload_JSONShape(t *testing.T) {
	// Nil timestamp path: key must still be present with value null.
	rule := minimalRule()
	rule.ActivatedAt = nil

	data, err := json.Marshal(events.NewRuleActivated(rule))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":          {},
		"status":      {},
		"activatedAt": {},
		"updatedAt":   {},
	}

	for key := range generic {
		_, ok := expectedKeys[key]
		assert.Truef(t, ok, "unexpected top-level key %q (drift?)", key)
	}

	for key := range expectedKeys {
		_, ok := generic[key]
		assert.Truef(t, ok, "must include %q", key)
	}

	// activatedAt key present even when nil (NOT omitempty), value null.
	val, present := generic["activatedAt"]
	require.True(t, present)
	assert.Nil(t, val, "activatedAt must serialize null when unset")

	for _, forbidden := range []string{"name", "description", "expression", "action", "scopes"} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "fenced/absent field %q must NOT appear", forbidden)
	}

	assert.Lenf(t, generic, 4, "expected 4 top-level fields, got %d (drift?)", len(generic))
}
