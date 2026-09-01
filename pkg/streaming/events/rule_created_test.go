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

// ruleFenceDescription/Expression/Name/CompiledProgram are deliberately
// populated on fixtures to prove the fence keeps them off the wire.
const (
	fenceName       = "Block high-value checking transactions"
	fenceExpression = "transaction.amount > 1000 && account.type == 'checking'"
)

// minimalRule returns the smallest Rule that satisfies the rule.created
// contract, with the fenced free-text fields populated on purpose so tests
// prove none of them leak onto the wire.
func minimalRule() *model.Rule {
	desc := "Denies transactions over $1000"

	return &model.Rule{
		ID:              fixedRuleUUID,
		Name:            fenceName,
		Description:     &desc,
		Expression:      fenceExpression,
		Action:          model.DecisionDeny,
		Scopes:          nil,
		Status:          model.RuleStatusDraft,
		CreatedAt:       fixedTime,
		UpdatedAt:       fixedTime,
		CompiledProgram: "COMPILED",
	}
}

func TestRuleCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "rule.created", events.RuleCreatedDefinition.Key())
	assert.Equal(t, "rule", events.RuleCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.RuleCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.RuleCreatedDefinition.SchemaVersion)
}

func TestNewRuleCreated_MapsMinimalRule(t *testing.T) {
	payload := events.NewRuleCreated(minimalRule())

	assert.Equal(t, fixedRuleUUID.String(), payload.ID)
	assert.Equal(t, "DRAFT", payload.Status)
	assert.Equal(t, "DENY", payload.Action)
	require.NotNil(t, payload.Scopes)
	assert.Len(t, payload.Scopes, 0)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewRuleCreated_MapsScopes(t *testing.T) {
	txType := model.TransactionTypeCard
	rule := minimalRule()
	rule.Scopes = []model.Scope{{SegmentID: &scopeSegmentID, TransactionType: &txType}}

	payload := events.NewRuleCreated(rule)

	require.Len(t, payload.Scopes, 1)
	require.NotNil(t, payload.Scopes[0].SegmentID)
	assert.Equal(t, scopeSegmentID.String(), *payload.Scopes[0].SegmentID)
}

func TestRuleCreatedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewRuleCreated(minimalRule())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.RuleCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.RuleCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestRuleCreatedPayload_JSONShape(t *testing.T) {
	txType := model.TransactionTypeCard
	rule := minimalRule()
	rule.Scopes = []model.Scope{{AccountID: &scopeAccountID, TransactionType: &txType}}

	data, err := json.Marshal(events.NewRuleCreated(rule))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":        {},
		"status":    {},
		"action":    {},
		"scopes":    {},
		"createdAt": {},
		"updatedAt": {},
	}

	for key := range generic {
		_, ok := expectedKeys[key]
		assert.Truef(t, ok, "wire payload has unexpected top-level key %q (drift?)", key)
	}

	for key := range expectedKeys {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	// Fence: free text and rule logic MUST NOT appear.
	for _, forbidden := range []string{"name", "description", "expression", "compiledProgram"} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "fenced field %q must NOT appear on the wire", forbidden)
	}

	// Nested scope has exactly the six structural keys, no free text.
	scopesRaw, ok := generic["scopes"].([]any)
	require.True(t, ok)
	require.Len(t, scopesRaw, 1)
	scope, ok := scopesRaw[0].(map[string]any)
	require.True(t, ok, "scope must serialize as an object")
	assert.Lenf(t, scope, 6, "nested scope must have 6 keys, got %d", len(scope))
	for _, forbidden := range []string{"name", "description", "expression"} {
		_, present := scope[forbidden]
		assert.Falsef(t, present, "scope must not carry %q", forbidden)
	}

	assert.Lenf(t, generic, 6, "expected 6 top-level fields, got %d (drift?)", len(generic))
}
