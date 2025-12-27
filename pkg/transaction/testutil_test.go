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
