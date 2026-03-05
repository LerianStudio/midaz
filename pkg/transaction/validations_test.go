// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/lib-commons/v4/commons"
	constant "github.com/LerianStudio/lib-commons/v4/commons/constants"
	"github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
)

func TestValidateBalancesRules(t *testing.T) {
	t.Parallel()

	// Create a context with logger and tracer
	ctx := context.Background()
	logger := &log.GoLogger{Level: log.LevelInfo}
	ctx = commons.ContextWithLogger(ctx, logger)
	tracer := otel.Tracer("test")
	ctx = commons.ContextWithTracer(ctx, tracer)

	tests := []struct {
		name        string
		transaction Transaction
		validate    Responses
		balances    []*Balance
		expectError bool
		errorCode   string
	}{
		{
			name: "valid balances - simple transfer",
			transaction: Transaction{
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{AccountAlias: "@account1"},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{AccountAlias: "@account2"},
						},
					},
				},
			},
			validate: Responses{
				Asset: "USD",
				From: map[string]Amount{
					"0#@account1#default": {Value: decimal.NewFromInt(100), Operation: constant.DEBIT, TransactionType: constant.CREATED},
				},
				To: map[string]Amount{
					"0#@account2#default": {Value: decimal.NewFromInt(100), Operation: constant.CREDIT, TransactionType: constant.CREATED},
				},
			},
			balances: []*Balance{
				{
					ID:             "123",
					Alias:          "@account1",
					Key:            "default",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(200),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
					AllowReceiving: true,
					AccountType:    "internal",
				},
				{
					ID:             "456",
					Alias:          "@account2",
					Key:            "default",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(50),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
					AllowReceiving: true,
					AccountType:    "internal",
				},
			},
			expectError: false,
		},
		{
			name:        "invalid - wrong number of balances",
			transaction: Transaction{},
			validate: Responses{
				From: map[string]Amount{
					"0#@account1#default": {Value: decimal.NewFromInt(100), Operation: constant.DEBIT, TransactionType: constant.CREATED},
				},
				To: map[string]Amount{
					"0#@account2#default": {Value: decimal.NewFromInt(100), Operation: constant.CREDIT, TransactionType: constant.CREATED},
				},
			},
			balances:    []*Balance{}, // Empty balances
			expectError: true,
			errorCode:   "0019", // ErrAccountIneligibility
		},
		{
			name:        "invalid - nil balance slice",
			transaction: Transaction{},
			validate: Responses{
				From: map[string]Amount{
					"0#@account1#default": {Value: decimal.NewFromInt(100), Operation: constant.DEBIT, TransactionType: constant.CREATED},
				},
				To: map[string]Amount{
					"0#@account2#default": {Value: decimal.NewFromInt(100), Operation: constant.CREDIT, TransactionType: constant.CREATED},
				},
			},
			balances:    nil,
			expectError: true,
			errorCode:   "0019", // ErrAccountIneligibility
		},
		{
			name: "invalid - sending not allowed on from balance",
			transaction: Transaction{
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{AccountAlias: "@account1"},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{AccountAlias: "@account2"},
						},
					},
				},
			},
			validate: Responses{
				Asset: "USD",
				From: map[string]Amount{
					"0#@account1#default": {Value: decimal.NewFromInt(100), Operation: constant.DEBIT, TransactionType: constant.CREATED},
				},
				To: map[string]Amount{
					"0#@account2#default": {Value: decimal.NewFromInt(100), Operation: constant.CREDIT, TransactionType: constant.CREATED},
				},
			},
			balances: []*Balance{
				{
					ID:             "123",
					Alias:          "@account1",
					Key:            "default",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(200),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   false, // Sending not allowed
					AllowReceiving: true,
					AccountType:    "internal",
				},
				{
					ID:             "456",
					Alias:          "@account2",
					Key:            "default",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(50),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
					AllowReceiving: true,
					AccountType:    "internal",
				},
			},
			expectError: true,
			errorCode:   "0024", // ErrAccountStatusTransactionRestriction
		},
		{
			name: "invalid - asset mismatch in from balance",
			transaction: Transaction{
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{AccountAlias: "@account1"},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{AccountAlias: "@account2"},
						},
					},
				},
			},
			validate: Responses{
				Asset: "USD",
				From: map[string]Amount{
					"0#@account1#default": {Value: decimal.NewFromInt(100), Operation: constant.DEBIT, TransactionType: constant.CREATED},
				},
				To: map[string]Amount{
					"0#@account2#default": {Value: decimal.NewFromInt(100), Operation: constant.CREDIT, TransactionType: constant.CREATED},
				},
			},
			balances: []*Balance{
				{
					ID:             "123",
					Alias:          "@account1",
					Key:            "default",
					AssetCode:      "EUR", // Wrong asset
					Available:      decimal.NewFromInt(200),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
					AllowReceiving: true,
					AccountType:    "internal",
				},
				{
					ID:             "456",
					Alias:          "@account2",
					Key:            "default",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(50),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
					AllowReceiving: true,
					AccountType:    "internal",
				},
			},
			expectError: true,
			errorCode:   "0034", // ErrAssetCodeNotFound
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateBalancesRules(ctx, tt.transaction, tt.validate, tt.balances)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorCode != "" {
					// Check if the error is a Response type and contains the error code
					if respErr, ok := err.(commons.Response); ok {
						assert.Equal(t, tt.errorCode, respErr.Code)
					} else {
						assert.Contains(t, err.Error(), tt.errorCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFromBalances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		balance     *Balance
		from        map[string]Amount
		asset       string
		expectError bool
		errorCode   string
	}{
		{
			name: "valid from balance",
			balance: &Balance{
				ID:           "123",
				Alias:        "@account1",
				Key:          "default",
				AssetCode:    "USD",
				Available:    decimal.NewFromInt(100),
				AllowSending: true,
				AccountType:  "internal",
			},
			from: map[string]Amount{
				"0#@account1#default": {Value: decimal.NewFromInt(50)},
			},
			asset:       "USD",
			expectError: false,
		},
		{
			name: "invalid - wrong asset code",
			balance: &Balance{
				ID:           "123",
				Alias:        "@account1",
				Key:          "default",
				AssetCode:    "EUR",
				Available:    decimal.NewFromInt(100),
				AllowSending: true,
				AccountType:  "internal",
			},
			from: map[string]Amount{
				"0#@account1#default": {Value: decimal.NewFromInt(50)},
			},
			asset:       "USD",
			expectError: true,
			errorCode:   "0034", // ErrAssetCodeNotFound
		},
		{
			name: "invalid - sending not allowed",
			balance: &Balance{
				ID:           "123",
				Alias:        "@account1",
				Key:          "default",
				AssetCode:    "USD",
				Available:    decimal.NewFromInt(100),
				AllowSending: false,
				AccountType:  "internal",
			},
			from: map[string]Amount{
				"0#@account1#default": {Value: decimal.NewFromInt(50)},
			},
			asset:       "USD",
			expectError: true,
			errorCode:   "0024", // ErrAccountStatusTransactionRestriction
		},
		{
			name: "valid - external account with zero balance",
			balance: &Balance{
				ID:           "123",
				Alias:        "@external",
				Key:          "default",
				AssetCode:    "USD",
				Available:    decimal.NewFromInt(0),
				AllowSending: true,
				AccountType:  constant.ExternalAccountType,
			},
			from: map[string]Amount{
				"0#@external#default": {Value: decimal.NewFromInt(50)},
			},
			asset:       "USD",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateFromBalances(tt.balance, tt.from, tt.asset, false)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorCode != "" {
					// Check if the error is a Response type and contains the error code
					if respErr, ok := err.(commons.Response); ok {
						assert.Equal(t, tt.errorCode, respErr.Code)
					} else {
						assert.Contains(t, err.Error(), tt.errorCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToBalances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		balance     *Balance
		to          map[string]Amount
		asset       string
		expectError bool
		errorCode   string
	}{
		{
			name: "valid to balance",
			balance: &Balance{
				ID:             "123",
				Alias:          "@account1",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(100),
				AllowReceiving: true,
				AccountType:    "internal",
			},
			to: map[string]Amount{
				"0#@account1#default": {Value: decimal.NewFromInt(50)},
			},
			asset:       "USD",
			expectError: false,
		},
		{
			name: "invalid - wrong asset code",
			balance: &Balance{
				ID:             "123",
				Alias:          "@account1",
				Key:            "default",
				AssetCode:      "EUR",
				Available:      decimal.NewFromInt(100),
				AllowReceiving: true,
				AccountType:    "internal",
			},
			to: map[string]Amount{
				"0#@account1#default": {Value: decimal.NewFromInt(50)},
			},
			asset:       "USD",
			expectError: true,
			errorCode:   "0034", // ErrAssetCodeNotFound
		},
		{
			name: "invalid - receiving not allowed",
			balance: &Balance{
				ID:             "123",
				Alias:          "@account1",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(100),
				AllowReceiving: false,
				AccountType:    "internal",
			},
			to: map[string]Amount{
				"0#@account1#default": {Value: decimal.NewFromInt(50)},
			},
			asset:       "USD",
			expectError: true,
			errorCode:   "0024", // ErrAccountStatusTransactionRestriction
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateToBalances(tt.balance, tt.to, tt.asset)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorCode != "" {
					// Check if the error is a Response type and contains the error code
					if respErr, ok := err.(commons.Response); ok {
						assert.Equal(t, tt.errorCode, respErr.Code)
					} else {
						assert.Contains(t, err.Error(), tt.errorCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOperateBalances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		amount      Amount
		balance     Balance
		operation   string
		expected    Balance
		expectError bool
	}{
		{
			name: "debit operation - CREATED",
			amount: Amount{
				Value:           decimal.NewFromInt(50),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
			balance: Balance{
				Available: decimal.NewFromInt(100),
				OnHold:    decimal.NewFromInt(10),
			},
			expected: Balance{
				Available: decimal.NewFromInt(50), // 100 - 50 = 50
				OnHold:    decimal.NewFromInt(10),
			},
			expectError: false,
		},
		{
			name: "credit operation - CREATED",
			amount: Amount{
				Value:           decimal.NewFromInt(50),
				Operation:       constant.CREDIT,
				TransactionType: constant.CREATED,
			},
			balance: Balance{
				Available: decimal.NewFromInt(100),
				OnHold:    decimal.NewFromInt(10),
			},
			expected: Balance{
				Available: decimal.NewFromInt(150), // 100 + 50 = 150
				OnHold:    decimal.NewFromInt(10),
			},
			expectError: false,
		},
		{
			name: "debit operation - APPROVED (releases hold only)",
			amount: Amount{
				Value:           decimal.NewFromInt(50),
				Operation:       constant.DEBIT,
				TransactionType: constant.APPROVED,
			},
			balance: Balance{
				Available: decimal.NewFromInt(100),
				OnHold:    decimal.NewFromInt(50),
			},
			expected: Balance{
				Available: decimal.NewFromInt(100), // No change to available
				OnHold:    decimal.NewFromInt(0),   // 50 - 50 = 0 (released from hold)
			},
			expectError: false,
		},
		{
			name: "credit operation - APPROVED (adds to available)",
			amount: Amount{
				Value:           decimal.NewFromInt(30),
				Operation:       constant.CREDIT,
				TransactionType: constant.APPROVED,
			},
			balance: Balance{
				Available: decimal.NewFromInt(100),
				OnHold:    decimal.NewFromInt(0),
			},
			expected: Balance{
				Available: decimal.NewFromInt(130), // 100 + 30 = 130
				OnHold:    decimal.NewFromInt(0),   // No change
			},
			expectError: false,
		},
		{
			name: "release operation - CANCELED (restores available, releases hold)",
			amount: Amount{
				Value:           decimal.NewFromInt(50),
				Operation:       constant.RELEASE,
				TransactionType: constant.CANCELED,
			},
			balance: Balance{
				Available: decimal.NewFromInt(50),
				OnHold:    decimal.NewFromInt(50),
			},
			expected: Balance{
				Available: decimal.NewFromInt(100), // 50 + 50 = 100 (restored)
				OnHold:    decimal.NewFromInt(0),   // 50 - 50 = 0
			},
			expectError: false,
		},
		{
			name: "onhold operation - PENDING (moves to hold)",
			amount: Amount{
				Value:           decimal.NewFromInt(30),
				Operation:       constant.ONHOLD,
				TransactionType: constant.PENDING,
			},
			balance: Balance{
				Available: decimal.NewFromInt(100),
				OnHold:    decimal.NewFromInt(0),
			},
			expected: Balance{
				Available: decimal.NewFromInt(70), // 100 - 30 = 70
				OnHold:    decimal.NewFromInt(30), // 0 + 30 = 30
			},
			expectError: false,
		},
		{
			name: "unknown operation - returns balance unchanged without version increment",
			amount: Amount{
				Value:           decimal.NewFromInt(50),
				Operation:       "UNKNOWN",
				TransactionType: "UNKNOWN",
			},
			balance: Balance{
				Available: decimal.NewFromInt(100),
				OnHold:    decimal.NewFromInt(10),
				Version:   5,
			},
			expected: Balance{
				Available: decimal.NewFromInt(100), // unchanged
				OnHold:    decimal.NewFromInt(10),  // unchanged
				Version:   5,                       // unchanged (no increment for unknown ops)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := OperateBalances(tt.amount, tt.balance)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Available.String(), result.Available.String())
				assert.Equal(t, tt.expected.OnHold.String(), result.OnHold.String())

				// For unknown operation, verify version is unchanged
				if tt.expected.Version > 0 {
					assert.Equal(t, tt.expected.Version, result.Version,
						"version should match expected value")
				}
			}
		})
	}
}

func TestAliasKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		alias      string
		balanceKey string
		want       string
	}{
		{
			name:       "alias with balance key",
			alias:      "@person1",
			balanceKey: "savings",
			want:       "@person1#savings",
		},
		{
			name:       "alias with empty balance key defaults to 'default'",
			alias:      "@person1",
			balanceKey: "",
			want:       "@person1#default",
		},
		{
			name:       "alias with special characters and balance key",
			alias:      "@external/BRL",
			balanceKey: "checking",
			want:       "@external/BRL#checking",
		},
		{
			name:       "empty alias with balance key",
			alias:      "",
			balanceKey: "current",
			want:       "#current",
		},
		{
			name:       "empty alias with empty balance key",
			alias:      "",
			balanceKey: "",
			want:       "#default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := AliasKey(tt.alias, tt.balanceKey)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSplitAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alias string
		want  string
	}{
		{
			name:  "alias without index",
			alias: "@person1",
			want:  "@person1",
		},
		{
			name:  "alias with index",
			alias: "1#@person1",
			want:  "@person1",
		},
		{
			name:  "alias with zero index",
			alias: "0#@person1",
			want:  "@person1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SplitAlias(tt.alias)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConcatAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		index int
		alias string
		want  string
	}{
		{
			name:  "concat with positive index",
			index: 1,
			alias: "@person1",
			want:  "1#@person1",
		},
		{
			name:  "concat with zero index",
			index: 0,
			alias: "@person2",
			want:  "0#@person2",
		},
		{
			name:  "concat with large index",
			index: 999,
			alias: "@person3",
			want:  "999#@person3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ConcatAlias(tt.index, tt.alias)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAppendIfNotExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		slice []string
		s     []string
		want  []string
	}{
		{
			name:  "append new elements",
			slice: []string{"a", "b"},
			s:     []string{"c", "d"},
			want:  []string{"a", "b", "c", "d"},
		},
		{
			name:  "skip existing elements",
			slice: []string{"a", "b"},
			s:     []string{"b", "c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "all elements exist",
			slice: []string{"a", "b", "c"},
			s:     []string{"a", "b"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "empty initial slice",
			slice: []string{},
			s:     []string{"a", "b"},
			want:  []string{"a", "b"},
		},
		{
			name:  "empty append slice",
			slice: []string{"a", "b"},
			s:     []string{},
			want:  []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := AppendIfNotExist(tt.slice, tt.s)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateSendSourceAndDistribute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		transaction Transaction
		want        *Responses
		expectError bool
		errorCode   string
	}{
		{
			name: "valid - simple source and distribute",
			transaction: Transaction{
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@account1",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@account2",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
				},
			},
			expectError: false, // Now expects success after fixing CalculateTotal
		},
		{
			name: "valid - multiple sources and distributes",
			transaction: Transaction{
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@account1",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(50),
								},
							},
							{
								AccountAlias: "@account2",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(50),
								},
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@account3",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(60),
								},
							},
							{
								AccountAlias: "@account4",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(40),
								},
							},
						},
					},
				},
			},
			expectError: false, // Now expects success after fixing CalculateTotal
		},
		{
			name: "valid transaction with shares",
			transaction: Transaction{
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@account1",
								Share: &Share{
									Percentage: 60,
								},
							},
							{
								AccountAlias: "@account2",
								Share: &Share{
									Percentage: 40,
								},
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@account3",
								Share: &Share{
									Percentage: 100,
								},
							},
						},
					},
				},
			},
			want: &Responses{
				Asset: "USD",
				From: map[string]Amount{
					"@account1": {Value: decimal.NewFromInt(60)},
					"@account2": {Value: decimal.NewFromInt(40)},
				},
				To: map[string]Amount{
					"@account3": {Value: decimal.NewFromInt(100)},
				},
			},
			expectError: false,
		},
		{
			name: "valid transaction with remains",
			transaction: Transaction{
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@account1",
								Share: &Share{
									Percentage: 50,
								},
								IsFrom: true,
							},
							{
								AccountAlias: "@account2",
								Remaining:    "remaining",
								IsFrom:       true,
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@account3",
								Remaining:    "remaining",
							},
						},
					},
				},
			},
			want: &Responses{
				Asset: "USD",
				From: map[string]Amount{
					"@account1": {Value: decimal.NewFromInt(50)},
					"@account2": {Value: decimal.NewFromInt(50)},
				},
				To: map[string]Amount{
					"@account3": {Value: decimal.NewFromInt(100)},
				},
			},
			expectError: false,
		},
		{
			name: "invalid - total mismatch",
			transaction: Transaction{
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@account1",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(60),
								},
							},
							{
								AccountAlias: "@account2",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(30), // Total is 90, not 100
								},
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@account3",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorCode:   "0073", // ErrTransactionValueMismatch
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			got, err := ValidateSendSourceAndDistribute(ctx, tt.transaction, constant.CREATED)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorCode != "" {
					// Check if the error is a Response type and contains the error code
					if respErr, ok := err.(commons.Response); ok {
						assert.Equal(t, tt.errorCode, respErr.Code)
					} else {
						assert.Contains(t, err.Error(), tt.errorCode)
					}
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)
				if tt.want != nil && got != nil {
					assert.Equal(t, tt.want.Asset, got.Asset)
					assert.Equal(t, len(tt.want.From), len(got.From))
					assert.Equal(t, len(tt.want.To), len(got.To))

					// Assert Amount.Value for each key in From map
					for key, wantAmount := range tt.want.From {
						gotAmount, exists := got.From[key]
						assert.True(t, exists, "From map should contain key %s", key)
						if exists {
							assert.True(t, wantAmount.Value.Equal(gotAmount.Value),
								"From[%s].Value: want=%s got=%s", key, wantAmount.Value, gotAmount.Value)
						}
					}

					// Assert Amount.Value for each key in To map
					for key, wantAmount := range tt.want.To {
						gotAmount, exists := got.To[key]
						assert.True(t, exists, "To map should contain key %s", key)
						if exists {
							assert.True(t, wantAmount.Value.Equal(gotAmount.Value),
								"To[%s].Value: want=%s got=%s", key, wantAmount.Value, gotAmount.Value)
						}
					}
				}
			}
		})
	}
}

