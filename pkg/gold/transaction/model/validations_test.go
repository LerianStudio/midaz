package model

import (
	"math"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/pkg"
	cn "github.com/LerianStudio/midaz/pkg/constant"
	a "github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/stretchr/testify/require"
)

func TestValidateAccounts(t *testing.T) {
	tests := []struct {
		name          string
		validate      Responses
		accounts      []*a.Account
		expectedError error
	}{
		{
			name: "Mismatch in number of accounts",
			validate: Responses{
				From: map[string]Amount{
					"account1": {Value: 100, Scale: 2},
				},
				To: map[string]Amount{
					"account2": {Value: 200, Scale: 2},
				},
			},
			accounts: []*a.Account{
				{
					Id:        "account1",
					Alias:     "alias1",
					AssetCode: cn.DefaultAssetCode,
				},
			},
			expectedError: pkg.ValidateBusinessError(cn.ErrAccountIneligibility, "ValidateAccounts"),
		},
		{
			name: "Invalid asset code",
			validate: Responses{
				From: map[string]Amount{
					"account1": {Value: 100, Scale: 2},
				},
				To: map[string]Amount{
					"account2": {Value: 200, Scale: 2},
				},
			},
			accounts: []*a.Account{
				{
					Id:        "account1",
					Alias:     "alias1",
					AssetCode: "nonDefaultAsset",
				},
				{
					Id:        "account2",
					Alias:     "alias2",
					AssetCode: cn.DefaultAssetCode,
				},
			},
			expectedError: pkg.ValidateBusinessError(cn.ErrAssetCodeNotFound, "ValidateAccounts"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccounts(tt.validate, tt.accounts)

			if (err != nil && tt.expectedError == nil) || (err == nil && tt.expectedError != nil) {
				t.Fatalf("Expected error: %v, got: %v", tt.expectedError, err)
			}

			if err != nil && tt.expectedError != nil && err.Error() != tt.expectedError.Error() {
				t.Errorf("Expected error message %v, got %v", tt.expectedError.Error(), err.Error())
			}
		})
	}
}

func TestValidateFromToOperation(t *testing.T) {
	tests := []struct {
		name            string
		ft              FromTo
		validate        Responses
		acc             *a.Account
		expectedError   error
		expectedAmount  Amount
		expectedBalance Balance
	}{
		{
			name: "Valid debit operation",
			ft: FromTo{
				Account: "account1",
				IsFrom:  true,
			},
			validate: Responses{
				From: map[string]Amount{
					"account1": {Value: 100, Scale: 2},
				},
			},
			acc: &a.Account{
				Alias:   "account1",
				Balance: &a.Balance{Available: 1000, Scale: 2},
			},
			expectedError:   nil,
			expectedAmount:  Amount{Value: 100, Scale: 2},
			expectedBalance: Balance{Available: 900},
		},
		{
			name: "Valid credit operation",
			ft: FromTo{
				Account: "account2",
				IsFrom:  false,
			},
			validate: Responses{
				To: map[string]Amount{
					"account2": {Value: 200, Scale: 2},
				},
			},
			acc: &a.Account{
				Alias:   "account2",
				Balance: &a.Balance{Available: 800, Scale: 2},
			},
			expectedError:   nil,
			expectedAmount:  Amount{Value: 200, Scale: 2},
			expectedBalance: Balance{Available: 1000},
		},
		{
			name: "Insufficient funds error",
			ft: FromTo{
				Account: "account3",
				IsFrom:  true,
			},
			validate: Responses{
				From: map[string]Amount{
					"account3": {Value: 1200, Scale: 2},
				},
			},
			acc: &a.Account{
				Alias:   "account3",
				Balance: &a.Balance{Available: 1000, Scale: 2},
			},
			expectedError:   pkg.ValidateBusinessError(cn.ErrInsufficientFunds, "ValidateFromToOperation", "account3"),
			expectedAmount:  Amount{},
			expectedBalance: Balance{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, balanceAfter, err := ValidateFromToOperation(tt.ft, tt.validate, tt.acc)

			if (err != nil && tt.expectedError == nil) || (err == nil && tt.expectedError != nil) {
				t.Fatalf("Expected error: %v, got: %v", tt.expectedError, err)
			}

			if err != nil && tt.expectedError != nil && !strings.Contains(err.Error(), tt.expectedError.Error()) {
				t.Errorf("Expected error message %v, got %v", tt.expectedError.Error(), err.Error())
			}

			if amount != tt.expectedAmount {
				t.Errorf("Expected amount %+v, got %+v", tt.expectedAmount, amount)
			}

			if balanceAfter.Available != tt.expectedBalance.Available {
				t.Errorf("Expected balance available %v, got %v", tt.expectedBalance.Available, balanceAfter.Available)
			}
		})
	}
}

