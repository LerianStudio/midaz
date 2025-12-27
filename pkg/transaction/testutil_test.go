package transaction

import (
	"testing"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewTestAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		asset           string
		value           decimal.Decimal
		operation       string
		transactionType string
	}{
		{
			name:            "debit created",
			asset:           "USD",
			value:           decimal.NewFromInt(100),
			operation:       constant.DEBIT,
			transactionType: constant.CREATED,
		},
		{
			name:            "credit created",
			asset:           "EUR",
			value:           decimal.NewFromFloat(50.5),
			operation:       constant.CREDIT,
			transactionType: constant.CREATED,
		},
		{
			name:            "onhold pending",
			asset:           "BRL",
			value:           decimal.NewFromInt(200),
			operation:       constant.ONHOLD,
			transactionType: constant.PENDING,
		},
		{
			name:            "release canceled",
			asset:           "GBP",
			value:           decimal.NewFromInt(75),
			operation:       constant.RELEASE,
			transactionType: constant.CANCELED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			amount := NewTestAmount(tt.asset, tt.value, tt.operation, tt.transactionType)

			assert.Equal(t, tt.asset, amount.Asset)
			assert.True(t, tt.value.Equal(amount.Value))
			assert.Equal(t, tt.operation, amount.Operation)
			assert.Equal(t, tt.transactionType, amount.TransactionType)
		})
	}
}

func TestNewTestDebitAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestDebitAmount("USD", decimal.NewFromInt(100))

	assert.Equal(t, "USD", amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(amount.Value))
	assert.Equal(t, constant.DEBIT, amount.Operation)
	assert.Equal(t, constant.CREATED, amount.TransactionType)
}

func TestNewTestCreditAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestCreditAmount("EUR", decimal.NewFromInt(50))

	assert.Equal(t, "EUR", amount.Asset)
	assert.True(t, decimal.NewFromInt(50).Equal(amount.Value))
	assert.Equal(t, constant.CREDIT, amount.Operation)
	assert.Equal(t, constant.CREATED, amount.TransactionType)
}

func TestNewTestPendingDebitAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestPendingDebitAmount("USD", decimal.NewFromInt(100))

	assert.Equal(t, "USD", amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(amount.Value))
	assert.Equal(t, constant.DEBIT, amount.Operation)
	assert.Equal(t, constant.PENDING, amount.TransactionType)
}

func TestNewTestPendingCreditAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestPendingCreditAmount("EUR", decimal.NewFromInt(50))

	assert.Equal(t, "EUR", amount.Asset)
	assert.True(t, decimal.NewFromInt(50).Equal(amount.Value))
	assert.Equal(t, constant.CREDIT, amount.Operation)
	assert.Equal(t, constant.PENDING, amount.TransactionType)
}

func TestNewTestOnHoldAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestOnHoldAmount("USD", decimal.NewFromInt(100))

	assert.Equal(t, "USD", amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(amount.Value))
	assert.Equal(t, constant.ONHOLD, amount.Operation)
	assert.Equal(t, constant.PENDING, amount.TransactionType)
}

func TestNewTestReleaseAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestReleaseAmount("USD", decimal.NewFromInt(100))

	assert.Equal(t, "USD", amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(amount.Value))
	assert.Equal(t, constant.RELEASE, amount.Operation)
	assert.Equal(t, constant.CANCELED, amount.TransactionType)
}

// TODO(review): Consider adding test cases for empty from/to maps (reported by code-reviewer on 2025-12-26, severity: Low)
func TestNewTestResponses(t *testing.T) {
	t.Parallel()

	from := map[string]Amount{
		"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100)),
	}
	to := map[string]Amount{
		"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100)),
	}

	responses := NewTestResponses(from, to)

	assert.NotNil(t, responses)
	assert.Equal(t, "USD", responses.Asset)
	assert.Len(t, responses.From, 1)
	assert.Len(t, responses.To, 1)
	assert.Contains(t, responses.Aliases, "@account1")
	assert.Contains(t, responses.Aliases, "@account2")
	assert.Contains(t, responses.Sources, "@account1")
	assert.Contains(t, responses.Destinations, "@account2")
}

func TestNewTestResponsesWithTotal(t *testing.T) {
	t.Parallel()

	from := map[string]Amount{
		"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100)),
	}
	to := map[string]Amount{
		"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100)),
	}

	responses := NewTestResponsesWithTotal(decimal.NewFromInt(100), "USD", from, to)

	assert.NotNil(t, responses)
	assert.True(t, decimal.NewFromInt(100).Equal(responses.Total))
	assert.Equal(t, "USD", responses.Asset)
}