func TestValidateTransactionWithPercentageAndRemaining(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		transaction Transaction
		expectError bool
		errorCode   string
	}{
		{
			name: "valid transaction with percentage and remaining",
			transaction: Transaction{
				ChartOfAccountsGroupName: "PAG_CONTAS_CODE_1",
				Description:              "description for the transaction person1 to person2 value of 100 reais",
				Metadata: map[string]interface{}{
					"depositType": "PIX",
					"valor":       "100.00",
				},
				Pending: false,
				Route:   "00000000-0000-0000-0000-000000000000",
				Send: Send{
					Asset: "BRL",
					Value: decimal.NewFromFloat(100.00),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@external/BRL",
								Remaining:    "remaining",
								Description:  "Loan payment 1",
								Route:        "00000000-0000-0000-0000-000000000000",
								Metadata: map[string]interface{}{
									"1":   "m",
									"Cpf": "43049498x",
								},
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@mcgregor_0",
								Share: &Share{
									Percentage: 50,
								},
								Route: "00000000-0000-0000-0000-000000000000",
								Metadata: map[string]interface{}{
									"mensagem": "tks",
								},
							},
							{
								AccountAlias: "@mcgregor_1",
								Share: &Share{
									Percentage: 50,
								},
								Description: "regression test",
								Metadata: map[string]interface{}{
									"key": "value",
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "transaction with value mismatch",
			transaction: Transaction{
				ChartOfAccountsGroupName: "PAG_CONTAS_CODE_1",
				Description:              "transaction with value mismatch",
				Pending:                  false,
				Send: Send{
					Asset: "BRL",
					Value: decimal.NewFromFloat(100.00),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@external/BRL",
								Amount: &Amount{
									Asset: "BRL",
									// Source amount doesn't match transaction value
									Value: decimal.NewFromFloat(90.00),
								},
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@mcgregor_0",
								Share: &Share{
									Percentage: 100,
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorCode:   "0073", // ErrTransactionValueMismatch
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			// Call ValidateSendSourceAndDistribute to get the responses
			responses, err := ValidateSendSourceAndDistribute(ctx, tt.transaction, constant.CREATED)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorCode != "" {
					errMsg := err.Error()
					assert.Contains(t, errMsg, tt.errorCode, "Error should contain the expected error code")
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, responses)

			// For successful case, validate response structure
			assert.Equal(t, tt.transaction.Send.Value, responses.Total)
			assert.Equal(t, tt.transaction.Send.Asset, responses.Asset)

			// Verify the source account is included in the response
			fromKey := "@external/BRL"
			_, exists := responses.From[fromKey]
			assert.True(t, exists, "From account should exist: %s", fromKey)

			// Verify the destination accounts are included in the response
			toKey1 := "@mcgregor_0"
			_, exists = responses.To[toKey1]
			assert.True(t, exists, "To account should exist: %s", toKey1)

			toKey2 := "@mcgregor_1"
			_, exists = responses.To[toKey2]
			assert.True(t, exists, "To account should exist: %s", toKey2)

			// Verify total amount is correctly distributed
			var total decimal.Decimal
			for _, amount := range responses.To {
				total = total.Add(amount.Value)
			}
			assert.True(t, responses.Total.Equal(total),
				"Total amount (%s) should equal sum of destination amounts (%s)",
				responses.Total.String(), total.String())
		})
	}
}

func TestDetermineOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		isPending         bool
		isFrom            bool
		transactionType   string
		expectedType      string
		expectedDirection string
	}{
		{
			name:              "pending from PENDING -> ONHOLD credit",
			isPending:         true,
			isFrom:            true,
			transactionType:   constant.PENDING,
			expectedType:      constant.ONHOLD,
			expectedDirection: constant.CREDIT,
		},
		{
			name:              "pending to PENDING -> CREDIT credit",
			isPending:         true,
			isFrom:            false,
			transactionType:   constant.PENDING,
			expectedType:      constant.CREDIT,
			expectedDirection: constant.CREDIT,
		},
		{
			name:              "pending from CANCELED -> RELEASE debit",
			isPending:         true,
			isFrom:            true,
			transactionType:   constant.CANCELED,
			expectedType:      constant.RELEASE,
			expectedDirection: constant.DEBIT,
		},
		{
			name:              "pending from APPROVED -> DEBIT debit",
			isPending:         true,
			isFrom:            true,
			transactionType:   constant.APPROVED,
			expectedType:      constant.DEBIT,
			expectedDirection: constant.DEBIT,
		},
		{
			name:              "pending to APPROVED -> CREDIT credit",
			isPending:         true,
			isFrom:            false,
			transactionType:   constant.APPROVED,
			expectedType:      constant.CREDIT,
			expectedDirection: constant.CREDIT,
		},
		{
			name:              "not pending from -> DEBIT debit",
			isPending:         false,
			isFrom:            true,
			transactionType:   constant.CREATED,
			expectedType:      constant.DEBIT,
			expectedDirection: constant.DEBIT,
		},
		{
			name:              "not pending to -> CREDIT credit",
			isPending:         false,
			isFrom:            false,
			transactionType:   constant.CREATED,
			expectedType:      constant.CREDIT,
			expectedDirection: constant.CREDIT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotType, gotDirection := DetermineOperation(tt.isPending, tt.isFrom, tt.transactionType)
			assert.Equal(t, tt.expectedType, gotType, "operation type mismatch")
			assert.Equal(t, tt.expectedDirection, gotDirection, "direction mismatch")
		})
	}
}

func TestOperateBalances_RouteValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		amount   Amount
		balance  Balance
		expected Balance
	}{
		{
			name: "ONHOLD+PENDING flag OFF - Available-- AND OnHold++ (current behavior)",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: false,
			},
			balance: Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(0),
			},
			expected: Balance{
				Available: decimal.NewFromInt(900),
				OnHold:    decimal.NewFromInt(100),
				Version:   1,
			},
		},
		{
			name: "DEBIT+PENDING flag ON - Available-- only with version+1",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.DEBIT,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(0),
			},
			expected: Balance{
				Available: decimal.NewFromInt(900),
				OnHold:    decimal.NewFromInt(0),
				Version:   1,
			},
		},
		{
			name: "ONHOLD+PENDING flag ON - OnHold++ only with version+1",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(0),
			},
			expected: Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(100),
				Version:   1,
			},
		},
		{
			name: "DEBIT+CREATED flag OFF - Available-- (regression)",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.DEBIT,
				TransactionType:        constant.CREATED,
				RouteValidationEnabled: false,
			},
			balance: Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(50),
			},
			expected: Balance{
				Available: decimal.NewFromInt(900),
				OnHold:    decimal.NewFromInt(50),
				Version:   1,
			},
		},
		{
			name: "RELEASE+CANCELED flag OFF - OnHold-- AND Available++ (regression)",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: false,
			},
			balance: Balance{
				Available: decimal.NewFromInt(900),
				OnHold:    decimal.NewFromInt(100),
			},
			expected: Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(0),
				Version:   1,
			},
		},
		{
			name: "RELEASE+CANCELED flag ON - OnHold-- only with version+1",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(900),
				OnHold:    decimal.NewFromInt(100),
			},
			expected: Balance{
				Available: decimal.NewFromInt(900),
				OnHold:    decimal.NewFromInt(0),
				Version:   1,
			},
		},
		{
			name: "CREDIT+CANCELED flag ON - Available++ only with version+1",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.CREDIT,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(900),
				OnHold:    decimal.NewFromInt(0),
			},
			expected: Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(0),
				Version:   1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := OperateBalances(tt.amount, tt.balance)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Available.String(), result.Available.String(), "available balance mismatch")
			assert.Equal(t, tt.expected.OnHold.String(), result.OnHold.String(), "onHold balance mismatch")
			assert.Equal(t, tt.expected.Version, result.Version, "version mismatch")
		})
	}
}

