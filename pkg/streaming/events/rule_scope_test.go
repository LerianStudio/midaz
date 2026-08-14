// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// fixedRuleUUID is a deterministic UUID used across rule event tests so
// subject/payload assertions match by exact value.
var fixedRuleUUID = uuid.MustParse("018f5e2a-1c3d-7a4b-9e6f-0a1b2c3d4e5f")

// ruleScopeIDs are deterministic UUIDs used to populate scope fields.
var (
	scopeSegmentID   = uuid.MustParse("111e5e2a-1c3d-7a4b-9e6f-0a1b2c3d4e01")
	scopePortfolioID = uuid.MustParse("222e5e2a-1c3d-7a4b-9e6f-0a1b2c3d4e02")
	scopeAccountID   = uuid.MustParse("333e5e2a-1c3d-7a4b-9e6f-0a1b2c3d4e03")
	scopeMerchantID  = uuid.MustParse("444e5e2a-1c3d-7a4b-9e6f-0a1b2c3d4e04")
)

// TestNewRuleScopePayloads_EmptySliceIsNonNil proves the mapper returns a
// non-nil empty slice for an empty input so the wire always serializes
// "scopes": [] and never null.
func TestNewRuleScopePayloads_EmptySliceIsNonNil(t *testing.T) {
	rule := &model.Rule{
		ID:     fixedRuleUUID,
		Status: model.RuleStatusDraft,
		Action: model.DecisionDeny,
		Scopes: nil,
	}

	payload := events.NewRuleCreated(rule)

	require.NotNil(t, payload.Scopes)
	assert.Len(t, payload.Scopes, 0)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	scopes, ok := generic["scopes"]
	require.True(t, ok, "scopes key must be present")
	assert.NotNil(t, scopes, "scopes must serialize as [] not null")
}

// TestNewRuleScopePayloads_PopulatedScope verifies each domain pointer maps
// to its string form when non-nil, and unset fields stay nil (JSON null).
func TestNewRuleScopePayloads_PopulatedScope(t *testing.T) {
	txType := model.TransactionTypeCard
	subType := "purchase"

	rule := &model.Rule{
		ID:     fixedRuleUUID,
		Status: model.RuleStatusActive,
		Action: model.DecisionAllow,
		Scopes: []model.Scope{
			{
				SegmentID:       &scopeSegmentID,
				TransactionType: &txType,
				SubType:         &subType,
			},
		},
	}

	payload := events.NewRuleCreated(rule)

	require.Len(t, payload.Scopes, 1)
	sc := payload.Scopes[0]

	require.NotNil(t, sc.SegmentID)
	assert.Equal(t, scopeSegmentID.String(), *sc.SegmentID)

	require.NotNil(t, sc.TransactionType)
	assert.Equal(t, "CARD", *sc.TransactionType)

	require.NotNil(t, sc.SubType)
	assert.Equal(t, "purchase", *sc.SubType)

	// Unset fields remain nil.
	assert.Nil(t, sc.PortfolioID)
	assert.Nil(t, sc.AccountID)
	assert.Nil(t, sc.MerchantID)
}

// TestNewRuleScopePayloads_AllFieldsSet verifies a scope with every field set
// maps each pointer without panic.
func TestNewRuleScopePayloads_AllFieldsSet(t *testing.T) {
	txType := model.TransactionTypePix
	subType := "transfer"

	rule := &model.Rule{
		ID:     fixedRuleUUID,
		Status: model.RuleStatusActive,
		Action: model.DecisionReview,
		Scopes: []model.Scope{
			{
				SegmentID:       &scopeSegmentID,
				PortfolioID:     &scopePortfolioID,
				AccountID:       &scopeAccountID,
				MerchantID:      &scopeMerchantID,
				TransactionType: &txType,
				SubType:         &subType,
			},
		},
	}

	payload := events.NewRuleCreated(rule)

	require.Len(t, payload.Scopes, 1)
	sc := payload.Scopes[0]

	require.NotNil(t, sc.SegmentID)
	assert.Equal(t, scopeSegmentID.String(), *sc.SegmentID)
	require.NotNil(t, sc.PortfolioID)
	assert.Equal(t, scopePortfolioID.String(), *sc.PortfolioID)
	require.NotNil(t, sc.AccountID)
	assert.Equal(t, scopeAccountID.String(), *sc.AccountID)
	require.NotNil(t, sc.MerchantID)
	assert.Equal(t, scopeMerchantID.String(), *sc.MerchantID)
	require.NotNil(t, sc.TransactionType)
	assert.Equal(t, "PIX", *sc.TransactionType)
	require.NotNil(t, sc.SubType)
	assert.Equal(t, "transfer", *sc.SubType)
}

// TestRuleScopePayload_JSONShape locks the nested scope object to exactly six
// keys and asserts no rule free-text leaks into a scope.
func TestRuleScopePayload_JSONShape(t *testing.T) {
	txType := model.TransactionTypeWire
	subType := "b2b"

	rule := &model.Rule{
		ID:     fixedRuleUUID,
		Status: model.RuleStatusActive,
		Action: model.DecisionAllow,
		Scopes: []model.Scope{
			{
				SegmentID:       &scopeSegmentID,
				PortfolioID:     &scopePortfolioID,
				AccountID:       &scopeAccountID,
				MerchantID:      &scopeMerchantID,
				TransactionType: &txType,
				SubType:         &subType,
			},
		},
	}

	data, err := json.Marshal(events.NewRuleCreated(rule))
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

	assert.Lenf(t, scope, 6, "expected 6 scope keys, got %d (drift?)", len(scope))
}
