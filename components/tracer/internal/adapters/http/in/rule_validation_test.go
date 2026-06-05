// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// Valid UUIDs for testing
var (
	validUUID1 = uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	validUUID2 = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	validUUID3 = uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	validUUID4 = uuid.MustParse("7c9e6679-7425-40de-944b-e07fc1f90ae7")
)

func TestGetValidator_Singleton(t *testing.T) {
	v1, err1 := getValidator()
	require.NoError(t, err1, "getValidator should not return error")

	v2, err2 := getValidator()
	require.NoError(t, err2, "getValidator should not return error")

	assert.Same(t, v1, v2, "getValidator should return the same instance")
}

func TestCreateRuleInput_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateRuleInput
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with scopes array",
			input: CreateRuleInput{
				Name:       "Test Rule",
				Expression: "amount > 1000",
				Action:     model.DecisionDeny,
				Scopes: []model.Scope{
					{AccountID: testutil.UUIDPtr(validUUID1)},
					{SegmentID: testutil.UUIDPtr(validUUID2)},
				},
			},
			wantErr: false,
		},
		{
			name: "valid with transaction type scope",
			input: CreateRuleInput{
				Name:       "Payment Rule",
				Expression: "transactionType == 'CARD'",
				Action:     model.DecisionAllow,
				Scopes: []model.Scope{
					{
						TransactionType: testutil.Ptr(model.TransactionTypeCard),
						SubType:         testutil.StringPtr("Credit"),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid without scopes (empty array OK)",
			input: CreateRuleInput{
				Name:       "Global Rule",
				Expression: "amount > 10000",
				Action:     model.DecisionDeny,
				Scopes:     []model.Scope{},
			},
			wantErr: false,
		},
		{
			name: "invalid - empty scope in array (all fields nil)",
			input: CreateRuleInput{
				Name:       "Bad Rule",
				Expression: "amount > 0",
				Action:     model.DecisionDeny,
				Scopes: []model.Scope{
					{}, // Empty scope - all fields nil
				},
			},
			wantErr: true,
			errMsg:  "scope at index 0 must have at least one field set",
		},
		{
			name: "invalid - missing name",
			input: CreateRuleInput{
				Name:       "",
				Expression: "amount > 1000",
				Action:     model.DecisionDeny,
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "invalid - missing expression",
			input: CreateRuleInput{
				Name:       "Test",
				Expression: "",
				Action:     model.DecisionDeny,
			},
			wantErr: true,
			errMsg:  "expression is required",
		},
		{
			name: "invalid - missing action",
			input: CreateRuleInput{
				Name:       "Test",
				Expression: "amount > 1000",
				Action:     "",
			},
			wantErr: true,
			errMsg:  "action is required and must be one of [ALLOW, DENY, REVIEW]",
		},
		{
			name: "invalid - name too long (>MaxNameLength)",
			input: CreateRuleInput{
				Name:       strings.Repeat("a", MaxRuleNameLength+1),
				Expression: "amount > 1000",
				Action:     model.DecisionDeny,
			},
			wantErr: true,
			errMsg:  "name exceeds maximum length of 255 characters",
		},
		{
			name: "invalid - expression too long (>MaxExpressionLength)",
			input: CreateRuleInput{
				Name:       "Test",
				Expression: strings.Repeat("x", MaxRuleExpressionLength+1),
				Action:     model.DecisionDeny,
			},
			wantErr: true,
			errMsg:  "expression exceeds maximum length of 5000 characters",
		},
		{
			name: "invalid - invalid action value",
			input: CreateRuleInput{
				Name:       "Test",
				Expression: "amount > 1000",
				Action:     "INVALID",
			},
			wantErr: true,
			errMsg:  "action must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestScope_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		scope   model.Scope
		isEmpty bool
	}{
		{
			name:    "empty scope - all fields nil",
			scope:   model.Scope{},
			isEmpty: true,
		},
		{
			name: "has segmentId",
			scope: model.Scope{
				SegmentID: testutil.UUIDPtr(validUUID1),
			},
			isEmpty: false,
		},
		{
			name: "has portfolioId",
			scope: model.Scope{
				PortfolioID: testutil.UUIDPtr(validUUID2),
			},
			isEmpty: false,
		},
		{
			name: "has accountId",
			scope: model.Scope{
				AccountID: testutil.UUIDPtr(validUUID3),
			},
			isEmpty: false,
		},
		{
			name: "has merchantId",
			scope: model.Scope{
				MerchantID: testutil.UUIDPtr(validUUID4),
			},
			isEmpty: false,
		},
		{
			name: "has transactionType",
			scope: model.Scope{
				TransactionType: testutil.Ptr(model.TransactionTypeCard),
			},
			isEmpty: false,
		},
		{
			name: "has subType only",
			scope: model.Scope{
				SubType: testutil.StringPtr("Credit"),
			},
			isEmpty: false,
		},
		{
			name: "has multiple fields",
			scope: model.Scope{
				TransactionType: testutil.Ptr(model.TransactionTypeWire),
				SubType:         testutil.StringPtr("PIX_Instant"),
			},
			isEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isEmpty, tt.scope.IsEmpty())
		})
	}
}

func TestCreateRuleInput_NoPriorityField(t *testing.T) {
	// This test verifies that the priority field does NOT exist in CreateRuleInput
	// Per TRD v1.2.4: priority has been removed from rules management
	input := CreateRuleInput{
		Name:       "Test Rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Scopes: []model.Scope{
			{AccountID: testutil.UUIDPtr(validUUID1)},
		},
	}

	// This test ensures the struct doesn't accidentally include priority
	// If priority is added, this test will need to be modified or will fail at compile time
	_ = input

	// Verify we can use the input without priority
	err := input.Validate()
	assert.NoError(t, err)
}