func TestDoubleEntryInvariant_RouteValidation(t *testing.T) {
	t.Parallel()

	// When route validation is ON and transaction is PENDING, two separate
	// operations are applied: DEBIT (Available-- only, version+1) then
	// ON_HOLD (OnHold++ only, version+1). Combined effect: version+2.

	value := decimal.NewFromInt(500)
	startBalance := Balance{
		Available: decimal.NewFromInt(2000),
		OnHold:    decimal.NewFromInt(0),
		Version:   5,
	}

	// Step 1: DEBIT (debit side of pending double-entry) — Available-- only
	debitAmount := Amount{
		Value:                  value,
		Operation:              constant.DEBIT,
		TransactionType:        constant.PENDING,
		RouteValidationEnabled: true,
	}

	afterDebit, err := OperateBalances(debitAmount, startBalance)
	assert.NoError(t, err)

	assert.True(t, afterDebit.Available.Equal(decimal.NewFromInt(1500)),
		"Available should decrease by value: got %s", afterDebit.Available)
	assert.True(t, afterDebit.OnHold.Equal(decimal.NewFromInt(0)),
		"OnHold should NOT change during DEBIT with flag ON: got %s", afterDebit.OnHold)
	assert.Equal(t, int64(6), afterDebit.Version,
		"version should increment by 1 for DEBIT operation")

	// Step 2: ON_HOLD (credit side of pending double-entry) — OnHold++ only
	onholdAmount := Amount{
		Value:                  value,
		Operation:              constant.ONHOLD,
		TransactionType:        constant.PENDING,
		RouteValidationEnabled: true,
	}

	afterOnHold, err := OperateBalances(onholdAmount, afterDebit)
	assert.NoError(t, err)

	assert.True(t, afterOnHold.Available.Equal(decimal.NewFromInt(1500)),
		"Available should NOT change during ON_HOLD with flag ON: got %s", afterOnHold.Available)
	assert.True(t, afterOnHold.OnHold.Equal(decimal.NewFromInt(500)),
		"OnHold should increase by value: got %s", afterOnHold.OnHold)
	assert.Equal(t, int64(7), afterOnHold.Version,
		"version should increment by 1 for ON_HOLD operation")

	// Double-entry invariant: combined debit effect == combined credit effect
	availableDecrease := startBalance.Available.Sub(afterOnHold.Available)
	onholdIncrease := afterOnHold.OnHold.Sub(startBalance.OnHold)
	assert.True(t, availableDecrease.Equal(onholdIncrease), "debit effect must equal credit effect")
}

