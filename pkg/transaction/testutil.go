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

// NewTestResponses creates a fully-initialized Responses struct for testing.
// This constructor ensures From and To maps are properly initialized.
//
// Parameters:
//   - from: Map of account aliases/keys to their debit Amounts
//   - to: Map of account aliases/keys to their credit Amounts
//
// Example:
//
//	responses := NewTestResponses(
//	    map[string]Amount{"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100))},
//	    map[string]Amount{"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100))},
//	)
func NewTestResponses(from, to map[string]Amount) *Responses {
	// Extract aliases from maps
	aliases := make([]string, 0, len(from)+len(to))
	sources := make([]string, 0, len(from))
	destinations := make([]string, 0, len(to))

	for k := range from {
		aliases = append(aliases, k)
		sources = append(sources, k)
	}

	for k := range to {
		aliases = append(aliases, k)
		destinations = append(destinations, k)
	}

	// Determine asset from first Amount (assumes all amounts use same asset)
	var asset string
	for _, v := range from {
		asset = v.Asset
		break
	}

	if asset == "" {
		for _, v := range to {
			asset = v.Asset
			break
		}
	}

	return &Responses{
		Asset:        asset,
		From:         from,
		To:           to,
		Aliases:      aliases,
		Sources:      sources,
		Destinations: destinations,
	}
}

// NewTestResponsesWithTotal creates a Responses struct with explicit total.
// Use this when you need to specify a total that differs from the sum of amounts.
//
// Parameters:
//   - total: The total transaction amount
//   - asset: The asset code
//   - from: Map of account aliases/keys to their debit Amounts
//   - to: Map of account aliases/keys to their credit Amounts
//
// Example:
//
//	responses := NewTestResponsesWithTotal(
//	    decimal.NewFromInt(100),
//	    "USD",
//	    map[string]Amount{"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100))},
//	    map[string]Amount{"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100))},
//	)
func NewTestResponsesWithTotal(total decimal.Decimal, asset string, from, to map[string]Amount) *Responses {
	resp := NewTestResponses(from, to)
	resp.Total = total
	resp.Asset = asset

	return resp
}
