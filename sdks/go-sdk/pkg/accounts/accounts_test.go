package accounts_test

import (
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/pkg/accounts"
	"github.com/stretchr/testify/assert"
)

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}

func TestGetAccountIdentifier(t *testing.T) {
	testCases := []struct {
		name           string
		account        *accounts.Account
		expectedResult string
	}{
		{
			name:           "Nil account",
			account:        nil,
			expectedResult: "",
		},
		{
			name: "Account with alias",
			account: &accounts.Account{
				ID:    "acc_123",
				Alias: strPtr("savings"),
			},
			expectedResult: "savings",
		},
		{
			name: "Account with nil alias",
			account: &accounts.Account{
				ID:    "acc_123",
				Alias: nil,
			},
			expectedResult: "acc_123",
		},
		{
			name: "Account with empty alias",
			account: &accounts.Account{
				ID:    "acc_123",
				Alias: strPtr(""),
			},
			expectedResult: "acc_123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := accounts.GetAccountIdentifier(tc.account)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestFindAccountByID(t *testing.T) {
	testCases := []struct {
		name          string
		accounts      []accounts.Account
		id            string
		expectedFound bool
	}{
		{
			name:          "Empty accounts",
			accounts:      []accounts.Account{},
			id:            "acc_123",
			expectedFound: false,
		},
		{
			name: "Account found",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
				},
			},
			id:            "acc_123",
			expectedFound: true,
		},
		{
			name: "Account not found",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
				},
			},
			id:            "acc_789",
			expectedFound: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := accounts.FindAccountByID(tc.accounts, tc.id)
			if tc.expectedFound {
				assert.NotNil(t, result)
				assert.Equal(t, tc.id, result.ID)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestFindAccountByAlias(t *testing.T) {
	testCases := []struct {
		name          string
		accounts      []accounts.Account
		alias         string
		expectedFound bool
	}{
		{
			name:          "Empty accounts",
			accounts:      []accounts.Account{},
			alias:         "savings",
			expectedFound: false,
		},
		{
			name: "Account found",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("savings"),
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("checking"),
				},
			},
			alias:         "savings",
			expectedFound: true,
		},
		{
			name: "Account not found",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("savings"),
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("checking"),
				},
			},
			alias:         "investment",
			expectedFound: false,
		},
		{
			name: "Nil alias",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     nil,
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("checking"),
				},
			},
			alias:         "savings",
			expectedFound: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := accounts.FindAccountByAlias(tc.accounts, tc.alias)
			if tc.expectedFound {
				assert.NotNil(t, result)
				assert.Equal(t, tc.alias, *result.Alias)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestFindAccountsByAssetCode(t *testing.T) {
	testCases := []struct {
		name            string
		accounts        []accounts.Account
		assetCode       string
		expectedCount   int
		expectedCodes   []string
		expectedIDs     []string
		expectedAliases []string
	}{
		{
			name:            "Empty accounts",
			accounts:        []accounts.Account{},
			assetCode:       "USD",
			expectedCount:   0,
			expectedCodes:   []string{},
			expectedIDs:     []string{},
			expectedAliases: []string{},
		},
		{
			name: "Multiple accounts found",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "FROZEN"},
					Alias:     strPtr("usd_checking"),
				},
			},
			assetCode:       "USD",
			expectedCount:   2,
			expectedCodes:   []string{"USD", "USD"},
			expectedIDs:     []string{"acc_123", "acc_789"},
			expectedAliases: []string{"usd_savings", "usd_checking"},
		},
		{
			name: "No accounts found",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
			},
			assetCode:       "GBP",
			expectedCount:   0,
			expectedCodes:   []string{},
			expectedIDs:     []string{},
			expectedAliases: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := accounts.FindAccountsByAssetCode(tc.accounts, tc.assetCode)
			assert.Equal(t, tc.expectedCount, len(result))

			for i, account := range result {
				assert.Equal(t, tc.expectedCodes[i], account.AssetCode)
				assert.Equal(t, tc.expectedIDs[i], account.ID)
				assert.Equal(t, tc.expectedAliases[i], *account.Alias)
			}
		})
	}
}

func TestFindAccountsByStatus(t *testing.T) {
	testCases := []struct {
		name             string
		accounts         []accounts.Account
		status           string
		expectedCount    int
		expectedIDs      []string
		expectedStatuses []string
	}{
		{
			name:             "Empty accounts",
			accounts:         []accounts.Account{},
			status:           "ACTIVE",
			expectedCount:    0,
			expectedIDs:      []string{},
			expectedStatuses: []string{},
		},
		{
			name: "Multiple accounts found",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "FROZEN"},
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
				},
			},
			status:           "ACTIVE",
			expectedCount:    2,
			expectedIDs:      []string{"acc_123", "acc_789"},
			expectedStatuses: []string{"ACTIVE", "ACTIVE"},
		},
		{
			name: "No accounts found",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "FROZEN"},
				},
			},
			status:           "CLOSED",
			expectedCount:    0,
			expectedIDs:      []string{},
			expectedStatuses: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := accounts.FindAccountsByStatus(tc.accounts, tc.status)
			assert.Equal(t, tc.expectedCount, len(result))

			for i, account := range result {
				assert.Equal(t, tc.expectedIDs[i], account.ID)
				assert.Equal(t, tc.expectedStatuses[i], account.Status.Code)
			}
		})
	}
}