func TestDoubleEntryInvariant_CanceledRouteValidation(t *testing.T) {
	t.Parallel()

	// When route validation is ON and transaction is CANCELED, OperateBalances
	// processes RELEASE (OnHold-- only, version+1), then a separate CREDIT
	// operation adds to Available (version+1). Each operation increments by 1.
	// BuildOperations creates 2 operation records: RELEASE(debit) + CREDIT(credit).

	value := decimal.NewFromInt(500)
	startBalance := Balance{
		Available: decimal.NewFromInt(1500),
		OnHold:    decimal.NewFromInt(500),
		Version:   7, // continuing from where PENDING left off
	}

	// Step 1: RELEASE (debit side of canceled double-entry) — OnHold-- only
	releaseAmount := Amount{
		Value:                  value,
		Operation:              constant.RELEASE,
		TransactionType:        constant.CANCELED,
		RouteValidationEnabled: true,
	}

	afterRelease, err := OperateBalances(releaseAmount, startBalance)
	assert.NoError(t, err)

	// RELEASE with flag ON should only decrement OnHold, NOT touch Available
	assert.True(t, afterRelease.OnHold.Equal(decimal.NewFromInt(0)),
		"OnHold should decrease by value: got %s", afterRelease.OnHold)
	assert.True(t, afterRelease.Available.Equal(decimal.NewFromInt(1500)),
		"Available should NOT change during RELEASE with flag ON: got %s", afterRelease.Available)

	// Version incremented by 1
	assert.Equal(t, int64(8), afterRelease.Version,
		"version should increment by 1 for RELEASE operation")

	// Step 2: CREDIT (credit side of canceled double-entry) — Available++ only
	creditAmount := Amount{
		Value:                  value,
		Operation:              constant.CREDIT,
		TransactionType:        constant.CANCELED,
		RouteValidationEnabled: true,
	}

	afterCredit, err := OperateBalances(creditAmount, afterRelease)
	assert.NoError(t, err)

	// CREDIT with CANCELED flag ON should add to Available
	assert.True(t, afterCredit.Available.Equal(decimal.NewFromInt(2000)),
		"Available should increase by value: got %s", afterCredit.Available)
	assert.True(t, afterCredit.OnHold.Equal(decimal.NewFromInt(0)),
		"OnHold should remain 0: got %s", afterCredit.OnHold)

	// Version incremented by 1 for the credit record
	assert.Equal(t, int64(9), afterCredit.Version,
		"version should increment by 1 for CREDIT operation")

	// Double-entry invariant: total effect of RELEASE+CREDIT restores the hold to available
	totalOnHoldDecrease := startBalance.OnHold.Sub(afterCredit.OnHold)
	totalAvailableIncrease := afterCredit.Available.Sub(startBalance.Available)
	assert.True(t, totalOnHoldDecrease.Equal(totalAvailableIncrease),
		"debit effect (OnHold decrease=%s) must equal credit effect (Available increase=%s)",
		totalOnHoldDecrease, totalAvailableIncrease)
}