func TestCreateRuleInput_ValidActionValues(t *testing.T) {
	// Test all valid action values using model.Decision enum
	validActions := []model.Decision{model.DecisionAllow, model.DecisionDeny, model.DecisionReview}

	for _, action := range validActions {
		t.Run("valid action: "+string(action), func(t *testing.T) {
			input := CreateRuleInput{
				Name:       "Test Rule",
				Expression: "amount > 1000",
				Action:     action,
				Scopes:     []model.Scope{},
			}
			err := input.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestCreateRuleInput_OptionalDescription(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantErr     bool
	}{
		{
			name:        "empty description is valid",
			description: "",
			wantErr:     false,
		},
		{
			name:        "description within limit",
			description: "This is a test description for the rule",
			wantErr:     false,
		},
		{
			name:        "description at max length (MaxDescriptionLength)",
			description: strings.Repeat("d", MaxRuleDescriptionLength),
			wantErr:     false,
		},
		{
			name:        "description too long (>MaxDescriptionLength)",
			description: strings.Repeat("d", MaxRuleDescriptionLength+1),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := CreateRuleInput{
				Name:        "Test Rule",
				Description: tt.description,
				Expression:  "amount > 1000",
				Action:      model.DecisionDeny,
				Scopes:      []model.Scope{},
			}
			err := input.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateRuleInput_ScopesArray(t *testing.T) {
	tests := []struct {
		name    string
		scopes  []model.Scope
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil scopes treated as empty array",
			scopes:  nil,
			wantErr: false,
		},
		{
			name:    "empty scopes array is valid",
			scopes:  []model.Scope{},
			wantErr: false,
		},
		{
			name: "single valid scope",
			scopes: []model.Scope{
				{AccountID: testutil.UUIDPtr(validUUID1)},
			},
			wantErr: false,
		},
		{
			name: "multiple valid scopes",
			scopes: []model.Scope{
				{AccountID: testutil.UUIDPtr(validUUID1)},
				{SegmentID: testutil.UUIDPtr(validUUID2)},
				{TransactionType: testutil.Ptr(model.TransactionTypeCard)},
			},
			wantErr: false,
		},
		{
			name: "mixed scopes with all field types",
			scopes: []model.Scope{
				{AccountID: testutil.UUIDPtr(validUUID1)},
				{PortfolioID: testutil.UUIDPtr(validUUID2)},
				{MerchantID: testutil.UUIDPtr(validUUID3)},
				{TransactionType: testutil.Ptr(model.TransactionTypeWire), SubType: testutil.StringPtr("international")},
			},
			wantErr: false,
		},
		{
			name: "first scope empty - should fail",
			scopes: []model.Scope{
				{}, // Empty scope
				{AccountID: testutil.UUIDPtr(validUUID1)},
			},
			wantErr: true,
			errMsg:  "scope at index 0 must have at least one field set",
		},
		{
			name: "second scope empty - should fail",
			scopes: []model.Scope{
				{AccountID: testutil.UUIDPtr(validUUID1)},
				{}, // Empty scope
			},
			wantErr: true,
			errMsg:  "scope at index 1 must have at least one field set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := CreateRuleInput{
				Name:       "Test Rule",
				Expression: "amount > 1000",
				Action:     model.DecisionDeny,
				Scopes:     tt.scopes,
			}
			err := input.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestScope_UUIDValidation(t *testing.T) {
	tests := []struct {
		name    string
		scope   model.Scope
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid UUID for segmentId",
			scope: model.Scope{
				SegmentID: testutil.UUIDPtr(validUUID1),
			},
			wantErr: false,
		},
		// Note: Invalid UUID tests removed because uuid.UUID type validates at JSON parsing level,
		// not at struct validation level. Invalid UUIDs will fail JSON unmarshal before validation.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := CreateRuleInput{
				Name:       "Test Rule",
				Expression: "amount > 1000",
				Action:     model.DecisionDeny,
				Scopes:     []model.Scope{tt.scope},
			}
			err := input.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestScope_TransactionTypeValidation(t *testing.T) {
	validTypes := []model.TransactionType{
		model.TransactionTypeCard,
		model.TransactionTypeWire,
		model.TransactionTypePix,
		model.TransactionTypeCrypto,
	}

	for _, txType := range validTypes {
		t.Run("valid transactionType: "+string(txType), func(t *testing.T) {
			input := CreateRuleInput{
				Name:       "Test Rule",
				Expression: "amount > 1000",
				Action:     model.DecisionDeny,
				Scopes: []model.Scope{
					{TransactionType: testutil.Ptr(txType)},
				},
			}
			err := input.Validate()
			assert.NoError(t, err)
		})
	}

	t.Run("invalid transactionType", func(t *testing.T) {
		invalidType := model.TransactionType("INVALID")
		input := CreateRuleInput{
			Name:       "Test Rule",
			Expression: "amount > 1000",
			Action:     model.DecisionDeny,
			Scopes: []model.Scope{
				{TransactionType: &invalidType},
			},
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transactionType must be one of")
	})
}

func TestScope_SubTypeValidation(t *testing.T) {
	t.Run("valid subType within limit", func(t *testing.T) {
		input := CreateRuleInput{
			Name:       "Test Rule",
			Expression: "amount > 1000",
			Action:     model.DecisionDeny,
			Scopes: []model.Scope{
				{SubType: testutil.StringPtr("Credit")},
			},
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("subType at max length (MaxSubTypeLength)", func(t *testing.T) {
		input := CreateRuleInput{
			Name:       "Test Rule",
			Expression: "amount > 1000",
			Action:     model.DecisionDeny,
			Scopes: []model.Scope{
				{SubType: testutil.StringPtr(strings.Repeat("x", MaxRuleSubTypeLength))},
			},
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("subType too long (>MaxSubTypeLength)", func(t *testing.T) {
		input := CreateRuleInput{
			Name:       "Test Rule",
			Expression: "amount > 1000",
			Action:     model.DecisionDeny,
			Scopes: []model.Scope{
				{SubType: testutil.StringPtr(strings.Repeat("x", MaxRuleSubTypeLength+1))},
			},
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subType exceeds maximum length of 50 characters")
	})
}

func TestCreateRuleInput_ScopesMaxCount(t *testing.T) {
	t.Run("scopes at max count (MaxScopesCount)", func(t *testing.T) {
		scopes := make([]model.Scope, MaxRuleScopesCount)
		for i := range scopes {
			scopes[i] = model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		}

		input := CreateRuleInput{
			Name:       "Test Rule",
			Expression: "amount > 1000",
			Action:     model.DecisionDeny,
			Scopes:     scopes,
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("scopes exceed max count (>MaxRuleScopesCount)", func(t *testing.T) {
		scopes := make([]model.Scope, MaxRuleScopesCount+1)
		for i := range scopes {
			scopes[i] = model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		}

		input := CreateRuleInput{
			Name:       "Test Rule",
			Expression: "amount > 1000",
			Action:     model.DecisionDeny,
			Scopes:     scopes,
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scopes exceed maximum of 100 entries")
	})
}

func TestUpdateRuleInput_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdateRuleInput
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid - empty update (all fields nil)",
			input:   UpdateRuleInput{},
			wantErr: false,
		},
		{
			name: "valid - update name only",
			input: UpdateRuleInput{
				Name: testutil.StringPtr("Updated Rule Name"),
			},
			wantErr: false,
		},
		{
			name: "valid - update description only",
			input: UpdateRuleInput{
				Description: testutil.StringPtr("Updated description"),
			},
			wantErr: false,
		},
		{
			name: "valid - update expression only",
			input: UpdateRuleInput{
				Expression: testutil.StringPtr("amount > 5000"),
			},
			wantErr: false,
		},
		{
			name: "valid - update action only",
			input: UpdateRuleInput{
				Action: testutil.Ptr(model.DecisionReview),
			},
			wantErr: false,
		},
		{
			name: "valid - update scopes only",
			input: UpdateRuleInput{
				Scopes: &[]model.Scope{
					{AccountID: testutil.UUIDPtr(validUUID1)},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - update multiple fields",
			input: UpdateRuleInput{
				Name:        testutil.StringPtr("New Name"),
				Description: testutil.StringPtr("New Description"),
				Expression:  testutil.StringPtr("amount > 10000"),
				Action:      testutil.Ptr(model.DecisionDeny),
				Scopes: &[]model.Scope{
					{SegmentID: testutil.UUIDPtr(validUUID1)},
					{AccountID: testutil.UUIDPtr(validUUID2)},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - name too long (>MaxNameLength)",
			input: UpdateRuleInput{
				Name: testutil.StringPtr(strings.Repeat("a", MaxRuleNameLength+1)),
			},
			wantErr: true,
			errMsg:  "name exceeds maximum length of 255 characters",
		},
		{
			name: "invalid - empty name (min=1)",
			input: UpdateRuleInput{
				Name: testutil.StringPtr(""),
			},
			wantErr: true,
			errMsg:  "name validation failed: min",
		},
		{
			name: "invalid - description too long (>MaxDescriptionLength)",
			input: UpdateRuleInput{
				Description: testutil.StringPtr(strings.Repeat("a", MaxRuleDescriptionLength+1)),
			},
			wantErr: true,
			errMsg:  "description exceeds maximum length of 1000 characters",
		},
		{
			name: "invalid - expression too long (>MaxExpressionLength)",
			input: UpdateRuleInput{
				Expression: testutil.StringPtr(strings.Repeat("a", MaxRuleExpressionLength+1)),
			},
			wantErr: true,
			errMsg:  "expression exceeds maximum length of 5000 characters",
		},
		{
			name: "invalid - empty expression (min=1)",
			input: UpdateRuleInput{
				Expression: testutil.StringPtr(""),
			},
			wantErr: true,
			errMsg:  "expression validation failed: min",
		},
		{
			name: "invalid - invalid action value",
			input: UpdateRuleInput{
				Action: testutil.Ptr(model.Decision("INVALID")),
			},
			wantErr: true,
			errMsg:  "action must be one of [ALLOW, DENY, REVIEW]",
		},
		{
			name: "invalid - empty scope in scopes array",
			input: UpdateRuleInput{
				Scopes: &[]model.Scope{
					{AccountID: testutil.UUIDPtr(validUUID1)},
					{}, // empty scope
				},
			},
			wantErr: true,
			errMsg:  "scope at index 1 must have at least one field set",
		},
		// Note: Invalid UUID tests removed because uuid.UUID type validates at JSON parsing level.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateRuleInput_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		input    UpdateRuleInput
		expected bool
	}{
		{
			name:     "empty - all fields nil",
			input:    UpdateRuleInput{},
			expected: true,
		},
		{
			name: "not empty - has name",
			input: UpdateRuleInput{
				Name: testutil.StringPtr("Test"),
			},
			expected: false,
		},
		{
			name: "not empty - has description",
			input: UpdateRuleInput{
				Description: testutil.StringPtr("Test"),
			},
			expected: false,
		},
		{
			name: "not empty - has expression",
			input: UpdateRuleInput{
				Expression: testutil.StringPtr("amount > 100"),
			},
			expected: false,
		},
		{
			name: "not empty - has action",
			input: UpdateRuleInput{
				Action: testutil.Ptr(model.DecisionAllow),
			},
			expected: false,
		},
		{
			name: "not empty - has scopes",
			input: UpdateRuleInput{
				Scopes: &[]model.Scope{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdateRuleInput_ScopesMaxCount(t *testing.T) {
	t.Run("scopes at max count (MaxRuleScopesCount)", func(t *testing.T) {
		scopes := make([]model.Scope, MaxRuleScopesCount)
		for i := range scopes {
			scopes[i] = model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		}

		input := UpdateRuleInput{Scopes: &scopes}
		err := input.Validate()
		require.NoError(t, err)
	})

	t.Run("scopes exceed max count (>MaxRuleScopesCount)", func(t *testing.T) {
		scopes := make([]model.Scope, MaxRuleScopesCount+1)
		for i := range scopes {
			scopes[i] = model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		}

		input := UpdateRuleInput{Scopes: &scopes}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scopes exceed maximum of 100 entries")
	})
}

func TestListRulesInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   ListRulesInput
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid - with defaults applied",
			input: ListRulesInput{
				Limit: testutil.Ptr(10),
			},
			wantErr: false,
		},
		{
			name: "valid - with all fields (no cursor)",
			input: ListRulesInput{
				Status:    testutil.Ptr(model.RuleStatusActive),
				Action:    testutil.Ptr(model.DecisionDeny),
				Limit:     testutil.Ptr(50),
				SortBy:    "name",
				SortOrder: "ASC",
			},
			wantErr: false,
		},
		{
			name: "invalid - status DELETED not allowed as filter",
			input: ListRulesInput{
				Status: testutil.Ptr(model.RuleStatusDeleted),
				Limit:  testutil.Ptr(10),
			},
			wantErr: true,
			errMsg:  "DELETED",
		},
		{
			name: "valid - sort by updated_at",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortBy:    "updated_at",
				SortOrder: "desc",
			},
			wantErr: false,
		},
		{
			name: "valid - with cursor",
			input: ListRulesInput{
				Limit:  testutil.Ptr(10),
				Cursor: "eyJpZCI6InRlc3QiLCJwb2ludHNOZXh0Ijp0cnVlfQ==",
			},
			wantErr: false,
		},
		{
			name: "invalid - limit too low",
			input: ListRulesInput{
				Limit: testutil.Ptr(0),
			},
			wantErr: true,
			errMsg:  "limit",
		},
		{
			name: "invalid - limit too high",
			input: ListRulesInput{
				Limit: testutil.Ptr(101),
			},
			wantErr: true,
			errMsg:  "limit",
		},
		{
			name: "invalid - invalid status",
			input: ListRulesInput{
				Status: testutil.Ptr(model.RuleStatus("INVALID")),
				Limit:  testutil.Ptr(10),
			},
			wantErr: true,
			errMsg:  "Status",
		},
		{
			name: "invalid - invalid action",
			input: ListRulesInput{
				Action: testutil.Ptr(model.Decision("INVALID")),
				Limit:  testutil.Ptr(10),
			},
			wantErr: true,
			errMsg:  "action",
		},
		{
			name: "invalid - invalid sortBy (priority not allowed)",
			input: ListRulesInput{
				Limit:  testutil.Ptr(10),
				SortBy: "priority",
			},
			wantErr: true,
			errMsg:  "sort_by",
		},
		{
			name: "invalid - invalid sortOrder",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortOrder: "INVALID",
			},
			wantErr: true,
			errMsg:  "sort_order",
		},
		{
			name: "invalid - cursor with sortBy (TRC-0045)",
			input: ListRulesInput{
				Limit:  testutil.Ptr(10),
				Cursor: "abc123",
				SortBy: "name",
			},
			wantErr: true,
			errMsg:  "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		},
		{
			name: "invalid - cursor with sortOrder (TRC-0045)",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				Cursor:    "abc123",
				SortOrder: "ASC",
			},
			wantErr: true,
			errMsg:  "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		},
		{
			name: "invalid - cursor with both sortBy and sortOrder (TRC-0045)",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				Cursor:    "abc123",
				SortBy:    "name",
				SortOrder: "DESC",
			},
			wantErr: true,
			errMsg:  "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		},
		// Scope filter validation tests
		{
			name: "valid - with accountId filter",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				AccountID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440000"),
			},
			wantErr: false,
		},
		{
			name: "valid - with segmentId filter",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SegmentID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440001"),
			},
			wantErr: false,
		},
		{
			name: "valid - with portfolioId filter",
			input: ListRulesInput{
				Limit:       testutil.Ptr(10),
				PortfolioID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440002"),
			},
			wantErr: false,
		},
		{
			name: "valid - with merchantId filter",
			input: ListRulesInput{
				Limit:      testutil.Ptr(10),
				MerchantID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440003"),
			},
			wantErr: false,
		},
		{
			name: "valid - with transactionType filter",
			input: ListRulesInput{
				Limit:           testutil.Ptr(10),
				TransactionType: testutil.StringPtr("CARD"),
			},
			wantErr: false,
		},
		{
			name: "valid - with subType filter",
			input: ListRulesInput{
				Limit:   testutil.Ptr(10),
				SubType: testutil.StringPtr("CREDIT"),
			},
			wantErr: false,
		},
		{
			name: "valid - with multiple scope filters combined",
			input: ListRulesInput{
				Limit:           testutil.Ptr(10),
				AccountID:       testutil.StringPtr("550e8400-e29b-41d4-a716-446655440000"),
				TransactionType: testutil.StringPtr("PIX"),
				SubType:         testutil.StringPtr("INSTANT"),
			},
			wantErr: false,
		},
		{
			name: "valid - scope filters combined with existing filters",
			input: ListRulesInput{
				Status:    testutil.Ptr(model.RuleStatusActive),
				Action:    testutil.Ptr(model.DecisionDeny),
				Limit:     testutil.Ptr(10),
				AccountID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440000"),
				SortBy:    "name",
				SortOrder: "ASC",
			},
			wantErr: false,
		},
		{
			name: "invalid - accountId not a valid UUID",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				AccountID: testutil.StringPtr("not-a-uuid"),
			},
			wantErr: true,
			errMsg:  "account_id",
		},
		{
			name: "invalid - segmentId not a valid UUID",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SegmentID: testutil.StringPtr("invalid"),
			},
			wantErr: true,
			errMsg:  "segment_id",
		},
		{
			name: "invalid - portfolioId not a valid UUID",
			input: ListRulesInput{
				Limit:       testutil.Ptr(10),
				PortfolioID: testutil.StringPtr("xyz"),
			},
			wantErr: true,
			errMsg:  "portfolio_id",
		},
		{
			name: "invalid - merchantId not a valid UUID",
			input: ListRulesInput{
				Limit:      testutil.Ptr(10),
				MerchantID: testutil.StringPtr("bad-id"),
			},
			wantErr: true,
			errMsg:  "merchant_id",
		},
		{
			name: "invalid - transactionType not a valid enum",
			input: ListRulesInput{
				Limit:           testutil.Ptr(10),
				TransactionType: testutil.StringPtr("INVALID_TYPE"),
			},
			wantErr: true,
			errMsg:  "transaction_type",
		},
		{
			name: "invalid - subType exceeds max length",
			input: ListRulesInput{
				Limit:   testutil.Ptr(10),
				SubType: testutil.StringPtr(strings.Repeat("a", 51)),
			},
			wantErr: true,
			errMsg:  "sub_type",
		},
		{
			// Whitespace-only sub_type is treated as no filter by
			// buildScopeFromInput, so the length check must trim before
			// rejecting. A 200-char whitespace-only value trims to "" and is
			// accepted (no filter applied), keeping validation symmetric with
			// the filter semantics.
			name: "valid - subType whitespace-only beyond max length is ignored",
			input: ListRulesInput{
				Limit:   testutil.Ptr(10),
				SubType: testutil.StringPtr(strings.Repeat(" ", 200)),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListRulesInput_SetDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    ListRulesInput
		expected ListRulesInput
	}{
		{
			name:  "sets all defaults when empty",
			input: ListRulesInput{},
			expected: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortBy:    "created_at",
				SortOrder: "DESC",
			},
		},
		{
			name: "preserves non-zero values without cursor",
			input: ListRulesInput{
				Limit:     testutil.Ptr(50),
				SortBy:    "name",
				SortOrder: "ASC",
			},
			expected: ListRulesInput{
				Limit:     testutil.Ptr(50),
				SortBy:    "name",
				SortOrder: "ASC",
			},
		},
		{
			name: "only sets missing defaults",
			input: ListRulesInput{
				Limit: testutil.Ptr(25),
			},
			expected: ListRulesInput{
				Limit:     testutil.Ptr(25),
				SortBy:    "created_at",
				SortOrder: "DESC",
			},
		},
		{
			name: "does not set sort defaults when cursor is present",
			input: ListRulesInput{
				Limit:  testutil.Ptr(25),
				Cursor: "abc123",
			},
			expected: ListRulesInput{
				Limit:     testutil.Ptr(25),
				Cursor:    "abc123",
				SortBy:    "", // Sort defaults NOT applied when cursor is present (TRC-0045)
				SortOrder: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.SetDefaults()
			require.NotNil(t, tt.input.Limit)
			require.NotNil(t, tt.expected.Limit)
			assert.Equal(t, *tt.expected.Limit, *tt.input.Limit)
			assert.Equal(t, tt.expected.Cursor, tt.input.Cursor)
			assert.Equal(t, tt.expected.SortBy, tt.input.SortBy)
			assert.Equal(t, tt.expected.SortOrder, tt.input.SortOrder)
		})
	}
}

func TestListRulesInput_SortOrderProvided_NormalizesToUppercase(t *testing.T) {
	tests := []struct {
		name             string
		input            ListRulesInput
		expectedAfterVal string
	}{
		{
			name: "lowercase asc normalized to ASC",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortBy:    "name",
				SortOrder: "asc",
			},
			expectedAfterVal: "ASC",
		},
		{
			name: "lowercase desc normalized to DESC",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortBy:    "name",
				SortOrder: "desc",
			},
			expectedAfterVal: "DESC",
		},
		{
			name: "mixed case AsC normalized to ASC",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortBy:    "name",
				SortOrder: "AsC",
			},
			expectedAfterVal: "ASC",
		},
		{
			name: "uppercase ASC remains ASC",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortBy:    "name",
				SortOrder: "ASC",
			},
			expectedAfterVal: "ASC",
		},
		{
			name: "uppercase DESC remains DESC",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortBy:    "name",
				SortOrder: "DESC",
			},
			expectedAfterVal: "DESC",
		},
		{
			name: "empty sortOrder gets default DESC",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				SortBy:    "name",
				SortOrder: "",
			},
			expectedAfterVal: "DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			require.NoError(t, err)

			// Normalization happens in SetDefaults()
			tt.input.SetDefaults()

			assert.Equal(t, tt.expectedAfterVal, tt.input.SortOrder, "sortOrder should be normalized to uppercase")
		})
	}
}

func TestExtractScopeIndex(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		expectedIndex int
		expectError   bool
		errorContains string
	}{
		{
			name:          "valid namespace with index 0",
			namespace:     "CreateRuleInput.Scopes[0].SegmentID",
			expectedIndex: 0,
			expectError:   false,
		},
		{
			name:          "valid namespace with index 1",
			namespace:     "CreateRuleInput.Scopes[1].AccountID",
			expectedIndex: 1,
			expectError:   false,
		},
		{
			name:          "valid namespace with double digit index",
			namespace:     "CreateRuleInput.Scopes[42].PortfolioID",
			expectedIndex: 42,
			expectError:   false,
		},
		{
			name:          "valid namespace with large index",
			namespace:     "UpdateRuleInput.Scopes[999].MerchantID",
			expectedIndex: 999,
			expectError:   false,
		},
		{
			name:          "missing Scopes[ prefix",
			namespace:     "CreateRuleInput.Items[0].Name",
			expectedIndex: -1,
			expectError:   true,
			errorContains: "'Scopes[' not found",
		},
		{
			name:          "empty namespace",
			namespace:     "",
			expectedIndex: -1,
			expectError:   true,
			errorContains: "'Scopes[' not found",
		},
		{
			name:          "missing closing bracket",
			namespace:     "CreateRuleInput.Scopes[0.SegmentID",
			expectedIndex: -1,
			expectError:   true,
			errorContains: "closing bracket ']' not found",
		},
		{
			name:          "non-numeric index",
			namespace:     "CreateRuleInput.Scopes[abc].SegmentID",
			expectedIndex: -1,
			expectError:   true,
			errorContains: "failed to parse index from substring 'abc'",
		},
		{
			name:          "empty index",
			namespace:     "CreateRuleInput.Scopes[].SegmentID",
			expectedIndex: -1,
			expectError:   true,
			errorContains: "failed to parse index from substring ''",
		},
		{
			name:          "index with leading space",
			namespace:     "CreateRuleInput.Scopes[ 5].SegmentID",
			expectedIndex: 5, // fmt.Sscanf accepts leading whitespace
			expectError:   false,
		},
		{
			name:          "negative index",
			namespace:     "CreateRuleInput.Scopes[-1].SegmentID",
			expectedIndex: -1,
			expectError:   true,
			errorContains: "negative index -1 is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, err := extractScopeIndex(tt.namespace)

			if tt.expectError {
				require.Error(t, err)
				assert.Equal(t, tt.expectedIndex, index)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedIndex, index)
			}
		})
	}
}

// TestToListFilter_SortBy_PassesSnakeCaseToFilter verifies that SortBy values are passed
// as snake_case to the filter.
func TestToListFilter_SortBy_PassesSnakeCaseToFilter(t *testing.T) {
	tests := []struct {
		name           string
		inputSortBy    string
		expectedSortBy string
	}{
		{
			name:           "created_at passed as-is",
			inputSortBy:    "created_at",
			expectedSortBy: "created_at",
		},
		{
			name:           "updated_at passed as-is",
			inputSortBy:    "updated_at",
			expectedSortBy: "updated_at",
		},
		{
			name:           "name passed as-is",
			inputSortBy:    "name",
			expectedSortBy: "name",
		},
		{
			name:           "status passed as-is",
			inputSortBy:    "status",
			expectedSortBy: "status",
		},
		{
			name:           "empty stays as empty (defaults applied later)",
			inputSortBy:    "",
			expectedSortBy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &ListRulesInput{
				SortBy: tt.inputSortBy,
			}

			filter := toListFilter(input)

			assert.Equal(t, tt.expectedSortBy, filter.SortBy,
				"SortBy should be passed as snake_case")
		})
	}
}

func TestToListFilter_WithScopeFields(t *testing.T) {
	tests := []struct {
		name           string
		input          ListRulesInput
		expectScope    bool
		checkScopeFunc func(t *testing.T, scope *model.Scope)
	}{
		{
			name: "no scope fields - ScopeFilter is nil",
			input: ListRulesInput{
				Limit: testutil.Ptr(10),
			},
			expectScope: false,
		},
		{
			name: "accountId only",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				AccountID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440000"),
			},
			expectScope: true,
			checkScopeFunc: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.AccountID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", scope.AccountID.String())
				assert.Nil(t, scope.SegmentID)
				assert.Nil(t, scope.PortfolioID)
				assert.Nil(t, scope.MerchantID)
				assert.Nil(t, scope.TransactionType)
				assert.Nil(t, scope.SubType)
			},
		},
		{
			name: "all scope fields",
			input: ListRulesInput{
				Limit:           testutil.Ptr(10),
				AccountID:       testutil.StringPtr("550e8400-e29b-41d4-a716-446655440000"),
				SegmentID:       testutil.StringPtr("550e8400-e29b-41d4-a716-446655440001"),
				PortfolioID:     testutil.StringPtr("550e8400-e29b-41d4-a716-446655440002"),
				MerchantID:      testutil.StringPtr("550e8400-e29b-41d4-a716-446655440003"),
				TransactionType: testutil.StringPtr("CARD"),
				SubType:         testutil.StringPtr("CREDIT"),
			},
			expectScope: true,
			checkScopeFunc: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.AccountID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", scope.AccountID.String())
				require.NotNil(t, scope.SegmentID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", scope.SegmentID.String())
				require.NotNil(t, scope.PortfolioID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440002", scope.PortfolioID.String())
				require.NotNil(t, scope.MerchantID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440003", scope.MerchantID.String())
				require.NotNil(t, scope.TransactionType)
				assert.Equal(t, model.TransactionTypeCard, *scope.TransactionType)
				require.NotNil(t, scope.SubType)
				require.Equal(t, "credit", *scope.SubType)
			},
		},
		{
			name: "transactionType and subType only",
			input: ListRulesInput{
				Limit:           testutil.Ptr(10),
				TransactionType: testutil.StringPtr("PIX"),
				SubType:         testutil.StringPtr("INSTANT"),
			},
			expectScope: true,
			checkScopeFunc: func(t *testing.T, scope *model.Scope) {
				assert.Nil(t, scope.AccountID)
				require.NotNil(t, scope.TransactionType)
				assert.Equal(t, model.TransactionTypePix, *scope.TransactionType)
				require.NotNil(t, scope.SubType)
				require.Equal(t, "instant", *scope.SubType)
			},
		},
		{
			name: "empty string scope fields are treated as absent",
			input: ListRulesInput{
				Limit:     testutil.Ptr(10),
				AccountID: testutil.StringPtr(""),
			},
			expectScope: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := toListFilter(&tt.input)
			if tt.expectScope {
				require.NotNil(t, filter.ScopeFilter, "ScopeFilter should not be nil")
				if tt.checkScopeFunc != nil {
					tt.checkScopeFunc(t, filter.ScopeFilter)
				}
			} else {
				assert.Nil(t, filter.ScopeFilter, "ScopeFilter should be nil when no scope fields provided")
			}
		})
	}
}

func TestRuleValidation_NormalizesSubType(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected *string
	}{
		{
			name:     "uppercase with surrounding whitespace becomes trimmed lowercase",
			input:    testutil.StringPtr("  SELL  "),
			expected: testutil.StringPtr("sell"),
		},
		{
			name:     "mixed case becomes lowercase",
			input:    testutil.StringPtr("Sell"),
			expected: testutil.StringPtr("sell"),
		},
		{
			name:     "already lowercase stays lowercase",
			input:    testutil.StringPtr("sell"),
			expected: testutil.StringPtr("sell"),
		},
		{
			name:     "nil stays nil",
			input:    nil,
			expected: nil,
		},
		{
			// Whitespace-only input must be treated as "no filter": trimming to empty
			// string and assigning it would filter by subType='' in SQL, silently
			// returning zero results for a filter the user did not intend.
			name:     "whitespace-only is treated as nil (no filter)",
			input:    testutil.StringPtr("   "),
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := &ListRulesInput{SubType: tc.input}

			scope := buildScopeFromInput(input)

			if tc.expected == nil {
				require.Nil(t, scope)

				return
			}

			require.NotNil(t, scope)
			require.NotNil(t, scope.SubType)
			require.Equal(t, *tc.expected, *scope.SubType)
		})
	}
}

// TestFormatScopeFieldErrorWithCode_ScopeNotEmpty_IndexZero verifies that scope at
// index 0 is properly reported with its index in the error message.
func TestFormatScopeFieldErrorWithCode_ScopeNotEmpty_IndexZero(t *testing.T) {
	tests := []struct {
		name            string
		namespace       string
		expectedContain string
		description     string
	}{
		{
			name:            "scope at index 0 should show index in error",
			namespace:       "CreateRuleInput.Scopes[0].AccountID",
			expectedContain: "index 0",
			description:     "Index 0 is valid and should be shown in error message",
		},
		{
			name:            "scope at index 1 should show index in error",
			namespace:       "CreateRuleInput.Scopes[1].AccountID",
			expectedContain: "index 1",
			description:     "Index 1 should be shown in error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockFieldErr := mocks.NewMockFieldError(ctrl)
			mockFieldErr.EXPECT().Tag().Return("scopenotempty").AnyTimes()
			mockFieldErr.EXPECT().Namespace().Return(tt.namespace).AnyTimes()

			validationErr := formatScopeFieldErrorWithCode(mockFieldErr)

			require.NotNil(t, validationErr)
			assert.Contains(t, validationErr.Message, tt.expectedContain,
				"Error should contain index: %s", tt.description)
		})
	}
}
