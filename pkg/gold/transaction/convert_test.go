//lint:file-ignore SA1019 This test file intentionally uses deprecated lib-commons transaction types to test conversion functions.
package transaction

import (
	"testing"
	"time"

	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestConvertLibToPkgTransaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    libTransaction.Transaction
		expected pkgTransaction.Transaction
	}{
		{
			name: "converts basic transaction fields",
			input: libTransaction.Transaction{
				ChartOfAccountsGroupName: "test-group",
				Description:              "Test transaction",
				Code:                     "TEST-001",
				Pending:                  false,
				Route:                    "test-route",
				Metadata:                 map[string]any{"key": "value"},
				Send: libTransaction.Send{
					Asset: "BRL",
					Value: decimal.NewFromInt(1000),
					Source: libTransaction.Source{
						Remaining: "",
						From:      []libTransaction.FromTo{},
					},
					Distribute: libTransaction.Distribute{
						Remaining: "",
						To:        []libTransaction.FromTo{},
					},
				},
			},
			expected: pkgTransaction.Transaction{
				ChartOfAccountsGroupName: "test-group",
				Description:              "Test transaction",
				Code:                     "TEST-001",
				Pending:                  false,
				Route:                    "test-route",
				Metadata:                 map[string]any{"key": "value"},
				TransactionDate:          nil,
				Send: pkgTransaction.Send{
					Asset: "BRL",
					Value: decimal.NewFromInt(1000),
					Source: pkgTransaction.Source{
						Remaining: "",
						From:      []pkgTransaction.FromTo{},
					},
					Distribute: pkgTransaction.Distribute{
						Remaining: "",
						To:        []pkgTransaction.FromTo{},
					},
				},
			},
		},
		{
			name: "converts pending transaction",
			input: libTransaction.Transaction{
				Pending: true,
				Send: libTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(500),
					Source: libTransaction.Source{
						From: []libTransaction.FromTo{},
					},
					Distribute: libTransaction.Distribute{
						To: []libTransaction.FromTo{},
					},
				},
			},
			expected: pkgTransaction.Transaction{
				Pending:         true,
				TransactionDate: nil,
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(500),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ConvertLibToPkgTransaction(tt.input)
			assert.Equal(t, tt.expected.ChartOfAccountsGroupName, result.ChartOfAccountsGroupName)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.Code, result.Code)
			assert.Equal(t, tt.expected.Pending, result.Pending)
			assert.Equal(t, tt.expected.Route, result.Route)
			assert.Equal(t, tt.expected.Metadata, result.Metadata)
			assert.Equal(t, tt.expected.Send.Asset, result.Send.Asset)
			assert.True(t, tt.expected.Send.Value.Equal(result.Send.Value))
		})
	}
}

func TestConvertTransactionDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       time.Time
		expectNil   bool
		expectValue time.Time
	}{
		{
			name:      "converts zero time to nil",
			input:     time.Time{},
			expectNil: true,
		},
		{
			name:        "converts valid time",
			input:       time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expectNil:   false,
			expectValue: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertTransactionDate(tt.input)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectValue, result.Time())
			}
		})
	}
}

func TestConvertSend(t *testing.T) {
	t.Parallel()

	input := libTransaction.Send{
		Asset: "BRL",
		Value: decimal.NewFromInt(1000),
		Source: libTransaction.Source{
			Remaining: "remaining_source",
			From: []libTransaction.FromTo{
				{AccountAlias: "@source1", BalanceKey: "default"},
			},
		},
		Distribute: libTransaction.Distribute{
			Remaining: "remaining_dest",
			To: []libTransaction.FromTo{
				{AccountAlias: "@dest1", BalanceKey: "default"},
			},
		},
	}

	result := convertSend(input)

	assert.Equal(t, "BRL", result.Asset)
	assert.True(t, decimal.NewFromInt(1000).Equal(result.Value))
	assert.Equal(t, "remaining_source", result.Source.Remaining)
	assert.Len(t, result.Source.From, 1)
	assert.Equal(t, "@source1", result.Source.From[0].AccountAlias)
	assert.Equal(t, "remaining_dest", result.Distribute.Remaining)
	assert.Len(t, result.Distribute.To, 1)
	assert.Equal(t, "@dest1", result.Distribute.To[0].AccountAlias)
}

func TestConvertSource(t *testing.T) {
	t.Parallel()

	input := libTransaction.Source{
		Remaining: "remaining",
		From: []libTransaction.FromTo{
			{AccountAlias: "@account1", BalanceKey: "key1"},
			{AccountAlias: "@account2", BalanceKey: "key2"},
		},
	}

	result := convertSource(input)

	assert.Equal(t, "remaining", result.Remaining)
	assert.Len(t, result.From, 2)
	assert.Equal(t, "@account1", result.From[0].AccountAlias)
	assert.Equal(t, "key1", result.From[0].BalanceKey)
	assert.Equal(t, "@account2", result.From[1].AccountAlias)
	assert.Equal(t, "key2", result.From[1].BalanceKey)
}

