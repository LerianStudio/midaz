// Package transaction provides types and utilities for transaction processing.
//
// TODO(review): Standardize documentation format across all test constructors - some use full Parameters
// section while others use abbreviated Example-only format (reported by code-reviewer on 2025-12-26, severity: Low)
package transaction

import (
	"testing"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/shopspring/decimal"
)

// NewTestAmount creates a fully-initialized Amount struct for testing.
// This constructor ensures all required fields are set, preventing panics
// in applyBalanceOperation caused by missing TransactionType.
//
// Parameters:
//   - t: The testing.T instance for error reporting
//   - asset: The asset code (e.g., "USD", "BRL", "EUR")
//   - value: The decimal value of the amount
//   - operation: The operation type (constant.DEBIT, constant.CREDIT, constant.ONHOLD, constant.RELEASE)
//   - transactionType: The transaction type (constant.CREATED, constant.PENDING, constant.APPROVED, constant.CANCELED)
//
// Example:
//
//	amount := NewTestAmount(t, "USD", decimal.NewFromInt(100), constant.DEBIT, constant.CREATED)
func NewTestAmount(tb testing.TB, asset string, value decimal.Decimal, operation, transactionType string) Amount {
	tb.Helper()

	if asset == "" {
		tb.Fatal("asset must not be empty for test Amount")
	}

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
// Fails the test if amounts have inconsistent assets.
//
// Parameters:
//   - t: The testing.T instance for error reporting
//   - from: Map of account aliases/keys to their debit Amounts
//   - to: Map of account aliases/keys to their credit Amounts
//
// Example:
//
//	responses := NewTestResponses(t,
//	    map[string]Amount{"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100))},
//	    map[string]Amount{"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100))},
//	)
func NewTestResponses(tb testing.TB, from, to map[string]Amount) *Responses {
	tb.Helper()

	// Extract aliases from maps and validate asset consistency
	aliases := make([]string, 0, len(from)+len(to))
	sources := make([]string, 0, len(from))
	destinations := make([]string, 0, len(to))

	var asset string

	for k, v := range from {
		aliases = append(aliases, k)
		sources = append(sources, k)

		if asset == "" {
			asset = v.Asset
		} else if asset != v.Asset {
			tb.Fatalf("inconsistent asset in from[%s]: expected %s, got %s", k, asset, v.Asset)
		}
	}

	for k, v := range to {
		aliases = append(aliases, k)
		destinations = append(destinations, k)

		if asset == "" {
			asset = v.Asset
		} else if asset != v.Asset {
			tb.Fatalf("inconsistent asset in to[%s]: expected %s, got %s", k, asset, v.Asset)
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
//   - t: The testing.T instance for error reporting
//   - total: The total transaction amount
//   - asset: The asset code
//   - from: Map of account aliases/keys to their debit Amounts
//   - to: Map of account aliases/keys to their credit Amounts
//
// Example:
//
//	responses := NewTestResponsesWithTotal(t,
//	    decimal.NewFromInt(100),
//	    "USD",
//	    map[string]Amount{"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100))},
//	    map[string]Amount{"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100))},
//	)
func NewTestResponsesWithTotal(tb testing.TB, total decimal.Decimal, asset string, from, to map[string]Amount) *Responses {
	tb.Helper()

	resp := NewTestResponses(tb, from, to)
	resp.Total = total
	resp.Asset = asset

	return resp
}

// NewTestBalance creates a fully-initialized Balance struct for testing.
// This constructor sets sensible defaults for testing scenarios.
//
// Parameters:
//   - id: The balance ID (UUID string)
//   - alias: The account alias (e.g., "@account1")
//   - assetCode: The asset code (e.g., "USD")
//   - available: The available balance amount
//
// Example:
//
//	balance := NewTestBalance(uuid.New().String(), "@account1", "USD", decimal.NewFromInt(1000))
func NewTestBalance(id, alias, assetCode string, available decimal.Decimal) *Balance {
	return &Balance{
		ID:        id,
		Alias:     alias,
		Key:       "default",
		AssetCode: assetCode,
		Available: available,
		OnHold:    decimal.Zero,
		Version:   1,
		// TODO(review): Consider using a constant for "deposit" account type (reported by code-reviewer on 2025-12-26, severity: Low)
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
}

// NewTestBalanceWithOrg creates a Balance with organization and ledger IDs.
// Use this when tests require full organizational context.
//
// TODO(review): Consider using options struct pattern to prevent parameter confusion (reported by code-reviewer on 2025-12-26, severity: Low)
//
// Parameters:
//   - id: The balance ID (UUID string)
//   - organizationID: The organization UUID string
//   - ledgerID: The ledger UUID string
//   - accountID: The account UUID string
//   - alias: The account alias (e.g., "@account1")
//   - assetCode: The asset code (e.g., "USD")
//   - available: The available balance amount
//
// Example:
//
//	balance := NewTestBalanceWithOrg(
//	    uuid.New().String(),
//	    orgID.String(),
//	    ledgerID.String(),
//	    accountID.String(),
//	    "@account1",
//	    "USD",
//	    decimal.NewFromInt(1000),
//	)
func NewTestBalanceWithOrg(id, organizationID, ledgerID, accountID, alias, assetCode string, available decimal.Decimal) *Balance {
	balance := NewTestBalance(id, alias, assetCode, available)
	balance.OrganizationID = organizationID
	balance.LedgerID = ledgerID
	balance.AccountID = accountID

	return balance
}

// NewTestExternalBalance creates a Balance for an external account type.
// External accounts have special validation rules in the transaction system.
//
// Parameters:
//   - id: The balance ID (UUID string)
//   - alias: The account alias (e.g., "@external/BRL")
//   - assetCode: The asset code (e.g., "USD")
//
// Example:
//
//	balance := NewTestExternalBalance(uuid.New().String(), "@external/BRL", "BRL")
func NewTestExternalBalance(id, alias, assetCode string) *Balance {
	return &Balance{
		ID:             id,
		Alias:          alias,
		Key:            "default",
		AssetCode:      assetCode,
		Available:      decimal.Zero, // External accounts typically have zero or negative balance
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    constant.ExternalAccountType,
		AllowSending:   true,
		AllowReceiving: true,
	}
}

// NewTestPendingDebitAmount creates a DEBIT Amount with PENDING transaction type.
// Use for testing pending/on-hold transactions.
//
// Example:
//
//	amount := NewTestPendingDebitAmount("USD", decimal.NewFromInt(100))
func NewTestPendingDebitAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.DEBIT,
		TransactionType: constant.PENDING,
	}
}

// NewTestPendingCreditAmount creates a CREDIT Amount with PENDING transaction type.
// Use for testing pending transactions.
//
// Example:
//
//	amount := NewTestPendingCreditAmount("USD", decimal.NewFromInt(100))
func NewTestPendingCreditAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.CREDIT,
		TransactionType: constant.PENDING,
	}
}

// NewTestOnHoldAmount creates an ONHOLD Amount for pending source transactions.
// The ONHOLD operation is used when a pending transaction holds funds.
//
// Example:
//
//	amount := NewTestOnHoldAmount("USD", decimal.NewFromInt(100))
func NewTestOnHoldAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.ONHOLD,
		TransactionType: constant.PENDING,
	}
}