func TestFilterAccounts(t *testing.T) {
	testCases := []struct {
		name          string
		accounts      []accounts.Account
		filters       map[string]string
		expectedCount int
		expectedIDs   []string
	}{
		{
			name:          "Empty accounts",
			accounts:      []accounts.Account{},
			filters:       map[string]string{"assetCode": "USD"},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name: "Filter by asset code",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "FROZEN"},
					Alias:     strPtr("usd_checking"),
				},
			},
			filters:       map[string]string{"assetCode": "USD"},
			expectedCount: 2,
			expectedIDs:   []string{"acc_123", "acc_789"},
		},
		{
			name: "Filter by asset code and status",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "FROZEN"},
					Alias:     strPtr("usd_checking"),
				},
			},
			filters:       map[string]string{"assetCode": "USD", "status": "ACTIVE"},
			expectedCount: 1,
			expectedIDs:   []string{"acc_123"},
		},
		{
			name: "Filter by alias contains",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "FROZEN"},
					Alias:     strPtr("usd_checking"),
				},
			},
			filters:       map[string]string{"aliasContains": "checking"},
			expectedCount: 2,
			expectedIDs:   []string{"acc_456", "acc_789"},
		},
		{
			name: "No matching accounts",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
			},
			filters:       map[string]string{"assetCode": "GBP"},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name: "Empty filters",
			accounts: []accounts.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    accounts.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
			},
			filters:       map[string]string{},
			expectedCount: 2,
			expectedIDs:   []string{"acc_123", "acc_456"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := accounts.FilterAccounts(tc.accounts, tc.filters)
			assert.Equal(t, tc.expectedCount, len(result))

			for i, account := range result {
				assert.Equal(t, tc.expectedIDs[i], account.ID)
			}
		})
	}
}

func TestFormatAccountSummary(t *testing.T) {
	testCases := []struct {
		name           string
		account        *accounts.Account
		expectedResult string
	}{
		{
			name:           "Nil account",
			account:        nil,
			expectedResult: "Account: <nil>",
		},
		{
			name: "Account with all fields",
			account: &accounts.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Type:      "ASSET",
				Status:    accounts.Status{Code: "ACTIVE"},
				Alias:     strPtr("savings"),
			},
			expectedResult: "Account: savings (acc_123) - Type: ASSET - Asset: USD - Status: ACTIVE",
		},
		{
			name: "Account with nil alias",
			account: &accounts.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Type:      "ASSET",
				Status:    accounts.Status{Code: "ACTIVE"},
				Alias:     nil,
			},
			expectedResult: "Account: <no alias> (acc_123) - Type: ASSET - Asset: USD - Status: ACTIVE",
		},
		{
			name: "Account with empty status",
			account: &accounts.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Type:      "ASSET",
				Status:    accounts.Status{Code: ""},
				Alias:     strPtr("savings"),
			},
			expectedResult: "Account: savings (acc_123) - Type: ASSET - Asset: USD - Status: <no status>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := accounts.FormatAccountSummary(tc.account)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestGetAccountBalanceSummary(t *testing.T) {
	testCases := []struct {
		name           string
		account        *accounts.Account
		balance        *accounts.Balance
		expectedError  bool
		expectedResult accounts.AccountBalanceSummary
	}{
		{
			name:          "Nil account",
			account:       nil,
			balance:       &accounts.Balance{ID: "bal_123", AccountID: "acc_123", AssetCode: "USD", Available: 10000, OnHold: 500, Scale: 2},
			expectedError: true,
		},
		{
			name: "Valid account and balance",
			account: &accounts.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Alias:     strPtr("savings"),
			},
			balance: &accounts.Balance{
				ID:        "bal_123",
				AccountID: "acc_123",
				AssetCode: "USD",
				Available: 10000,
				OnHold:    500,
				Scale:     2,
			},
			expectedError: false,
			expectedResult: accounts.AccountBalanceSummary{
				AccountID:    "acc_123",
				AccountAlias: "savings",
				AssetCode:    "USD",
				Available:    10000,
				AvailableStr: "100.00",
				OnHold:       500,
				OnHoldStr:    "5.00",
				Total:        10500,
				TotalStr:     "105.00",
				Scale:        2,
			},
		},
		{
			name: "Account with nil alias",
			account: &accounts.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Alias:     nil,
			},
			balance: &accounts.Balance{
				ID:        "bal_123",
				AccountID: "acc_123",
				AssetCode: "USD",
				Available: 10000,
				OnHold:    500,
				Scale:     2,
			},
			expectedError: false,
			expectedResult: accounts.AccountBalanceSummary{
				AccountID:    "acc_123",
				AccountAlias: "",
				AssetCode:    "USD",
				Available:    10000,
				AvailableStr: "100.00",
				OnHold:       500,
				OnHoldStr:    "5.00",
				Total:        10500,
				TotalStr:     "105.00",
				Scale:        2,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := accounts.GetAccountBalanceSummary(tc.account, tc.balance)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult.AccountID, result.AccountID)
				assert.Equal(t, tc.expectedResult.AccountAlias, result.AccountAlias)
				assert.Equal(t, tc.expectedResult.AssetCode, result.AssetCode)
				assert.Equal(t, tc.expectedResult.Available, result.Available)
				assert.Equal(t, tc.expectedResult.AvailableStr, result.AvailableStr)
				assert.Equal(t, tc.expectedResult.OnHold, result.OnHold)
				assert.Equal(t, tc.expectedResult.OnHoldStr, result.OnHoldStr)
				assert.Equal(t, tc.expectedResult.Total, result.Total)
				assert.Equal(t, tc.expectedResult.TotalStr, result.TotalStr)
				assert.Equal(t, tc.expectedResult.Scale, result.Scale)
			}
		})
	}
}
