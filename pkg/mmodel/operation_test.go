package mmodel

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestOperationAmount_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		amount   OperationAmount
		expected bool
	}{
		{
			name:     "nil value is empty",
			amount:   OperationAmount{Value: nil},
			expected: true,
		},
		{
			name: "zero value is not empty",
			amount: OperationAmount{
				Value: decimalPtr(decimal.Zero),
			},
			expected: false,
		},
		{
			name: "positive value is not empty",
			amount: OperationAmount{
				Value: decimalPtr(decimal.NewFromInt(100)),
			},
			expected: false,
		},
		{
			name: "negative value is not empty",
			amount: OperationAmount{
				Value: decimalPtr(decimal.NewFromInt(-50)),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.amount.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOperationBalance_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		balance  OperationBalance
		expected bool
	}{
		{
			name:     "both nil is empty",
			balance:  OperationBalance{Available: nil, OnHold: nil},
			expected: true,
		},
		{
			name: "only available set is not empty",
			balance: OperationBalance{
				Available: decimalPtr(decimal.NewFromInt(100)),
				OnHold:    nil,
			},
			expected: false,
		},
		{
			name: "only onHold set is not empty",
			balance: OperationBalance{
				Available: nil,
				OnHold:    decimalPtr(decimal.NewFromInt(50)),
			},
			expected: false,
		},
		{
			name: "both set is not empty",
			balance: OperationBalance{
				Available: decimalPtr(decimal.NewFromInt(100)),
				OnHold:    decimalPtr(decimal.NewFromInt(50)),
			},
			expected: false,
		},
		{
			name: "zero values are not empty",
			balance: OperationBalance{
				Available: decimalPtr(decimal.Zero),
				OnHold:    decimalPtr(decimal.Zero),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.balance.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOperation_ToLog(t *testing.T) {
	createdAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	deletedAt := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	amount := OperationAmount{Value: decimalPtr(decimal.NewFromInt(1000))}
	balance := OperationBalance{
		Available: decimalPtr(decimal.NewFromInt(5000)),
		OnHold:    decimalPtr(decimal.NewFromInt(500)),
		Version:   int64Ptr(1),
	}
	balanceAfter := OperationBalance{
		Available: decimalPtr(decimal.NewFromInt(4000)),
		OnHold:    decimalPtr(decimal.NewFromInt(500)),
		Version:   int64Ptr(2),
	}
	status := Status{Code: "ACTIVE"}

	operation := &Operation{
		ID:              "op-123",
		TransactionID:   "txn-456",
		Description:     "Test operation",
		Type:            "DEBIT",
		AssetCode:       "BRL",
		ChartOfAccounts: "1000",
		Amount:          amount,
		Balance:         balance,
		BalanceAfter:    balanceAfter,
		Status:          status,
		AccountID:       "acc-789",
		AccountAlias:    "@test_account",
		BalanceKey:      "default",
		BalanceID:       "bal-012",
		OrganizationID:  "org-345",
		LedgerID:        "led-678",
		Route:           "route-001",
		BalanceAffected: true,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		DeletedAt:       &deletedAt,
		Metadata:        map[string]any{"key": "value"},
	}

	log := operation.ToLog()

	// Verify immutable fields are copied
	assert.Equal(t, operation.ID, log.ID)
	assert.Equal(t, operation.TransactionID, log.TransactionID)
	assert.Equal(t, operation.Type, log.Type)
	assert.Equal(t, operation.AssetCode, log.AssetCode)
	assert.Equal(t, operation.ChartOfAccounts, log.ChartOfAccounts)
	assert.Equal(t, operation.Amount, log.Amount)
	assert.Equal(t, operation.Balance, log.Balance)
	assert.Equal(t, operation.BalanceAfter, log.BalanceAfter)
	assert.Equal(t, operation.Status, log.Status)
	assert.Equal(t, operation.AccountID, log.AccountID)
	assert.Equal(t, operation.AccountAlias, log.AccountAlias)
	assert.Equal(t, operation.BalanceKey, log.BalanceKey)
	assert.Equal(t, operation.BalanceID, log.BalanceID)
	assert.Equal(t, operation.Route, log.Route)
	assert.Equal(t, operation.CreatedAt, log.CreatedAt)
	assert.Equal(t, operation.BalanceAffected, log.BalanceAffected)

	// Verify OperationLog doesn't have mutable fields
	// (Description, Metadata, UpdatedAt, DeletedAt, OrganizationID, LedgerID are excluded)
}

func TestOperation_ToLog_WithMinimalData(t *testing.T) {
	operation := &Operation{
		ID:              "op-minimal",
		TransactionID:   "txn-minimal",
		Type:            "CREDIT",
		BalanceAffected: false,
	}

	log := operation.ToLog()

	assert.Equal(t, "op-minimal", log.ID)
	assert.Equal(t, "txn-minimal", log.TransactionID)
	assert.Equal(t, "CREDIT", log.Type)
	assert.False(t, log.BalanceAffected)
	assert.Empty(t, log.AssetCode)
	assert.Empty(t, log.AccountID)
}

// Helper functions for creating pointers
func decimalPtr(d decimal.Decimal) *decimal.Decimal {
	return &d
}

func int64Ptr(i int64) *int64 {
	return &i
}

func stringPtr(s string) *string {
	return &s
}