// NewTestReleaseAmount creates a RELEASE Amount for canceled transactions.
// The RELEASE operation is used when releasing held funds.
//
// Example:
//
//	amount := NewTestReleaseAmount("USD", decimal.NewFromInt(100))
func NewTestReleaseAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.RELEASE,
		TransactionType: constant.CANCELED,
	}
}

// NewTestAmountPtr creates a pointer to a fully-initialized Amount struct.
// Use this when the Amount needs to be assigned to a pointer field.
//
// Example:
//
//	fromTo := FromTo{
//	    Amount: NewTestAmountPtr(t, "USD", decimal.NewFromInt(100), constant.DEBIT, constant.CREATED),
//	}
func NewTestAmountPtr(tb testing.TB, asset string, value decimal.Decimal, operation, transactionType string) *Amount {
	tb.Helper()

	amount := NewTestAmount(tb, asset, value, operation, transactionType)

	return &amount
}

// NewTestDebitAmountPtr creates a pointer to a DEBIT Amount with CREATED transaction type.
//
// Example:
//
//	fromTo := FromTo{Amount: NewTestDebitAmountPtr("USD", decimal.NewFromInt(100))}
func NewTestDebitAmountPtr(asset string, value decimal.Decimal) *Amount {
	amount := NewTestDebitAmount(asset, value)
	return &amount
}

// NewTestCreditAmountPtr creates a pointer to a CREDIT Amount with CREATED transaction type.
//
// Example:
//
//	fromTo := FromTo{Amount: NewTestCreditAmountPtr("USD", decimal.NewFromInt(100))}
func NewTestCreditAmountPtr(asset string, value decimal.Decimal) *Amount {
	amount := NewTestCreditAmount(asset, value)
	return &amount
}
