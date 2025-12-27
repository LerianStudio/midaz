package transaction

import (
	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/shopspring/decimal"
)

// NewTestAmount creates a fully-initialized Amount struct for testing.
// This constructor ensures all required fields are set, preventing panics
// in applyBalanceOperation caused by missing TransactionType.
//
// Parameters:
//   - asset: The asset code (e.g., "USD", "BRL", "EUR")
//   - value: The decimal value of the amount
//   - operation: The operation type (constant.DEBIT, constant.CREDIT, constant.ONHOLD, constant.RELEASE)
//   - transactionType: The transaction type (constant.CREATED, constant.PENDING, constant.APPROVED, constant.CANCELED)
//
// Example:
//
//	amount := NewTestAmount("USD", decimal.NewFromInt(100), constant.DEBIT, constant.CREATED)
func NewTestAmount(asset string, value decimal.Decimal, operation, transactionType string) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       operation,
		TransactionType: transactionType,
	}
}

// NewTestDebitAmount creates a DEBIT Amount with CREATED transaction type.
// This is a convenience constructor for the most common debit scenario.
//
// Parameters:
//   - asset: The asset code (e.g., "USD", "BRL", "EUR")
//   - value: The decimal value of the amount
//
// Example:
//
//	amount := NewTestDebitAmount("USD", decimal.NewFromInt(100))
func NewTestDebitAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.DEBIT,
		TransactionType: constant.CREATED,
	}
}

// NewTestCreditAmount creates a CREDIT Amount with CREATED transaction type.
// This is a convenience constructor for the most common credit scenario.
//
// Parameters:
//   - asset: The asset code (e.g., "USD", "BRL", "EUR")
//   - value: The decimal value of the amount
//
// Example:
//
//	amount := NewTestCreditAmount("USD", decimal.NewFromInt(100))
func NewTestCreditAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.CREDIT,
		TransactionType: constant.CREATED,
	}
}