func TestNewTestBalance(t *testing.T) {
	t.Parallel()

	balance := NewTestBalance("test-id", "@account1", "USD", decimal.NewFromInt(1000))

	assert.NotNil(t, balance)
	assert.Equal(t, "test-id", balance.ID)
	assert.Equal(t, "@account1", balance.Alias)
	assert.Equal(t, "default", balance.Key)
	assert.Equal(t, "USD", balance.AssetCode)
	assert.True(t, decimal.NewFromInt(1000).Equal(balance.Available))
	assert.True(t, decimal.Zero.Equal(balance.OnHold))
	assert.Equal(t, int64(1), balance.Version)
	assert.Equal(t, "deposit", balance.AccountType)
	assert.True(t, balance.AllowSending)
	assert.True(t, balance.AllowReceiving)
}

func TestNewTestBalanceWithOrg(t *testing.T) {
	t.Parallel()

	balance := NewTestBalanceWithOrg(
		"balance-id",
		"org-id",
		"ledger-id",
		"account-id",
		"@account1",
		"USD",
		decimal.NewFromInt(500),
	)

	assert.NotNil(t, balance)
	assert.Equal(t, "balance-id", balance.ID)
	assert.Equal(t, "org-id", balance.OrganizationID)
	assert.Equal(t, "ledger-id", balance.LedgerID)
	assert.Equal(t, "account-id", balance.AccountID)
	assert.Equal(t, "@account1", balance.Alias)
	assert.Equal(t, "USD", balance.AssetCode)
	assert.True(t, decimal.NewFromInt(500).Equal(balance.Available))
}

func TestNewTestExternalBalance(t *testing.T) {
	t.Parallel()

	balance := NewTestExternalBalance("ext-id", "@external/BRL", "BRL")

	assert.NotNil(t, balance)
	assert.Equal(t, "ext-id", balance.ID)
	assert.Equal(t, "@external/BRL", balance.Alias)
	assert.Equal(t, "BRL", balance.AssetCode)
	assert.True(t, decimal.Zero.Equal(balance.Available))
	assert.Equal(t, constant.ExternalAccountType, balance.AccountType)
}

// TestAmountWorksWithOperateBalances verifies that Amount structs created by
// constructors work correctly with the OperateBalances function.
func TestAmountWorksWithOperateBalances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		amount            Amount
		initialAvailable  decimal.Decimal
		initialOnHold     decimal.Decimal
		expectedAvailable decimal.Decimal
		expectedOnHold    decimal.Decimal
	}{
		{
			name:              "debit created reduces available",
			amount:            NewTestDebitAmount("USD", decimal.NewFromInt(50)),
			initialAvailable:  decimal.NewFromInt(100),
			initialOnHold:     decimal.Zero,
			expectedAvailable: decimal.NewFromInt(50),
			expectedOnHold:    decimal.Zero,
		},
		{
			name:              "credit created increases available",
			amount:            NewTestCreditAmount("USD", decimal.NewFromInt(50)),
			initialAvailable:  decimal.NewFromInt(100),
			initialOnHold:     decimal.Zero,
			expectedAvailable: decimal.NewFromInt(150),
			expectedOnHold:    decimal.Zero,
		},
		{
			name:              "onhold pending moves to onHold",
			amount:            NewTestOnHoldAmount("USD", decimal.NewFromInt(30)),
			initialAvailable:  decimal.NewFromInt(100),
			initialOnHold:     decimal.Zero,
			expectedAvailable: decimal.NewFromInt(70),
			expectedOnHold:    decimal.NewFromInt(30),
		},
		{
			name:              "release canceled returns from onHold",
			amount:            NewTestReleaseAmount("USD", decimal.NewFromInt(30)),
			initialAvailable:  decimal.NewFromInt(70),
			initialOnHold:     decimal.NewFromInt(30),
			expectedAvailable: decimal.NewFromInt(100),
			expectedOnHold:    decimal.Zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balance := Balance{
				Available: tt.initialAvailable,
				OnHold:    tt.initialOnHold,
				Version:   1,
			}

			result, err := OperateBalances(tt.amount, balance)

			assert.NoError(t, err)
			assert.True(t, tt.expectedAvailable.Equal(result.Available),
				"expected available %s, got %s", tt.expectedAvailable, result.Available)
			assert.True(t, tt.expectedOnHold.Equal(result.OnHold),
				"expected onHold %s, got %s", tt.expectedOnHold, result.OnHold)
		})
	}
}