func TestUpdateAccountsWithError(t *testing.T) {
	fromTo := map[string]Amount{
		"account1": {Value: 100, Scale: 2, Asset: "USD"},
	}

	accounts := []*a.Account{
		{
			Id:    "account1",
			Alias: "alias1",
			Balance: &a.Balance{
				Available: 1000,
				Scale:     2,
				OnHold:    0,
			},
			Status: &a.Status{
				Code:        "active",
				Description: "Account is active",
			},
			AllowSending:   true,
			AllowReceiving: true,
		},
	}

	resultChan := make(chan []*a.Account)
	errorChan := make(chan error, 1)

	go UpdateAccounts("DEBIT", fromTo, accounts, resultChan, errorChan)

	select {
	case err := <-errorChan:
		if err == nil {
			t.Fatalf("Expected mocked operation error, got %v", err)
		}
	case updatedAccounts := <-resultChan:
		if len(updatedAccounts) > 0 {
			t.Fatalf("Expected no accounts to be updated, but got %d", len(updatedAccounts))
		}
	default:
	}
}

func TestUpdateAccounts(t *testing.T) {
	fromTo := map[string]Amount{
		"account1": {Value: 100, Scale: 2, Asset: "USD"},
		"account2": {Value: 200, Scale: 2, Asset: "USD"},
	}

	accounts := []*a.Account{
		{
			Id:    "account1",
			Alias: "alias1",
			Balance: &a.Balance{
				Available: 1000,
				Scale:     2,
				OnHold:    0,
			},
			Status: &a.Status{
				Code:        "active",
				Description: "Account is active",
			},
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			Id:    "account2",
			Alias: "alias2",
			Balance: &a.Balance{
				Available: 2000,
				Scale:     2,
				OnHold:    0,
			},
			Status: &a.Status{
				Code:        "active",
				Description: "Account is active",
			},
			AllowSending:   true,
			AllowReceiving: true,
		},
	}

	resultChan := make(chan []*a.Account)
	errorChan := make(chan error, 1)

	go UpdateAccounts("DEBIT", fromTo, accounts, resultChan, errorChan)

	updatedAccounts := <-resultChan

	select {
	case err := <-errorChan:
		t.Fatalf("Unexpected error: %v", err)
	default:
	}

	if len(updatedAccounts) != len(accounts) {
		t.Fatalf("Expected %d updated accounts, got %d", len(accounts), len(updatedAccounts))
	}

	expectedBalances := []float64{900.0, 1800.0}
	for i, acc := range updatedAccounts {
		if acc.Balance.Available != expectedBalances[i] {
			t.Errorf("Expected balance for account %s to be %f, got %f",
				acc.Id, expectedBalances[i], acc.Balance.Available)
		}
	}
}

