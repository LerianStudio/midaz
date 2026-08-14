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

func TestRuleDraftedDefinition_Key(t *testing.T) {
	assert.Equal(t, "rule.drafted", events.RuleDraftedDefinition.Key())
	assert.Equal(t, "rule", events.RuleDraftedDefinition.ResourceType)
	assert.Equal(t, "drafted", events.RuleDraftedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.RuleDraftedDefinition.SchemaVersion)
}

func TestNewRuleDrafted_MapsRule(t *testing.T) {
	rule := minimalRule()
	rule.Status = model.RuleStatusDraft

	payload := events.NewRuleDrafted(rule)

	assert.Equal(t, fixedRuleUUID.String(), payload.ID)
	assert.Equal(t, "DRAFT", payload.Status)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestRuleDraftedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewRuleDrafted(minimalRule())

	req, err := payload.ToEmitRequest("tenant-x", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.RuleDraftedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-x", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.RuleDraftedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestRuleDraftedPayload_JSONShape(t *testing.T) {
	data, err := json.Marshal(events.NewRuleDrafted(minimalRule()))
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

	for _, forbidden := range []string{"name", "description", "expression", "action", "scopes", "activatedAt", "deactivatedAt"} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "fenced/absent field %q must NOT appear", forbidden)
	}

	assert.Lenf(t, generic, 3, "expected 3 top-level fields, got %d (drift?)", len(generic))
}
