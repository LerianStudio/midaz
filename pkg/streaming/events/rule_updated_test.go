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

func TestRuleUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "rule.updated", events.RuleUpdatedDefinition.Key())
	assert.Equal(t, "rule", events.RuleUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.RuleUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.RuleUpdatedDefinition.SchemaVersion)
}

func TestNewRuleUpdated_MapsMinimalRule(t *testing.T) {
	rule := minimalRule()
	rule.Status = model.RuleStatusActive
	rule.Action = model.DecisionAllow

	payload := events.NewRuleUpdated(rule)

	assert.Equal(t, fixedRuleUUID.String(), payload.ID)
	assert.Equal(t, "ACTIVE", payload.Status)
	assert.Equal(t, "ALLOW", payload.Action)
	require.NotNil(t, payload.Scopes)
	assert.Len(t, payload.Scopes, 0)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewRuleUpdated_MapsScopes(t *testing.T) {
	subType := "purchase"
	rule := minimalRule()
	rule.Scopes = []model.Scope{{MerchantID: &scopeMerchantID, SubType: &subType}}

	payload := events.NewRuleUpdated(rule)

	require.Len(t, payload.Scopes, 1)
	require.NotNil(t, payload.Scopes[0].MerchantID)
	assert.Equal(t, scopeMerchantID.String(), *payload.Scopes[0].MerchantID)
	require.NotNil(t, payload.Scopes[0].SubType)
	assert.Equal(t, "purchase", *payload.Scopes[0].SubType)
}

func TestRuleUpdatedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewRuleUpdated(minimalRule())

	req, err := payload.ToEmitRequest("tenant-2", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.RuleUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-2", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.RuleUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestRuleUpdatedPayload_JSONShape(t *testing.T) {
	data, err := json.Marshal(events.NewRuleUpdated(minimalRule()))
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

	for _, forbidden := range []string{"name", "description", "expression", "compiledProgram"} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "fenced field %q must NOT appear on the wire", forbidden)
	}

	assert.Lenf(t, generic, 6, "expected 6 top-level fields, got %d (drift?)", len(generic))
}

// TestRuleUpdatedPayload_JSONShape_PopulatedScope locks the nested scope
// object on the rule.updated path to exactly the six structural keys and
// asserts no rule free-text leaks into a scope (parity with rule.created).
func TestRuleUpdatedPayload_JSONShape_PopulatedScope(t *testing.T) {
	txType := model.TransactionTypePix
	subType := "transfer"

	rule := minimalRule()
	rule.Scopes = []model.Scope{
		{
			SegmentID:       &scopeSegmentID,
			PortfolioID:     &scopePortfolioID,
			AccountID:       &scopeAccountID,
			MerchantID:      &scopeMerchantID,
			TransactionType: &txType,
			SubType:         &subType,
		},
	}

	data, err := json.Marshal(events.NewRuleUpdated(rule))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	scopesRaw, ok := generic["scopes"].([]any)
	require.True(t, ok, "scopes must serialize as an array")
	require.Len(t, scopesRaw, 1)

	scope, ok := scopesRaw[0].(map[string]any)
	require.True(t, ok, "scope must serialize as an object")

	expectedKeys := map[string]struct{}{
		"segmentId":       {},
		"portfolioId":     {},
		"accountId":       {},
		"merchantId":      {},
		"transactionType": {},
		"subType":         {},
	}

	for key := range scope {
		_, allowed := expectedKeys[key]
		assert.Truef(t, allowed, "scope has unexpected key %q (drift?)", key)
	}

	for key := range expectedKeys {
		_, present := scope[key]
		assert.Truef(t, present, "scope must include %q", key)
	}

	for _, forbidden := range []string{"name", "description", "expression"} {
		_, present := scope[forbidden]
		assert.Falsef(t, present, "scope must not carry %q", forbidden)
	}

	assert.Lenf(t, scope, 6, "expected 6 scope keys, got %d (drift?)", len(scope))
}
