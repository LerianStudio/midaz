package mmodel

import (
	"fmt"
	"strings"
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

func TestNewOperation_ValidInputs_ReturnsOperation(t *testing.T) {
	// Arrange
	id := "123e4567-e89b-12d3-a456-426614174000"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"
	opType := "DEBIT"
	assetCode := "USD"
	amountValue := decimal.NewFromInt(100)

	// Act
	op := NewOperation(id, transactionID, opType, assetCode, amountValue)

	// Assert
	if op == nil {
		t.Fatal("Expected non-nil operation")
	}
	if op.ID != id {
		t.Errorf("Expected ID %s, got %s", id, op.ID)
	}
	if op.TransactionID != transactionID {
		t.Errorf("Expected TransactionID %s, got %s", transactionID, op.TransactionID)
	}
	if op.Type != opType {
		t.Errorf("Expected Type %s, got %s", opType, op.Type)
	}
	if op.AssetCode != assetCode {
		t.Errorf("Expected AssetCode %s, got %s", assetCode, op.AssetCode)
	}
	if !op.Amount.Value.Equal(amountValue) {
		t.Errorf("Expected Amount %s, got %s", amountValue.String(), op.Amount.Value.String())
	}
	if op.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestNewOperation_InvalidID_Panics(t *testing.T) {
	// Arrange
	invalidID := "not-a-uuid"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for invalid ID")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "operation ID must be valid UUID") {
			t.Errorf("Expected panic about invalid UUID, got: %v", r)
		}
	}()

	NewOperation(invalidID, transactionID, "DEBIT", "USD", decimal.NewFromInt(100))
}

func TestNewOperation_InvalidType_Panics(t *testing.T) {
	// Arrange
	id := "123e4567-e89b-12d3-a456-426614174000"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for invalid type")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "operation type must be DEBIT or CREDIT") {
			t.Errorf("Expected panic about operation type, got: %v", r)
		}
	}()

	NewOperation(id, transactionID, "INVALID", "USD", decimal.NewFromInt(100))
}

func TestNewOperation_NegativeAmount_Panics(t *testing.T) {
	// Arrange
	id := "123e4567-e89b-12d3-a456-426614174000"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"
	negativeAmount := decimal.NewFromInt(-100)

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for negative amount")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "amount must be non-negative") {
			t.Errorf("Expected panic about non-negative amount, got: %v", r)
		}
	}()

	NewOperation(id, transactionID, "DEBIT", "USD", negativeAmount)
}

func TestNewOperation_EmptyAssetCode_Panics(t *testing.T) {
	// Arrange
	id := "123e4567-e89b-12d3-a456-426614174000"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for empty asset code")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "assetCode must not be empty") {
			t.Errorf("Expected panic about empty asset code, got: %v", r)
		}
	}()

	NewOperation(id, transactionID, "DEBIT", "", decimal.NewFromInt(100))
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