func TestFindScale(t *testing.T) {
	tests := []struct {
		name     string
		asset    string
		v        float64
		s        int
		expected Amount
	}{
		{
			name:  "Integer value with scale 0",
			asset: "USD",
			v:     100.0,
			s:     0,
			expected: Amount{
				Asset: "USD",
				Value: 100,
				Scale: 0,
			},
		},
		{
			name:  "Decimal value with no additional scale",
			asset: "USD",
			v:     123.45,
			s:     0,
			expected: Amount{
				Asset: "USD",
				Value: 12345,
				Scale: 2,
			},
		},
		{
			name:  "Decimal value with existing scale",
			asset: "USD",
			v:     123.45,
			s:     1,
			expected: Amount{
				Asset: "USD",
				Value: 12345,
				Scale: 3,
			},
		},
		{
			name:  "Integer value with existing scale",
			asset: "USD",
			v:     200.0,
			s:     1,
			expected: Amount{
				Asset: "USD",
				Value: 200,
				Scale: 1,
			},
		},
		{
			name:  "Large value with decimal",
			asset: "BTC",
			v:     0.12345678,
			s:     2,
			expected: Amount{
				Asset: "BTC",
				Value: 12345678,
				Scale: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindScale(tt.asset, tt.v, tt.s)

			if result.Asset != tt.expected.Asset {
				t.Errorf("Expected asset %s, got %s", tt.expected.Asset, result.Asset)
			}

			if result.Value != tt.expected.Value {
				t.Errorf("Expected value %d, got %d", tt.expected.Value, result.Value)
			}

			if result.Scale != tt.expected.Scale {
				t.Errorf("Expected scale %d, got %d", tt.expected.Scale, result.Scale)
			}
		})
	}
}