func TestConvertDistribute(t *testing.T) {
	t.Parallel()

	input := libTransaction.Distribute{
		Remaining: "remaining",
		To: []libTransaction.FromTo{
			{AccountAlias: "@dest1", BalanceKey: "key1"},
			{AccountAlias: "@dest2", BalanceKey: "key2"},
		},
	}

	result := convertDistribute(input)

	assert.Equal(t, "remaining", result.Remaining)
	assert.Len(t, result.To, 2)
	assert.Equal(t, "@dest1", result.To[0].AccountAlias)
	assert.Equal(t, "key1", result.To[0].BalanceKey)
	assert.Equal(t, "@dest2", result.To[1].AccountAlias)
	assert.Equal(t, "key2", result.To[1].BalanceKey)
}

func TestConvertFromTo(t *testing.T) {
	t.Parallel()

	amt := libTransaction.Amount{
		Asset: "BRL",
		Value: decimal.NewFromInt(100),
	}
	share := libTransaction.Share{
		Percentage:             50,
		PercentageOfPercentage: 10,
	}
	rate := libTransaction.Rate{
		From:       "BRL",
		To:         "USD",
		Value:      decimal.NewFromFloat(5.5),
		ExternalID: "ext-123",
	}

	input := libTransaction.FromTo{
		AccountAlias:    "@account",
		BalanceKey:      "balance-key",
		Amount:          &amt,
		Share:           &share,
		Remaining:       "remaining",
		Rate:            &rate,
		Description:     "description",
		ChartOfAccounts: "1000",
		Metadata:        map[string]any{"meta": "data"},
		IsFrom:          true,
		Route:           "route-123",
	}

	result := convertFromTo(input)

	assert.Equal(t, "@account", result.AccountAlias)
	assert.Equal(t, "balance-key", result.BalanceKey)
	assert.NotNil(t, result.Amount)
	assert.Equal(t, "BRL", result.Amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(result.Amount.Value))
	assert.NotNil(t, result.Share)
	assert.Equal(t, int64(50), result.Share.Percentage)
	assert.Equal(t, int64(10), result.Share.PercentageOfPercentage)
	assert.Equal(t, "remaining", result.Remaining)
	assert.NotNil(t, result.Rate)
	assert.Equal(t, "BRL", result.Rate.From)
	assert.Equal(t, "USD", result.Rate.To)
	assert.Equal(t, "description", result.Description)
	assert.Equal(t, "1000", result.ChartOfAccounts)
	assert.Equal(t, map[string]any{"meta": "data"}, result.Metadata)
	assert.True(t, result.IsFrom)
	assert.Equal(t, "route-123", result.Route)
}

func TestConvertAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     *libTransaction.Amount
		expectNil bool
	}{
		{
			name:      "nil input returns nil",
			input:     nil,
			expectNil: true,
		},
		{
			name: "valid input converts correctly",
			input: &libTransaction.Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(100),
				Operation:       "debit",
				TransactionType: "transfer",
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertAmount(tt.input)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.Asset, result.Asset)
				assert.True(t, tt.input.Value.Equal(result.Value))
				assert.Equal(t, tt.input.Operation, result.Operation)
				assert.Equal(t, tt.input.TransactionType, result.TransactionType)
			}
		})
	}
}

func TestConvertShare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     *libTransaction.Share
		expectNil bool
	}{
		{
			name:      "nil input returns nil",
			input:     nil,
			expectNil: true,
		},
		{
			name: "valid input converts correctly",
			input: &libTransaction.Share{
				Percentage:             75,
				PercentageOfPercentage: 25,
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertShare(tt.input)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.Percentage, result.Percentage)
				assert.Equal(t, tt.input.PercentageOfPercentage, result.PercentageOfPercentage)
			}
		})
	}
}

func TestConvertRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     *libTransaction.Rate
		expectNil bool
	}{
		{
			name:      "nil input returns nil",
			input:     nil,
			expectNil: true,
		},
		{
			name: "valid input converts correctly",
			input: &libTransaction.Rate{
				From:       "BRL",
				To:         "USD",
				Value:      decimal.NewFromFloat(5.25),
				ExternalID: "ext-rate-123",
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertRate(tt.input)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.From, result.From)
				assert.Equal(t, tt.input.To, result.To)
				assert.True(t, tt.input.Value.Equal(result.Value))
				assert.Equal(t, tt.input.ExternalID, result.ExternalID)
			}
		})
	}
}

func TestConvertLibToPkgTransaction_WithTransactionDate(t *testing.T) {
	t.Parallel()

	transactionDate := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)

	input := libTransaction.Transaction{
		ChartOfAccountsGroupName: "test-group",
		Description:              "Transaction with date",
		TransactionDate:          transactionDate,
		Send: libTransaction.Send{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
			Source: libTransaction.Source{
				From: []libTransaction.FromTo{},
			},
			Distribute: libTransaction.Distribute{
				To: []libTransaction.FromTo{},
			},
		},
	}

	result := ConvertLibToPkgTransaction(input)

	assert.NotNil(t, result.TransactionDate)
	assert.Equal(t, transactionDate, result.TransactionDate.Time())
}

func TestConvertFromTo_WithNilNestedFields(t *testing.T) {
	t.Parallel()

	input := libTransaction.FromTo{
		AccountAlias: "@account",
		BalanceKey:   "default",
		Amount:       nil,
		Share:        nil,
		Rate:         nil,
	}

	result := convertFromTo(input)

	assert.Equal(t, "@account", result.AccountAlias)
	assert.Equal(t, "default", result.BalanceKey)
	assert.Nil(t, result.Amount)
	assert.Nil(t, result.Share)
	assert.Nil(t, result.Rate)
}