// TestProperty_OperateBalances_SumInvariant validates that applying a sequence of
// CREDIT/DEBIT operations results in a balance equal to the expected sum.
// This is a pure property test with no I/O - runs 1000 iterations quickly.
func TestProperty_OperateBalances_SumInvariant(t *testing.T) {
	t.Parallel()

	f := func(seed int64, numOps uint8) bool {
		rng := rand.New(rand.NewSource(seed))
		ops := int(numOps)%20 + 1

		balance := Balance{
			Available: decimal.NewFromInt(1000),
			OnHold:    decimal.Zero,
			Version:   1,
		}

		expectedSum := balance.Available

		for i := 0; i < ops; i++ {
			value := decimal.NewFromInt(int64(rng.Intn(50) + 1))
			var amount Amount

			// 50% DEBIT (if funds available), 50% CREDIT
			if rng.Intn(2) == 0 && expectedSum.GreaterThanOrEqual(value) {
				amount = Amount{
					Value:           value,
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				}
				expectedSum = expectedSum.Sub(value)
			} else {
				amount = Amount{
					Value:           value,
					Operation:       constant.CREDIT,
					TransactionType: constant.CREATED,
				}
				expectedSum = expectedSum.Add(value)
			}

			var err error
			balance, err = OperateBalances(amount, balance)
			if err != nil {
				t.Logf("OperateBalances error: %v", err)
				return false
			}
		}

		if !balance.Available.Equal(expectedSum) {
			t.Logf("Mismatch: balance.Available=%s expectedSum=%s seed=%d numOps=%d",
				balance.Available, expectedSum, seed, numOps)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("OperateBalances sum invariant failed: %v", err)
	}
}

func TestOperateBalances_RouteValidation_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		amount   Amount
		balance  Balance
		expected Balance
	}{
		{
			name: "RELEASE+CANCELED flag ON - zero amount leaves balance unchanged, version+1",
			amount: Amount{
				Value:                  decimal.NewFromInt(0),
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(500),
				OnHold:    decimal.NewFromInt(100),
				Version:   3,
			},
			expected: Balance{
				Available: decimal.NewFromInt(500),
				OnHold:    decimal.NewFromInt(100),
				Version:   4,
			},
		},
		{
			name: "CREDIT+CANCELED flag ON - zero amount leaves Available unchanged, version+1",
			amount: Amount{
				Value:                  decimal.NewFromInt(0),
				Operation:              constant.CREDIT,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(500),
				OnHold:    decimal.NewFromInt(0),
				Version:   5,
			},
			expected: Balance{
				Available: decimal.NewFromInt(500),
				OnHold:    decimal.NewFromInt(0),
				Version:   6,
			},
		},
		{
			name: "RELEASE+CANCELED flag ON - large value decrements OnHold to negative",
			amount: Amount{
				Value:                  decimal.NewFromInt(999999999),
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(100),
				OnHold:    decimal.NewFromInt(50),
				Version:   1,
			},
			expected: Balance{
				Available: decimal.NewFromInt(100),
				OnHold:    decimal.NewFromInt(-999999949),
				Version:   2,
			},
		},
		{
			name: "RELEASE+CANCELED flag ON - version starting at 0",
			amount: Amount{
				Value:                  decimal.NewFromInt(50),
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(200),
				OnHold:    decimal.NewFromInt(50),
				Version:   0,
			},
			expected: Balance{
				Available: decimal.NewFromInt(200),
				OnHold:    decimal.NewFromInt(0),
				Version:   1,
			},
		},
		{
			name: "CREDIT+CANCELED flag ON - version starting at 0",
			amount: Amount{
				Value:                  decimal.NewFromInt(50),
				Operation:              constant.CREDIT,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			balance: Balance{
				Available: decimal.NewFromInt(200),
				OnHold:    decimal.NewFromInt(0),
				Version:   0,
			},
			expected: Balance{
				Available: decimal.NewFromInt(250),
				OnHold:    decimal.NewFromInt(0),
				Version:   1,
			},
		},
		{
			name: "RELEASE+CANCELED flag OFF - zero OnHold stays at zero, Available still increases",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: false,
			},
			balance: Balance{
				Available: decimal.NewFromInt(500),
				OnHold:    decimal.NewFromInt(0),
				Version:   1,
			},
			expected: Balance{
				Available: decimal.NewFromInt(600),
				OnHold:    decimal.NewFromInt(-100),
				Version:   2,
			},
		},
		{
			name: "ONHOLD+PENDING flag OFF - zero Available goes negative, OnHold increases",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: false,
			},
			balance: Balance{
				Available: decimal.NewFromInt(0),
				OnHold:    decimal.NewFromInt(0),
				Version:   0,
			},
			expected: Balance{
				Available: decimal.NewFromInt(-100),
				OnHold:    decimal.NewFromInt(100),
				Version:   1,
			},
		},
		{
			name: "CREDIT+CANCELED flag OFF - falls to default, no special route handling",
			amount: Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.CREDIT,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: false,
			},
			balance: Balance{
				Available: decimal.NewFromInt(500),
				OnHold:    decimal.NewFromInt(50),
				Version:   3,
			},
			expected: Balance{
				Available: decimal.NewFromInt(500),
				OnHold:    decimal.NewFromInt(50),
				Version:   3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := OperateBalances(tt.amount, tt.balance)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Available.String(), result.Available.String(), "available balance mismatch")
			assert.Equal(t, tt.expected.OnHold.String(), result.OnHold.String(), "onHold balance mismatch")
			assert.Equal(t, tt.expected.Version, result.Version, "version mismatch")
		})
	}
}