func TestScale(t *testing.T) {
	tests := []struct {
		name     string
		v        int
		s0       int
		s1       int
		expected float64
	}{
		{
			name:     "Same scale",
			v:        1000,
			s0:       2,
			s1:       2,
			expected: 1000.0,
		},
		{
			name:     "Scale up",
			v:        1000,
			s0:       2,
			s1:       3,
			expected: 10000.0,
		},
		{
			name:     "Scale down",
			v:        1000,
			s0:       3,
			s1:       2,
			expected: 100.0,
		},
		{
			name:     "Negative scale difference",
			v:        1000,
			s0:       -1,
			s1:       2,
			expected: 1e+06,
		},
		{
			name:     "Zero value",
			v:        0,
			s0:       3,
			s1:       2,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Scale(tt.v, tt.s0, tt.s1)
			if math.Abs(result-tt.expected) > 1e-6 {
				t.Errorf("Scale(%d, %d, %d) = %f; want %f", tt.v, tt.s0, tt.s1, result, tt.expected)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name      string
		total     Amount
		amount    Amount
		remaining Amount
		expected  struct {
			total     Amount
			amount    Amount
			remaining Amount
		}
	}{
		{
			name: "Same scale for total, amount, and remaining",
			total: Amount{
				Value: 500,
				Scale: 2,
			},
			amount: Amount{
				Value: 200,
				Scale: 2,
			},
			remaining: Amount{
				Value: 1000,
				Scale: 2,
			},
			expected: struct {
				total     Amount
				amount    Amount
				remaining Amount
			}{
				total: Amount{
					Value: 700,
					Scale: 2,
				},
				amount: Amount{
					Value: 200,
					Scale: 2,
				},
				remaining: Amount{
					Value: 800,
					Scale: 2,
				},
			},
		},
		{
			name: "Different scale between total and amount",
			total: Amount{
				Value: 500,
				Scale: 2,
			},
			amount: Amount{
				Value: 200,
				Scale: 3,
			},
			remaining: Amount{
				Value: 1000,
				Scale: 3,
			},
			expected: struct {
				total     Amount
				amount    Amount
				remaining Amount
			}{
				total: Amount{
					Value: 5200,
					Scale: 3,
				},
				amount: Amount{
					Value: 200,
					Scale: 3,
				},
				remaining: Amount{
					Value: 800,
					Scale: 3,
				},
			},
		},
		{
			name: "Different scale for remaining and amount",
			total: Amount{
				Value: 0,
				Scale: 2,
			},
			amount: Amount{
				Value: 300,
				Scale: 3,
			},
			remaining: Amount{
				Value: 1500,
				Scale: 2,
			},
			expected: struct {
				total     Amount
				amount    Amount
				remaining Amount
			}{
				total: Amount{
					Value: 300,
					Scale: 3,
				},
				amount: Amount{
					Value: 300,
					Scale: 3,
				},
				remaining: Amount{
					Value: 14700,
					Scale: 3,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalize(&tt.total, &tt.amount, &tt.remaining)

			if tt.total != tt.expected.total {
				t.Errorf("Expected total %+v, got %+v", tt.expected.total, tt.total)
			}

			if tt.amount != tt.expected.amount {
				t.Errorf("Expected amount %+v, got %+v", tt.expected.amount, tt.amount)
			}

			if tt.remaining != tt.expected.remaining {
				t.Errorf("Expected remaining %+v, got %+v", tt.expected.remaining, tt.remaining)
			}
		})
	}
}

func TestOperateAmounts(t *testing.T) {
	tests := []struct {
		name       string
		amount     Amount
		balance    *a.Balance
		operation  string
		expected   Balance
		shouldFail bool
	}{
		{
			name: "Debit with matching scale",
			amount: Amount{
				Asset: "USD",
				Scale: 2,
				Value: 100,
			},
			balance: &a.Balance{
				Available: 1000,
				OnHold:    0,
				Scale:     2,
			},
			operation: cn.DEBIT,
			expected: Balance{
				Available: 900,
				OnHold:    0,
				Scale:     2,
			},
			shouldFail: false,
		},
		{
			name: "Debit with different scales",
			amount: Amount{
				Asset: "USD",
				Scale: 3,
				Value: 100,
			},
			balance: &a.Balance{
				Available: 10000,
				OnHold:    0,
				Scale:     2,
			},
			operation: cn.DEBIT,
			expected: Balance{
				Available: 99900,
				OnHold:    0,
				Scale:     3,
			},
			shouldFail: false,
		},
		{
			name: "Credit with matching scale",
			amount: Amount{
				Asset: "USD",
				Scale: 2,
				Value: 200,
			},
			balance: &a.Balance{
				Available: 1000,
				OnHold:    0,
				Scale:     2,
			},
			operation: "CREDIT",
			expected: Balance{
				Available: 1200,
				OnHold:    0,
				Scale:     2,
			},
			shouldFail: false,
		},
		{
			name: "Credit with different scales",
			amount: Amount{
				Asset: "USD",
				Scale: 3,
				Value: 500,
			},
			balance: &a.Balance{
				Available: 10000,
				OnHold:    0,
				Scale:     2,
			},
			operation: "CREDIT",
			expected: Balance{
				Available: 100500,
				OnHold:    0,
				Scale:     3,
			},
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OperateAmounts(tt.amount, tt.balance, tt.operation)
			if result.Available != tt.expected.Available || result.Scale != tt.expected.Scale || result.OnHold != tt.expected.OnHold {
				t.Errorf("Expected result %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func Test_calculateTotal(t *testing.T) {
	fromTos := []FromTo{
		{
			Account: "@account1",
			Share: &Share{
				Percentage:             50,
				PercentageOfPercentage: 100,
			},
		},
		{
			Account: "@account2",
			Amount: &Amount{
				Asset: "USD",
				Scale: 2,
				Value: 5000,
			},
		},
		{
			Account:   "@account3",
			Remaining: "remaining",
		},
	}

	send := Send{
		Asset: "USD",
		Scale: 2,
		Value: 10000,
	}

	tChan := make(chan int)
	ftChan := make(chan map[string]Amount)
	sdChan := make(chan []string)

	go calculateTotal(fromTos, send, tChan, ftChan, sdChan)

	ttl := <-tChan
	fmto := <-ftChan
	sources := <-sdChan

	expectedTotal := 10000
	if ttl != expectedTotal {
		t.Errorf("Expected total %d, got %d", expectedTotal, ttl)
	}

	expectedFmto := map[string]Amount{
		"@account1": {Asset: "USD", Scale: 2, Value: 5000},
		"@account2": {Asset: "USD", Scale: 2, Value: 5000},
		"@account3": {Asset: "USD", Scale: 2, Value: 0},
	}
	if !compareAmountMaps(fmto, expectedFmto) {
		t.Errorf("Expected fmto %v, got %v", expectedFmto, fmto)
	}

	expectedSources := []string{"@account1", "@account2", "@account3"}
	if !compareSlices(sources, expectedSources) {
		t.Errorf("Expected sources %v, got %v", expectedSources, sources)
	}
}

func compareAmountMaps(a, b map[string]Amount) bool {
	if len(a) != len(b) {
		return false
	}
	for key, valueA := range a {
		if valueB, ok := b[key]; !ok || valueA != valueB {
			return false
		}
	}
	return true
}

func TestValidateSendSourceAndDistribute(t *testing.T) {
	t.Run("case 01 success", func(t *testing.T) {
		mockTransaction := Transaction{
			Description: "Test Transaction",
			Code:        "00000000-0000-0000-0000-000000000000",
			Send: Send{
				Value: 100,
				Source: Source{
					From: []FromTo{
						{
							Account: "@account1",
							Amount:  &Amount{Value: 50},
						},
						{
							Account: "@account2",
							Amount:  &Amount{Value: 50},
						},
					},
				},
			},
			Distribute: Distribute{
				To: []FromTo{
					{
						Account: "@account3",
						Amount:  &Amount{Value: 60},
					},
					{
						Account: "@account4",
						Amount:  &Amount{Value: 40},
					},
				},
			},
		}

		expectedResponse := &Responses{
			Total: 100,
			From: map[string]Amount{
				"@account1": {Value: 50},
				"@account2": {Value: 50},
			},
			To: map[string]Amount{
				"@account3": {Value: 60},
				"@account4": {Value: 40},
			},
			Sources:      []string{"@account1", "@account2"},
			Destinations: []string{"@account3", "@account4"},
			Aliases:      []string{"@account1", "@account2", "@account3", "@account4"},
		}

		response, err := ValidateSendSourceAndDistribute(mockTransaction)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verifica os valores da resposta
		if response.Total != expectedResponse.Total {
			t.Errorf("Expected total %d, got %d", expectedResponse.Total, response.Total)
		}

		if !compareMaps(response.From, expectedResponse.From) {
			t.Errorf("Expected From %v, got %v", expectedResponse.From, response.From)
		}

		if !compareMaps(response.To, expectedResponse.To) {
			t.Errorf("Expected To %v, got %v", expectedResponse.To, response.To)
		}

		if !compareSlices(response.Sources, expectedResponse.Sources) {
			t.Errorf("Expected Sources %v, got %v", expectedResponse.Sources, response.Sources)
		}

		if !compareSlices(response.Destinations, expectedResponse.Destinations) {
			t.Errorf("Expected Destinations %v, got %v", expectedResponse.Destinations, response.Destinations)
		}

		if !compareSlices(response.Aliases, expectedResponse.Aliases) {
			t.Errorf("Expected Aliases %v, got %v", expectedResponse.Aliases, response.Aliases)
		}
	})

	t.Run("case 02 error", func(t *testing.T) {
		mockTransaction := Transaction{
			Description: "Test Transaction",
			Code:        "00000000-0000-0000-0000-000000000000",
			Send: Send{
				Value: 100,
				Source: Source{
					From: []FromTo{
						{
							Account: "@account1",
							Amount:  &Amount{Value: 49},
						},
						{
							Account: "@account2",
							Amount:  &Amount{Value: 50},
						},
					},
				},
			},
			Distribute: Distribute{
				To: []FromTo{
					{
						Account: "@account3",
						Amount:  &Amount{Value: 60},
					},
					{
						Account: "@account4",
						Amount:  &Amount{Value: 40},
					},
				},
			},
		}

		_, err := ValidateSendSourceAndDistribute(mockTransaction)
		require.Error(t, err)
	})

	t.Run("case 03 error", func(t *testing.T) {
		mockTransaction := Transaction{
			Description: "Test Transaction",
			Code:        "00000000-0000-0000-0000-000000000000",
			Send: Send{
				Value: 100,
				Source: Source{
					From: []FromTo{
						{
							Account: "@account1",
							Amount:  &Amount{Value: 50},
						},
						{
							Account: "@account2",
							Amount:  &Amount{Value: 50},
						},
					},
				},
			},
			Distribute: Distribute{
				To: []FromTo{
					{
						Account: "@account3",
						Amount:  &Amount{Value: 41},
					},
					{
						Account: "@account4",
						Amount:  &Amount{Value: 40},
					},
				},
			},
		}

		_, err := ValidateSendSourceAndDistribute(mockTransaction)
		require.Error(t, err)
	})
}

func compareMaps(a, b map[string]Amount) bool {
	if len(a) != len(b) {
		return false
	}
	for key, valueA := range a {
		if valueB, ok := b[key]; !ok || valueA.Value != valueB.Value {
			return false
		}
	}
	return true
}

func compareSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestValidateAccountsFrom(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		account     *a.Account
		expectError bool
		expectedErr error
	}{
		{
			name: "case 00",
			key:  "123",
			account: &a.Account{
				Id:             "124",
				Alias:          "@external",
				AllowSending:   true,
				AllowReceiving: false,
				Balance:        &a.Balance{Available: 100},
			},
			expectError: false,
			expectedErr: nil,
		},
		{
			name: "case 01",
			key:  "123",
			account: &a.Account{
				Id:             "123",
				Alias:          "alias123",
				AllowSending:   false,
				Balance:        &a.Balance{Available: 100},
				AllowReceiving: true,
			},
			expectError: true,
			expectedErr: pkg.ValidateBusinessError(cn.ErrAccountStatusTransactionRestriction, "ValidateAccounts"),
		},
		{
			name: "case 02",
			key:  "123",
			account: &a.Account{
				Id:             "124",
				Alias:          "123",
				AllowSending:   true,
				Balance:        &a.Balance{Available: 0},
				AllowReceiving: true,
			},
			expectError: true,
			expectedErr: pkg.ValidateBusinessError(cn.ErrInsufficientFunds, "ValidateAccounts", "123"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAccountsFrom(tt.key, tt.account)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.expectedErr.Error()) {
					t.Errorf("Expected error %v, got %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func Test_validateAccountsTo(t *testing.T) {
	tests := []struct {
		key         string
		account     *a.Account
		expectError bool
	}{
		{
			key: "123",
			account: &a.Account{
				Id:             "123",
				Alias:          "alias123",
				AllowReceiving: false,
			},
			expectError: true,
		},
		{
			key: "alias123",
			account: &a.Account{
				Id:             "456",
				Alias:          "alias123",
				AllowReceiving: true,
			},
			expectError: false,
		},
		{
			key: "789",
			account: &a.Account{
				Id:             "789",
				Alias:          "alias789",
				AllowReceiving: true,
			},
			expectError: true,
		},
	}
	for _, tt := range tests {
		err := validateAccountsTo(tt.key, tt.account)
		if (err != nil) != tt.expectError {
			t.Errorf("validateAccountsTo(%q, %v) = %v; want error: %v",
				tt.key, tt.account, err, tt.expectError)
		}
	}
}
