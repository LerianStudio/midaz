// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
	"tracer/pkg/model"
)

// Valid UUIDs for limit validation testing
var (
	limitValidUUID1 = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	limitValidUUID2 = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c9")
	limitValidUUID3 = uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d480")
	limitValidUUID4 = uuid.MustParse("7c9e6679-7425-40de-944b-e07fc1f90ae8")
)

func TestLimitValidator_UsesSharedSingleton(t *testing.T) {
	// Verify that limit validation uses the same validator singleton
	v1, err1 := getValidator()
	require.NoError(t, err1, "getValidator should not return error")

	v2, err2 := getValidator()
	require.NoError(t, err2, "getValidator should not return error")

	assert.Same(t, v1, v2, "getValidator should return the same instance")
}

func TestLimitScopeInput_IsEmpty(t *testing.T) {
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
				SegmentID: testutil.UUIDPtr(limitValidUUID1),
			},
			isEmpty: false,
		},
		{
			name: "has portfolioId",
			scope: model.Scope{
				PortfolioID: testutil.UUIDPtr(limitValidUUID2),
			},
			isEmpty: false,
		},
		{
			name: "has accountId",
			scope: model.Scope{
				AccountID: testutil.UUIDPtr(limitValidUUID3),
			},
			isEmpty: false,
		},
		{
			name: "has merchantId",
			scope: model.Scope{
				MerchantID: testutil.UUIDPtr(limitValidUUID4),
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
				SubType:         testutil.StringPtr("International"),
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

func TestCreateLimitInput_ValidLimitTypeValues(t *testing.T) {
	validTypes := []model.LimitType{
		model.LimitTypeDaily,
		model.LimitTypeWeekly,
		model.LimitTypeMonthly,
		model.LimitTypeCustom,
		model.LimitTypePerTransaction,
	}

	for _, limitType := range validTypes {
		t.Run("valid limitType: "+string(limitType), func(t *testing.T) {
			input := CreateLimitInput{
				Name:      "Test Limit",
				LimitType: limitType,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
			}
			err := input.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestCreateLimitInput_CurrencyValidation(t *testing.T) {
	tests := []struct {
		name     string
		currency string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid - BRL",
			currency: "BRL",
			wantErr:  false,
		},
		{
			name:     "valid - USD",
			currency: "USD",
			wantErr:  false,
		},
		{
			name:     "valid - EUR",
			currency: "EUR",
			wantErr:  false,
		},
		{
			name:     "invalid - lowercase",
			currency: "brl",
			wantErr:  true,
			errMsg:   "currency must be uppercase",
		},
		{
			name:     "invalid - too short",
			currency: "BR",
			wantErr:  true,
			errMsg:   "currency must be exactly 3 characters",
		},
		{
			name:     "invalid - too long",
			currency: "BRLL",
			wantErr:  true,
			errMsg:   "currency must be exactly 3 characters",
		},
		{
			name:     "invalid - empty",
			currency: "",
			wantErr:  true,
			errMsg:   "currency is a required field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  tt.currency,
				Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
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

func TestCreateLimitInput_MaxAmountValidation(t *testing.T) {
	tests := []struct {
		name      string
		maxAmount decimal.Decimal
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid - positive amount",
			maxAmount: decimal.RequireFromString("1000"),
			wantErr:   false,
		},
		{
			name:      "valid - minimum positive (1)",
			maxAmount: decimal.RequireFromString("1"),
			wantErr:   false,
		},
		{
			name:      "valid - large amount",
			maxAmount: decimal.RequireFromString("9999999999.99"),
			wantErr:   false,
		},
		{
			name:      "invalid - zero",
			maxAmount: decimal.RequireFromString("0"),
			wantErr:   true,
			errMsg:    "maxAmount", // Required validation triggers first for zero value
		},
		{
			name:      "invalid - negative",
			maxAmount: decimal.RequireFromString("-1"),
			wantErr:   true,
			errMsg:    "maxAmount must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: tt.maxAmount,
				Currency:  "BRL",
				Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
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

func TestCreateLimitInput_ScopesMaxCount(t *testing.T) {
	t.Run("scopes at max count", func(t *testing.T) {
		scopes := make([]model.Scope, MaxLimitScopesCount)
		for i := range scopes {
			scopes[i] = model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		}

		input := CreateLimitInput{
			Name:      "Test Limit",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes:    scopes,
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("scopes exceed max count", func(t *testing.T) {
		scopes := make([]model.Scope, MaxLimitScopesCount+1)
		for i := range scopes {
			scopes[i] = model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		}

		input := CreateLimitInput{
			Name:      "Test Limit",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes:    scopes,
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scopes must have a maximum of 100 items")
	})
}

func TestLimitScopeInput_TransactionTypeValidation(t *testing.T) {
	validTypes := []model.TransactionType{
		model.TransactionTypeCard,
		model.TransactionTypeWire,
		model.TransactionTypePix,
		model.TransactionTypeCrypto,
	}

	for _, txType := range validTypes {
		t.Run("valid transactionType: "+string(txType), func(t *testing.T) {
			input := CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
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
		input := CreateLimitInput{
			Name:      "Test Limit",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes: []model.Scope{
				{TransactionType: &invalidType},
			},
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transactionType must be one of")
	})
}

func TestLimitScopeInput_SubTypeValidation(t *testing.T) {
	t.Run("valid subType within limit", func(t *testing.T) {
		input := CreateLimitInput{
			Name:      "Test Limit",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes: []model.Scope{
				{SubType: testutil.StringPtr("Credit")},
			},
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("subType at max length", func(t *testing.T) {
		input := CreateLimitInput{
			Name:      "Test Limit",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes: []model.Scope{
				{SubType: testutil.StringPtr(strings.Repeat("x", MaxLimitSubTypeLength))},
			},
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("subType too long", func(t *testing.T) {
		input := CreateLimitInput{
			Name:      "Test Limit",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes: []model.Scope{
				{SubType: testutil.StringPtr(strings.Repeat("x", MaxLimitSubTypeLength+1))},
			},
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subType must be a maximum of 50 characters")
	})
}

func TestCreateLimitInput_DescriptionValidation(t *testing.T) {
	t.Run("valid - nil description", func(t *testing.T) {
		input := CreateLimitInput{
			Name:        "Test Limit",
			Description: nil,
			LimitType:   model.LimitTypeDaily,
			MaxAmount:   decimal.RequireFromString("1000"),
			Currency:    "BRL",
			Scopes:      []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid - description at max length", func(t *testing.T) {
		input := CreateLimitInput{
			Name:        "Test Limit",
			Description: testutil.StringPtr(strings.Repeat("d", MaxLimitDescriptionLength)),
			LimitType:   model.LimitTypeDaily,
			MaxAmount:   decimal.RequireFromString("1000"),
			Currency:    "BRL",
			Scopes:      []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid - description too long", func(t *testing.T) {
		input := CreateLimitInput{
			Name:        "Test Limit",
			Description: testutil.StringPtr(strings.Repeat("d", MaxLimitDescriptionLength+1)),
			LimitType:   model.LimitTypeDaily,
			MaxAmount:   decimal.RequireFromString("1000"),
			Currency:    "BRL",
			Scopes:      []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "description must be a maximum of 1000 characters")
	})
}

func TestCreateLimitInput_NameValidation(t *testing.T) {
	t.Run("valid - name at max length", func(t *testing.T) {
		input := CreateLimitInput{
			Name:      strings.Repeat("n", MaxLimitNameLength),
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
		}
		err := input.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid - name too long", func(t *testing.T) {
		input := CreateLimitInput{
			Name:      strings.Repeat("n", MaxLimitNameLength+1),
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name must be a maximum of 255 characters")
	})

	t.Run("invalid - empty name", func(t *testing.T) {
		input := CreateLimitInput{
			Name:      "",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "BRL",
			Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(limitValidUUID1)}},
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is a required field")
	})
}

func TestUpdateLimitInput_ScopesMaxCount(t *testing.T) {
	t.Run("scopes at max count", func(t *testing.T) {
		scopes := make([]model.Scope, MaxLimitScopesCount)
		for i := range scopes {
			scopes[i] = model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		}

		input := UpdateLimitInput{Scopes: &scopes}
		err := input.Validate()
		require.NoError(t, err)
	})

	t.Run("scopes exceed max count", func(t *testing.T) {
		scopes := make([]model.Scope, MaxLimitScopesCount+1)
		for i := range scopes {
			scopes[i] = model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		}

		input := UpdateLimitInput{Scopes: &scopes}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scopes must have a maximum of 100 items")
	})
}

func TestFormatLimitValidationError_ScopeIndexExtraction(t *testing.T) {
	// Test that scope index is correctly extracted for error messages
	tests := []struct {
		name        string
		scopes      []model.Scope
		errContains string
	}{
		{
			name: "error at index 0",
			scopes: []model.Scope{
				{}, // Empty scope at index 0
				{AccountID: testutil.UUIDPtr(limitValidUUID1)},
			},
			errContains: "scope at index 0 must have at least one field set",
		},
		{
			name: "error at index 1",
			scopes: []model.Scope{
				{AccountID: testutil.UUIDPtr(limitValidUUID1)},
				{}, // Empty scope at index 1
			},
			errContains: "scope at index 1 must have at least one field set",
		},
		{
			name: "error at index 5",
			scopes: []model.Scope{
				{AccountID: testutil.UUIDPtr(limitValidUUID1)},
				{AccountID: testutil.UUIDPtr(limitValidUUID2)},
				{AccountID: testutil.UUIDPtr(limitValidUUID3)},
				{AccountID: testutil.UUIDPtr(limitValidUUID4)},
				{TransactionType: testutil.Ptr(model.TransactionTypeCard)},
				{}, // Empty scope at index 5
			},
			errContains: "scope at index 5 must have at least one field set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes:    tt.scopes,
			}
			err := input.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestValidateLimitType_PointerHandling(t *testing.T) {
	// Test validateLimitType handles pointer types correctly
	t.Run("nil pointer returns true (omitempty)", func(t *testing.T) {
		// This is tested indirectly via ListLimitsInput which uses string type
		limit := 10
		input := ListLimitsInput{
			Limit:     &limit,
			LimitType: "", // Empty means not set
		}
		err := input.Validate()
		assert.NoError(t, err)
	})
}

func TestListLimitsInput_ValidLimitTypeFilter(t *testing.T) {
	validTypes := []string{"DAILY", "WEEKLY", "MONTHLY", "CUSTOM", "PER_TRANSACTION"}

	for _, limitType := range validTypes {
		t.Run("valid limit_type filter: "+limitType, func(t *testing.T) {
			limit := 10
			input := ListLimitsInput{
				Limit:     &limit,
				LimitType: limitType,
			}
			err := input.Validate()
			assert.NoError(t, err)
		})
	}

	t.Run("invalid limit_type filter", func(t *testing.T) {
		limit := 10
		input := ListLimitsInput{
			Limit:     &limit,
			LimitType: "INVALID",
		}
		err := input.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "limit_type must be one of")

		var valErr *ValidationError
		require.ErrorAs(t, err, &valErr)
		assert.Equal(t, "TRC-0006", valErr.Code)
	})
}

func TestValidateLimitStatus_PointerHandling(t *testing.T) {
	// Test validateLimitStatus handles pointer types correctly
	t.Run("nil pointer returns true (omitempty)", func(t *testing.T) {
		limit := 10
		input := ListLimitsInput{
			Limit:  &limit,
			Status: "", // Empty means not set
		}
		err := input.Validate()
		assert.NoError(t, err)
	})
}

func TestListLimitsInput_ValidateScopeFields(t *testing.T) {
	tests := []struct {
		name        string
		input       ListLimitsInput
		expectError bool
		errContains string
		errCode     string
	}{
		{
			name: "valid - accountId UUID",
			input: ListLimitsInput{
				AccountID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440001"),
				Limit:     testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - segmentId UUID",
			input: ListLimitsInput{
				SegmentID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440002"),
				Limit:     testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - portfolioId UUID",
			input: ListLimitsInput{
				PortfolioID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440003"),
				Limit:       testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - merchantId UUID",
			input: ListLimitsInput{
				MerchantID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440004"),
				Limit:      testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - transactionType CARD",
			input: ListLimitsInput{
				TransactionType: testutil.StringPtr("CARD"),
				Limit:           testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - transactionType PIX",
			input: ListLimitsInput{
				TransactionType: testutil.StringPtr("PIX"),
				Limit:           testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - subType within limit",
			input: ListLimitsInput{
				SubType: testutil.StringPtr("Credit"),
				Limit:   testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - multiple scope fields combined",
			input: ListLimitsInput{
				AccountID:       testutil.StringPtr("550e8400-e29b-41d4-a716-446655440001"),
				TransactionType: testutil.StringPtr("WIRE"),
				Limit:           testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - name filter only",
			input: ListLimitsInput{
				Name:  testutil.StringPtr("Daily"),
				Limit: testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - name with scope filters",
			input: ListLimitsInput{
				Name:      testutil.StringPtr("Daily"),
				AccountID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440001"),
				Limit:     testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - no scope fields (backward compatible)",
			input: ListLimitsInput{
				Limit: testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "error - invalid accountId UUID",
			input: ListLimitsInput{
				AccountID: testutil.StringPtr("not-a-uuid"),
				Limit:     testutil.Ptr(10),
			},
			expectError: true,
			errContains: "account_id must be a valid UUID",
			errCode:     "TRC-0006",
		},
		{
			name: "error - invalid segmentId UUID",
			input: ListLimitsInput{
				SegmentID: testutil.StringPtr("invalid"),
				Limit:     testutil.Ptr(10),
			},
			expectError: true,
			errContains: "segment_id must be a valid UUID",
			errCode:     "TRC-0006",
		},
		{
			name: "error - invalid portfolioId UUID",
			input: ListLimitsInput{
				PortfolioID: testutil.StringPtr("bad-uuid"),
				Limit:       testutil.Ptr(10),
			},
			expectError: true,
			errContains: "portfolio_id must be a valid UUID",
			errCode:     "TRC-0006",
		},
		{
			name: "error - invalid merchantId UUID",
			input: ListLimitsInput{
				MerchantID: testutil.StringPtr("xyz"),
				Limit:      testutil.Ptr(10),
			},
			expectError: true,
			errContains: "merchant_id must be a valid UUID",
			errCode:     "TRC-0006",
		},
		{
			name: "error - invalid transactionType enum",
			input: ListLimitsInput{
				TransactionType: testutil.StringPtr("INVALID_TYPE"),
				Limit:           testutil.Ptr(10),
			},
			expectError: true,
			errContains: "transaction_type must be one of",
			errCode:     "TRC-0006",
		},
		{
			name: "error - subType exceeds max length",
			input: ListLimitsInput{
				SubType: testutil.StringPtr(strings.Repeat("x", MaxLimitSubTypeLength+1)),
				Limit:   testutil.Ptr(10),
			},
			expectError: true,
			errContains: "sub_type exceeds maximum length",
			errCode:     "TRC-0006",
		},
		{
			// Whitespace-only sub_type is treated as no filter by
			// buildLimitScopeFromInput, so the length check must trim before
			// rejecting. A 200-char whitespace-only value trims to "" and is
			// accepted (no filter applied), keeping validation symmetric with
			// the filter semantics.
			name: "valid - subType whitespace-only beyond max length is ignored",
			input: ListLimitsInput{
				SubType: testutil.StringPtr(strings.Repeat(" ", 200)),
				Limit:   testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - name at exact max length boundary (255 chars)",
			input: ListLimitsInput{
				Name:  testutil.StringPtr(strings.Repeat("a", MaxLimitNameFilterLength)),
				Limit: testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "error - name exceeds max length (256 chars)",
			input: ListLimitsInput{
				Name:  testutil.StringPtr(strings.Repeat("a", MaxLimitNameFilterLength+1)),
				Limit: testutil.Ptr(10),
			},
			expectError: true,
			errContains: "name filter exceeds maximum length",
			errCode:     "TRC-0006",
		},
		{
			name: "valid - name with LIKE special characters (escaped safely)",
			input: ListLimitsInput{
				Name:  testutil.StringPtr("100%_match"),
				Limit: testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - empty name string is accepted (no filter applied)",
			input: ListLimitsInput{
				Name:  testutil.StringPtr(""),
				Limit: testutil.Ptr(10),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.expectError {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				if tt.errCode != "" {
					var valErr *ValidationError
					require.ErrorAs(t, err, &valErr)
					assert.Equal(t, tt.errCode, valErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestToListLimitsFilter_SortOrderUppercase(t *testing.T) {
	tests := []struct {
		name          string
		inputOrder    string
		expectedOrder string
	}{
		{
			name:          "lowercase asc becomes ASC",
			inputOrder:    "asc",
			expectedOrder: "ASC",
		},
		{
			name:          "lowercase desc becomes DESC",
			inputOrder:    "desc",
			expectedOrder: "DESC",
		},
		{
			name:          "uppercase ASC stays ASC",
			inputOrder:    "ASC",
			expectedOrder: "ASC",
		},
		{
			name:          "uppercase DESC stays DESC",
			inputOrder:    "DESC",
			expectedOrder: "DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := 10
			input := &ListLimitsInput{
				Limit:     &limit,
				SortOrder: tt.inputOrder,
			}
			result := ToListLimitsFilter(input)
			assert.Equal(t, tt.expectedOrder, result.SortOrder)
		})
	}
}

func TestBuildLimitScopeFromInput(t *testing.T) {
	tests := []struct {
		name        string
		input       ListLimitsInput
		expectNil   bool
		checkFields func(t *testing.T, scope *model.Scope)
	}{
		{
			name:      "no scope fields returns nil",
			input:     ListLimitsInput{},
			expectNil: true,
		},
		{
			name: "empty string fields returns nil",
			input: ListLimitsInput{
				AccountID: testutil.StringPtr(""),
			},
			expectNil: true,
		},
		{
			name: "accountId only",
			input: ListLimitsInput{
				AccountID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440001"),
			},
			expectNil: false,
			checkFields: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.AccountID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", scope.AccountID.String())
				assert.Nil(t, scope.SegmentID)
				assert.Nil(t, scope.PortfolioID)
				assert.Nil(t, scope.MerchantID)
				assert.Nil(t, scope.TransactionType)
				assert.Nil(t, scope.SubType)
			},
		},
		{
			name: "transactionType only",
			input: ListLimitsInput{
				TransactionType: testutil.StringPtr("CARD"),
			},
			expectNil: false,
			checkFields: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.TransactionType)
				assert.Equal(t, model.TransactionTypeCard, *scope.TransactionType)
				assert.Nil(t, scope.AccountID)
			},
		},
		{
			name: "subType only",
			input: ListLimitsInput{
				SubType: testutil.StringPtr("Credit"),
			},
			expectNil: false,
			checkFields: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.SubType)
				require.Equal(t, "credit", *scope.SubType)
			},
		},
		{
			name: "portfolioId only",
			input: ListLimitsInput{
				PortfolioID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440003"),
			},
			expectNil: false,
			checkFields: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.PortfolioID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440003", scope.PortfolioID.String())
				assert.Nil(t, scope.AccountID)
				assert.Nil(t, scope.SegmentID)
				assert.Nil(t, scope.MerchantID)
			},
		},
		{
			name: "merchantId only",
			input: ListLimitsInput{
				MerchantID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440004"),
			},
			expectNil: false,
			checkFields: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.MerchantID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440004", scope.MerchantID.String())
				assert.Nil(t, scope.AccountID)
				assert.Nil(t, scope.PortfolioID)
				assert.Nil(t, scope.SegmentID)
			},
		},
		{
			name: "segmentId only",
			input: ListLimitsInput{
				SegmentID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440005"),
			},
			expectNil: false,
			checkFields: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.SegmentID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440005", scope.SegmentID.String())
				assert.Nil(t, scope.AccountID)
				assert.Nil(t, scope.PortfolioID)
				assert.Nil(t, scope.MerchantID)
			},
		},
		{
			name: "multiple fields combined",
			input: ListLimitsInput{
				AccountID:       testutil.StringPtr("550e8400-e29b-41d4-a716-446655440001"),
				SegmentID:       testutil.StringPtr("550e8400-e29b-41d4-a716-446655440002"),
				TransactionType: testutil.StringPtr("WIRE"),
				SubType:         testutil.StringPtr("International"),
			},
			expectNil: false,
			checkFields: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope.AccountID)
				require.NotNil(t, scope.SegmentID)
				require.NotNil(t, scope.TransactionType)
				require.NotNil(t, scope.SubType)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", scope.AccountID.String())
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440002", scope.SegmentID.String())
				assert.Equal(t, model.TransactionTypeWire, *scope.TransactionType)
				require.Equal(t, "international", *scope.SubType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope := buildLimitScopeFromInput(&tt.input)
			if tt.expectNil {
				assert.Nil(t, scope)
			} else {
				require.NotNil(t, scope)
				if tt.checkFields != nil {
					tt.checkFields(t, scope)
				}
			}
		})
	}
}

func TestToListLimitsFilter_WithNameAndScopeFields(t *testing.T) {
	t.Run("name filter is passed through", func(t *testing.T) {
		input := &ListLimitsInput{
			Name:  testutil.StringPtr("Daily"),
			Limit: testutil.Ptr(10),
		}
		filter := ToListLimitsFilter(input)
		require.NotNil(t, filter.Name)
		assert.Equal(t, "Daily", *filter.Name)
		assert.Nil(t, filter.ScopeFilter)
	})

	t.Run("scope filter is built from fields", func(t *testing.T) {
		input := &ListLimitsInput{
			AccountID: testutil.StringPtr("550e8400-e29b-41d4-a716-446655440001"),
			Limit:     testutil.Ptr(10),
		}
		filter := ToListLimitsFilter(input)
		require.NotNil(t, filter.ScopeFilter)
		require.NotNil(t, filter.ScopeFilter.AccountID)
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", filter.ScopeFilter.AccountID.String())
	})

	t.Run("no scope fields means nil ScopeFilter", func(t *testing.T) {
		input := &ListLimitsInput{
			Status: "ACTIVE",
			Limit:  testutil.Ptr(10),
		}
		filter := ToListLimitsFilter(input)
		assert.Nil(t, filter.ScopeFilter)
		require.NotNil(t, filter.Status)
		assert.Equal(t, model.LimitStatusActive, *filter.Status)
	})

	t.Run("name and scope combined", func(t *testing.T) {
		input := &ListLimitsInput{
			Name:            testutil.StringPtr("Monthly"),
			TransactionType: testutil.StringPtr("PIX"),
			Status:          "ACTIVE",
			Limit:           testutil.Ptr(20),
		}
		filter := ToListLimitsFilter(input)
		require.NotNil(t, filter.Name)
		assert.Equal(t, "Monthly", *filter.Name)
		require.NotNil(t, filter.ScopeFilter)
		require.NotNil(t, filter.ScopeFilter.TransactionType)
		assert.Equal(t, model.TransactionTypePix, *filter.ScopeFilter.TransactionType)
		require.NotNil(t, filter.Status)
		assert.Equal(t, model.LimitStatusActive, *filter.Status)
		assert.Equal(t, 20, filter.Limit)
	})
}

func TestLimitValidation_NormalizesSubType(t *testing.T) {
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
			input:    testutil.StringPtr("Credit"),
			expected: testutil.StringPtr("credit"),
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
			input := &ListLimitsInput{SubType: tc.input}

			scope := buildLimitScopeFromInput(input)

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
