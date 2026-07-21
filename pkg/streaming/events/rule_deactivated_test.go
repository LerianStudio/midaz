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

func TestRuleDeactivatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "rule.deactivated", events.RuleDeactivatedDefinition.Key())
	assert.Equal(t, "rule", events.RuleDeactivatedDefinition.ResourceType)
	assert.Equal(t, "deactivated", events.RuleDeactivatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.RuleDeactivatedDefinition.SchemaVersion)
}

func TestNewRuleDeactivated_MapsWithTimestamp(t *testing.T) {
	deactivatedAt := fixedTime
	rule := minimalRule()
	rule.Status = model.RuleStatusInactive
	rule.DeactivatedAt = &deactivatedAt

	payload := events.NewRuleDeactivated(rule)

	assert.Equal(t, fixedRuleUUID.String(), payload.ID)
	assert.Equal(t, "INACTIVE", payload.Status)
	require.NotNil(t, payload.DeactivatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", *payload.DeactivatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewRuleDeactivated_NilTimestamp(t *testing.T) {
	rule := minimalRule()
	rule.Status = model.RuleStatusInactive
	rule.DeactivatedAt = nil

	payload := events.NewRuleDeactivated(rule)

	assert.Nil(t, payload.DeactivatedAt)
}

func TestRuleDeactivatedPayload_ToEmitRequest(t *testing.T) {
	deactivatedAt := fixedTime
	rule := minimalRule()
	rule.DeactivatedAt = &deactivatedAt

	payload := events.NewRuleDeactivated(rule)

	req, err := payload.ToEmitRequest("tenant-9", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.RuleDeactivatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-9", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.RuleDeactivatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestRuleDeactivatedPayload_JSONShape(t *testing.T) {
	rule := minimalRule()
	rule.DeactivatedAt = nil

	data, err := json.Marshal(events.NewRuleDeactivated(rule))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":            {},
		"status":        {},
		"deactivatedAt": {},
		"updatedAt":     {},
	}

	for key := range generic {
		_, ok := expectedKeys[key]
		assert.Truef(t, ok, "unexpected top-level key %q (drift?)", key)
	}

	for key := range expectedKeys {
		_, ok := generic[key]
		assert.Truef(t, ok, "must include %q", key)
	}

	val, present := generic["deactivatedAt"]
	require.True(t, present)
	assert.Nil(t, val, "deactivatedAt must serialize null when unset")

	for _, forbidden := range []string{"name", "description", "expression", "action", "scopes", "activatedAt"} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "fenced/absent field %q must NOT appear", forbidden)
	}

	assert.Lenf(t, generic, 4, "expected 4 top-level fields, got %d (drift?)", len(generic))
}