func TestDetermineOperation_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		isPending         bool
		isFrom            bool
		transactionType   string
		expectedType      string
		expectedDirection string
	}{
		{
			name:              "APPROVED source - DEBIT debit",
			isPending:         true,
			isFrom:            true,
			transactionType:   constant.APPROVED,
			expectedType:      constant.DEBIT,
			expectedDirection: constant.DEBIT,
		},
		{
			name:              "APPROVED destination - CREDIT credit",
			isPending:         true,
			isFrom:            false,
			transactionType:   constant.APPROVED,
			expectedType:      constant.CREDIT,
			expectedDirection: constant.CREDIT,
		},
		{
			name:              "CANCELED destination - falls to default CREDIT credit",
			isPending:         true,
			isFrom:            false,
			transactionType:   constant.CANCELED,
			expectedType:      constant.CREDIT,
			expectedDirection: constant.CREDIT,
		},
		{
			name:              "empty transactionType not pending from - DEBIT debit",
			isPending:         false,
			isFrom:            true,
			transactionType:   "",
			expectedType:      constant.DEBIT,
			expectedDirection: constant.DEBIT,
		},
		{
			name:              "empty transactionType not pending to - CREDIT credit",
			isPending:         false,
			isFrom:            false,
			transactionType:   "",
			expectedType:      constant.CREDIT,
			expectedDirection: constant.CREDIT,
		},
		{
			name:              "unknown transactionType pending - falls to default CREDIT credit",
			isPending:         true,
			isFrom:            false,
			transactionType:   "UNKNOWN",
			expectedType:      constant.CREDIT,
			expectedDirection: constant.CREDIT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotType, gotDirection := DetermineOperation(tt.isPending, tt.isFrom, tt.transactionType)
			assert.Equal(t, tt.expectedType, gotType, "operation type mismatch")
			assert.Equal(t, tt.expectedDirection, gotDirection, "direction mismatch")
		})
	}
}

