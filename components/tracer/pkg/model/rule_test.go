// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"testing"
	"time"

	"tracer/internal/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleStatus_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		status   RuleStatus
		expected bool
	}{
		{
			name:     "Success - DRAFT is valid",
			status:   RuleStatusDraft,
			expected: true,
		},
		{
			name:     "Success - ACTIVE is valid",
			status:   RuleStatusActive,
			expected: true,
		},
		{
			name:     "Success - INACTIVE is valid",
			status:   RuleStatusInactive,
			expected: true,
		},
		{
			name:     "Success - DELETED is valid",
			status:   RuleStatusDeleted,
			expected: true,
		},
		{
			name:     "Error - empty string is invalid",
			status:   RuleStatus(""),
			expected: false,
		},
		{
			name:     "Error - lowercase draft is invalid",
			status:   RuleStatus("draft"),
			expected: false,
		},
		{
			name:     "Error - random string is invalid",
			status:   RuleStatus("INVALID"),
			expected: false,
		},
		{
			name:     "Error - partial match is invalid",
			status:   RuleStatus("DRAF"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.status.IsValid()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestRule_JSONSerialization(t *testing.T) {
	t.Run("Success - rule serializes to JSON correctly", func(t *testing.T) {
		ruleID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
		description := "Test rule description"
		createdAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		updatedAt := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

		rule := Rule{
			ID:          ruleID,
			Name:        "test rule",
			Description: &description,
			Expression:  "amount > 1000",
			Action:      DecisionDeny,
			Scopes:      []Scope{},
			Status:      RuleStatusDraft,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
			DeletedAt:   nil,
		}

		data, err := json.Marshal(rule)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		assert.Equal(t, ruleID.String(), result["ruleId"])
		assert.Equal(t, "test rule", result["name"])
		assert.Equal(t, description, result["description"])
		assert.Equal(t, "amount > 1000", result["expression"])
		assert.Equal(t, "DENY", result["action"])
		assert.Equal(t, "DRAFT", result["status"])
		assert.NotNil(t, result["scopes"])
		assert.NotNil(t, result["createdAt"])
		assert.NotNil(t, result["updatedAt"])
	})

	t.Run("Success - rule without description serializes correctly", func(t *testing.T) {
		rule := Rule{
			ID:          testutil.MustDeterministicUUID(1),
			Name:        "test rule",
			Description: nil,
			Expression:  "amount > 1000",
			Action:      DecisionAllow,
			Scopes:      []Scope{},
			Status:      RuleStatusActive,
			CreatedAt:   testutil.FixedTime(),
			UpdatedAt:   testutil.FixedTime(),
		}

		data, err := json.Marshal(rule)
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		_, hasDescription := result["description"]
		assert.False(t, hasDescription, "description should be omitted when nil")
	})

	t.Run("Success - rule deserializes from JSON correctly", func(t *testing.T) {
		jsonData := `{
			"ruleId": "550e8400-e29b-41d4-a716-446655440001",
			"name": "test rule",
			"description": "Test description",
			"expression": "amount > 1000",
			"action": "DENY",
			"scopes": [],
			"status": "DRAFT",
			"createdAt": "2024-01-15T10:00:00Z",
			"updatedAt": "2024-01-15T11:00:00Z"
		}`

		var rule Rule
		err := json.Unmarshal([]byte(jsonData), &rule)
		require.NoError(t, err)

		assert.Equal(t, uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"), rule.ID)
		assert.Equal(t, "test rule", rule.Name)
		require.NotNil(t, rule.Description)
		assert.Equal(t, "Test description", *rule.Description)
		assert.Equal(t, "amount > 1000", rule.Expression)
		assert.Equal(t, DecisionDeny, rule.Action)
		assert.Equal(t, RuleStatusDraft, rule.Status)
		assert.Empty(t, rule.Scopes)
	})
}

func TestListRulesFilter_Defaults(t *testing.T) {
	t.Run("Success - filter with zero values", func(t *testing.T) {
		filter := ListRulesFilter{}

		assert.Nil(t, filter.Status)
		assert.Nil(t, filter.Action)
		assert.Zero(t, filter.Limit)
		assert.Empty(t, filter.Cursor)
		assert.Empty(t, filter.SortBy)
		assert.Empty(t, filter.SortOrder)
	})

	t.Run("Success - filter with all values set", func(t *testing.T) {
		status := RuleStatusActive
		action := DecisionDeny

		filter := ListRulesFilter{
			Status:    &status,
			Action:    &action,
			Limit:     10,
			Cursor:    "abc123",
			SortBy:    "created_at",
			SortOrder: "DESC",
		}

		require.NotNil(t, filter.Status)
		assert.Equal(t, RuleStatusActive, *filter.Status)
		require.NotNil(t, filter.Action)
		assert.Equal(t, DecisionDeny, *filter.Action)
		assert.Equal(t, 10, filter.Limit)
		assert.Equal(t, "abc123", filter.Cursor)
		assert.Equal(t, "created_at", filter.SortBy)
		assert.Equal(t, "DESC", filter.SortOrder)
	})
}

func TestListRulesResult_Fields(t *testing.T) {
	t.Run("Success - result with rules and pagination", func(t *testing.T) {
		rules := []Rule{
			{ID: testutil.MustDeterministicUUID(2), Name: "rule1", Status: RuleStatusDraft},
			{ID: testutil.MustDeterministicUUID(3), Name: "rule2", Status: RuleStatusActive},
		}

		result := ListRulesResult{
			Rules:      rules,
			NextCursor: "next_cursor_value",
			HasMore:    true,
		}

		assert.Len(t, result.Rules, 2)
		assert.Equal(t, "next_cursor_value", result.NextCursor)
		assert.True(t, result.HasMore)
	})

	t.Run("Success - result with empty rules", func(t *testing.T) {
		result := ListRulesResult{
			Rules:      []Rule{},
			NextCursor: "",
			HasMore:    false,
		}

		assert.Empty(t, result.Rules)
		assert.Empty(t, result.NextCursor)
		assert.False(t, result.HasMore)
	})
}

// TestNewRule_NormalizesScopeSubType verifies that NewRule normalizes every
// Scope.SubType in the provided slice to trimmed lowercase canonical form.
// Persisted scopes must share the same canonical shape regardless of input casing
// so DB state matches the case-insensitive runtime matching semantics.
func TestNewRule_NormalizesScopeSubType(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(100)

	scopes := []Scope{
		{AccountID: &accountID, SubType: testutil.StringPtr("SELL")},
		{AccountID: &accountID, SubType: testutil.StringPtr("  buy  ")},
		{AccountID: &accountID, SubType: testutil.StringPtr("Credit")},
		{AccountID: &accountID, SubType: nil}, // nil stays nil
	}

	rule, err := NewRule(
		"test rule",
		"transaction.amount > 0",
		DecisionAllow,
		scopes,
		nil,
		testutil.FixedTime(),
	)

	require.NoError(t, err)
	require.Len(t, rule.Scopes, 4)

	require.NotNil(t, rule.Scopes[0].SubType)
	require.Equal(t, "sell", *rule.Scopes[0].SubType, "uppercase should be lowered")

	require.NotNil(t, rule.Scopes[1].SubType)
	require.Equal(t, "buy", *rule.Scopes[1].SubType, "whitespace should be trimmed and lowered")

	require.NotNil(t, rule.Scopes[2].SubType)
	require.Equal(t, "credit", *rule.Scopes[2].SubType, "mixed case should be lowered")

	assert.Nil(t, rule.Scopes[3].SubType, "nil SubType must stay nil")
}

// TestRule_Update_NormalizesScopeSubType verifies that Rule.Update normalizes
// every Scope.SubType in the updated slice to trimmed lowercase canonical form.
func TestRule_Update_NormalizesScopeSubType(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(101)

	rule, err := NewRule(
		"test rule",
		"transaction.amount > 0",
		DecisionAllow,
		[]Scope{{AccountID: &accountID, SubType: testutil.StringPtr("sell")}},
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err)

	newScopes := []Scope{
		{AccountID: &accountID, SubType: testutil.StringPtr("SELL")},
		{AccountID: &accountID, SubType: testutil.StringPtr("  BUY  ")},
		{AccountID: &accountID, SubType: nil},
	}

	err = rule.Update(nil, nil, nil, &newScopes, testutil.FixedTime())
	require.NoError(t, err)

	require.Len(t, rule.Scopes, 3)

	require.NotNil(t, rule.Scopes[0].SubType)
	require.Equal(t, "sell", *rule.Scopes[0].SubType)

	require.NotNil(t, rule.Scopes[1].SubType)
	require.Equal(t, "buy", *rule.Scopes[1].SubType)

	assert.Nil(t, rule.Scopes[2].SubType)
}

// TestNewRule_DeepCopiesScopeMerchantID verifies that NewRule deep-copies the
// Scope.MerchantID pointer so external mutations to the caller's slice do not
// leak into the stored rule. This mirrors the defensive copy behaviour applied
// to AccountID, SegmentID and PortfolioID.
func TestNewRule_DeepCopiesScopeMerchantID(t *testing.T) {
	originalValue := testutil.MustDeterministicUUID(200)
	mutatedValue := testutil.MustDeterministicUUID(201)

	// Caller owns the pointer target. The rule's stored scope must not share
	// this memory, otherwise mutating callerMerchant below would leak.
	callerMerchant := originalValue

	scopes := []Scope{
		{MerchantID: &callerMerchant},
	}

	rule, err := NewRule(
		"test rule",
		"transaction.amount > 0",
		DecisionAllow,
		scopes,
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err)
	require.Len(t, rule.Scopes, 1)
	require.NotNil(t, rule.Scopes[0].MerchantID)
	require.Equal(t, originalValue, *rule.Scopes[0].MerchantID)

	// Mutate the caller's pointer target after construction. The stored rule
	// must be unaffected because NewRule performed a deep copy of MerchantID.
	callerMerchant = mutatedValue

	require.NotNil(t, rule.Scopes[0].MerchantID)
	require.Equal(t, originalValue, *rule.Scopes[0].MerchantID,
		"external mutation of MerchantID must not leak into stored rule")
}

// TestRule_Update_DeepCopiesScopeMerchantID verifies that Rule.Update
// deep-copies the Scope.MerchantID pointer so external mutations to the
// caller's slice do not leak into the rule state after update.
func TestRule_Update_DeepCopiesScopeMerchantID(t *testing.T) {
	initialAccountID := testutil.MustDeterministicUUID(202)
	originalValue := testutil.MustDeterministicUUID(203)
	mutatedValue := testutil.MustDeterministicUUID(204)

	rule, err := NewRule(
		"test rule",
		"transaction.amount > 0",
		DecisionAllow,
		[]Scope{{AccountID: &initialAccountID}},
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err)

	// Caller owns the pointer target. The rule's stored scope must not share
	// this memory, otherwise mutating callerMerchant below would leak.
	callerMerchant := originalValue

	newScopes := []Scope{
		{MerchantID: &callerMerchant},
	}

	err = rule.Update(nil, nil, nil, &newScopes, testutil.FixedTime())
	require.NoError(t, err)
	require.Len(t, rule.Scopes, 1)
	require.NotNil(t, rule.Scopes[0].MerchantID)
	require.Equal(t, originalValue, *rule.Scopes[0].MerchantID)

	// Mutate the caller's pointer target after update. The rule's stored
	// MerchantID must remain the original because Update deep-copied it.
	callerMerchant = mutatedValue

	require.NotNil(t, rule.Scopes[0].MerchantID)
	require.Equal(t, originalValue, *rule.Scopes[0].MerchantID,
		"external mutation of MerchantID must not leak into stored rule")
}

// TestNewRule_DeepCopiesScopeTransactionType verifies that NewRule deep-copies
// the Scope.TransactionType pointer so external mutations to the caller's slice
// do not leak into the stored rule. Mirrors the defensive copy behaviour
// applied to AccountID, SegmentID, PortfolioID and MerchantID.
func TestNewRule_DeepCopiesScopeTransactionType(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(300)
	originalValue := TransactionTypePix
	mutatedValue := TransactionTypeCard

	// Caller owns the pointer target. The rule's stored scope must not share
	// this memory, otherwise mutating callerTxType below would leak.
	callerTxType := originalValue

	scopes := []Scope{
		{AccountID: &accountID, TransactionType: &callerTxType},
	}

	rule, err := NewRule(
		"test rule",
		"transaction.amount > 0",
		DecisionAllow,
		scopes,
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err)
	require.Len(t, rule.Scopes, 1)
	require.NotNil(t, rule.Scopes[0].TransactionType)
	require.Equal(t, originalValue, *rule.Scopes[0].TransactionType)

	// Mutate the caller's pointer target after construction. The stored rule
	// must be unaffected because NewRule performed a deep copy of TransactionType.
	callerTxType = mutatedValue

	require.NotNil(t, rule.Scopes[0].TransactionType)
	require.Equal(t, originalValue, *rule.Scopes[0].TransactionType,
		"external mutation of TransactionType must not leak into stored rule")
}

// TestRule_Update_DeepCopiesScopeTransactionType verifies that Rule.Update
// deep-copies the Scope.TransactionType pointer so external mutations to the
// caller's slice do not leak into the rule state after update.
func TestRule_Update_DeepCopiesScopeTransactionType(t *testing.T) {
	initialAccountID := testutil.MustDeterministicUUID(301)
	originalValue := TransactionTypePix
	mutatedValue := TransactionTypeCard

	rule, err := NewRule(
		"test rule",
		"transaction.amount > 0",
		DecisionAllow,
		[]Scope{{AccountID: &initialAccountID}},
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err)

	// Caller owns the pointer target. The rule's stored scope must not share
	// this memory, otherwise mutating callerTxType below would leak.
	callerTxType := originalValue

	newScopes := []Scope{
		{AccountID: &initialAccountID, TransactionType: &callerTxType},
	}

	err = rule.Update(nil, nil, nil, &newScopes, testutil.FixedTime())
	require.NoError(t, err)
	require.Len(t, rule.Scopes, 1)
	require.NotNil(t, rule.Scopes[0].TransactionType)
	require.Equal(t, originalValue, *rule.Scopes[0].TransactionType)

	// Mutate the caller's pointer target after update. The rule's stored
	// TransactionType must remain the original because Update deep-copied it.
	callerTxType = mutatedValue

	require.NotNil(t, rule.Scopes[0].TransactionType)
	require.Equal(t, originalValue, *rule.Scopes[0].TransactionType,
		"external mutation of TransactionType must not leak into stored rule")
}