func TestIsDoubleEntrySource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		amount   Amount
		expected bool
	}{
		{
			name: "PENDING ONHOLD with route validation",
			amount: Amount{
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: true,
			},
			expected: true,
		},
		{
			name: "CANCELED RELEASE with route validation",
			amount: Amount{
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			expected: true,
		},
		{
			name: "PENDING ONHOLD without route validation",
			amount: Amount{
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: false,
			},
			expected: false,
		},
		{
			name: "CREATED DEBIT with route validation",
			amount: Amount{
				Operation:              constant.DEBIT,
				TransactionType:        constant.CREATED,
				RouteValidationEnabled: true,
			},
			expected: false,
		},
		{
			name: "APPROVED CREDIT with route validation",
			amount: Amount{
				Operation:              constant.CREDIT,
				TransactionType:        constant.APPROVED,
				RouteValidationEnabled: true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsDoubleEntrySource(tt.amount)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSplitDoubleEntryOps(t *testing.T) {
	t.Parallel()

	t.Run("PENDING splits into DEBIT + ONHOLD", func(t *testing.T) {
		t.Parallel()

		amt := Amount{
			Operation:              constant.ONHOLD,
			TransactionType:        constant.PENDING,
			RouteValidationEnabled: true,
			Value:                  decimal.NewFromInt(100),
			Asset:                  "BRL",
		}

		op1, op2 := SplitDoubleEntryOps(amt)
		assert.Equal(t, constant.DEBIT, op1.Operation)
		assert.Equal(t, constant.ONHOLD, op2.Operation)
		assert.True(t, op1.Value.Equal(amt.Value))
		assert.True(t, op2.Value.Equal(amt.Value))
		assert.Equal(t, amt.TransactionType, op1.TransactionType)
		assert.Equal(t, amt.TransactionType, op2.TransactionType)
		assert.True(t, op1.RouteValidationEnabled)
		assert.True(t, op2.RouteValidationEnabled)
	})

	t.Run("CANCELED splits into RELEASE + CREDIT", func(t *testing.T) {
		t.Parallel()

		amt := Amount{
			Operation:              constant.RELEASE,
			TransactionType:        constant.CANCELED,
			RouteValidationEnabled: true,
			Value:                  decimal.NewFromInt(200),
			Asset:                  "BRL",
		}

		op1, op2 := SplitDoubleEntryOps(amt)
		assert.Equal(t, constant.RELEASE, op1.Operation)
		assert.Equal(t, constant.CREDIT, op2.Operation)
		assert.True(t, op1.Value.Equal(amt.Value))
		assert.True(t, op2.Value.Equal(amt.Value))
	})
}
