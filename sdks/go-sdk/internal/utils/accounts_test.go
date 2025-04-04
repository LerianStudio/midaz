package utils_test

import (
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/internal/utils"
	"github.com/stretchr/testify/assert"
)

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}

func TestGetAccountIdentifier(t *testing.T) {
	testCases := []struct {
		name           string
		account        *utils.Account
		expectedResult string
	}{
		{
			name:           "Nil account",
			account:        nil,
			expectedResult: "",
		},
		{
			name: "Account with alias",
			account: &utils.Account{
				ID:    "acc_123",
				Alias: strPtr("savings"),
			},
			expectedResult: "savings",
		},
		{
			name: "Account with nil alias",
			account: &utils.Account{
				ID:    "acc_123",
				Alias: nil,
			},
			expectedResult: "acc_123",
		},
		{
			name: "Account with empty alias",
			account: &utils.Account{
				ID:    "acc_123",
				Alias: strPtr(""),
			},
			expectedResult: "acc_123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.GetAccountIdentifier(tc.account)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestFindAccountByID(t *testing.T) {
	testCases := []struct {
		name          string
		accounts      []utils.Account
		id            string
		expectedFound bool
	}{
		{
			name:          "Empty accounts",
			accounts:      []utils.Account{},
			id:            "acc_123",
			expectedFound: false,
		},
		{
			name: "Account found",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
				},
			},
			id:            "acc_123",
			expectedFound: true,
		},
		{
			name: "Account not found",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
				},
			},
			id:            "acc_789",
			expectedFound: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FindAccountByID(tc.accounts, tc.id)
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
		accounts      []utils.Account
		alias         string
		expectedFound bool
	}{
		{
			name:          "Empty accounts",
			accounts:      []utils.Account{},
			alias:         "savings",
			expectedFound: false,
		},
		{
			name: "Account found",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("savings"),
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("checking"),
				},
			},
			alias:         "savings",
			expectedFound: true,
		},
		{
			name: "Account not found",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("savings"),
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("checking"),
				},
			},
			alias:         "investment",
			expectedFound: false,
		},
		{
			name: "Nil alias",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     nil,
				},
				{
					ID:        "acc_456",
					Name:      "Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("checking"),
				},
			},
			alias:         "savings",
			expectedFound: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FindAccountByAlias(tc.accounts, tc.alias)
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
		accounts        []utils.Account
		assetCode       string
		expectedCount   int
		expectedCodes   []string
		expectedIDs     []string
		expectedAliases []string
	}{
		{
			name:            "Empty accounts",
			accounts:        []utils.Account{},
			assetCode:       "USD",
			expectedCount:   0,
			expectedCodes:   []string{},
			expectedIDs:     []string{},
			expectedAliases: []string{},
		},
		{
			name: "Multiple accounts found",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
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
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
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
			result := utils.FindAccountsByAssetCode(tc.accounts, tc.assetCode)
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
		accounts         []utils.Account
		status           string
		expectedCount    int
		expectedIDs      []string
		expectedStatuses []string
	}{
		{
			name:             "Empty accounts",
			accounts:         []utils.Account{},
			status:           "ACTIVE",
			expectedCount:    0,
			expectedIDs:      []string{},
			expectedStatuses: []string{},
		},
		{
			name: "Multiple accounts found",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "FROZEN"},
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
				},
			},
			status:           "ACTIVE",
			expectedCount:    2,
			expectedIDs:      []string{"acc_123", "acc_789"},
			expectedStatuses: []string{"ACTIVE", "ACTIVE"},
		},
		{
			name: "No accounts found",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "FROZEN"},
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
			result := utils.FindAccountsByStatus(tc.accounts, tc.status)
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
		accounts      []utils.Account
		filters       map[string]string
		expectedCount int
		expectedIDs   []string
	}{
		{
			name:          "Empty accounts",
			accounts:      []utils.Account{},
			filters:       map[string]string{"assetCode": "USD"},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name: "Filter by asset code",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "FROZEN"},
					Alias:     strPtr("usd_checking"),
				},
			},
			filters:       map[string]string{"assetCode": "USD"},
			expectedCount: 2,
			expectedIDs:   []string{"acc_123", "acc_789"},
		},
		{
			name: "Filter by asset code and status",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "FROZEN"},
					Alias:     strPtr("usd_checking"),
				},
			},
			filters:       map[string]string{"assetCode": "USD", "status": "ACTIVE"},
			expectedCount: 1,
			expectedIDs:   []string{"acc_123"},
		},
		{
			name: "Filter by alias contains",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
				{
					ID:        "acc_789",
					Name:      "USD Checking",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "FROZEN"},
					Alias:     strPtr("usd_checking"),
				},
			},
			filters:       map[string]string{"aliasContains": "checking"},
			expectedCount: 2,
			expectedIDs:   []string{"acc_456", "acc_789"},
		},
		{
			name: "No matching accounts",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("eur_checking"),
				},
			},
			filters:       map[string]string{"assetCode": "GBP"},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name: "Empty filters",
			accounts: []utils.Account{
				{
					ID:        "acc_123",
					Name:      "USD Savings",
					AssetCode: "USD",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
					Alias:     strPtr("usd_savings"),
				},
				{
					ID:        "acc_456",
					Name:      "EUR Checking",
					AssetCode: "EUR",
					Type:      "ASSET",
					Status:    utils.Status{Code: "ACTIVE"},
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
			result := utils.FilterAccounts(tc.accounts, tc.filters)
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
		account        *utils.Account
		expectedResult string
	}{
		{
			name:           "Nil account",
			account:        nil,
			expectedResult: "Account: <nil>",
		},
		{
			name: "Account with all fields",
			account: &utils.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Type:      "ASSET",
				Status:    utils.Status{Code: "ACTIVE"},
				Alias:     strPtr("savings"),
			},
			expectedResult: "Account: savings (acc_123) - Type: ASSET - Asset: USD - Status: ACTIVE",
		},
		{
			name: "Account with nil alias",
			account: &utils.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Type:      "ASSET",
				Status:    utils.Status{Code: "ACTIVE"},
				Alias:     nil,
			},
			expectedResult: "Account: <no alias> (acc_123) - Type: ASSET - Asset: USD - Status: ACTIVE",
		},
		{
			name: "Account with empty status",
			account: &utils.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Type:      "ASSET",
				Status:    utils.Status{Code: ""},
				Alias:     strPtr("savings"),
			},
			expectedResult: "Account: savings (acc_123) - Type: ASSET - Asset: USD - Status: <no status>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FormatAccountSummary(tc.account)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestGetAccountBalanceSummary(t *testing.T) {
	testCases := []struct {
		name           string
		account        *utils.Account
		balance        *utils.Balance
		expectedError  bool
		expectedResult utils.AccountBalanceSummary
	}{
		{
			name:          "Nil account",
			account:       nil,
			balance:       &utils.Balance{ID: "bal_123", AccountID: "acc_123", AssetCode: "USD", Available: 10000, OnHold: 500, Scale: 2},
			expectedError: true,
		},
		{
			name: "Valid account and balance",
			account: &utils.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Alias:     strPtr("savings"),
			},
			balance: &utils.Balance{
				ID:        "bal_123",
				AccountID: "acc_123",
				AssetCode: "USD",
				Available: 10000,
				OnHold:    500,
				Scale:     2,
			},
			expectedError: false,
			expectedResult: utils.AccountBalanceSummary{
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
			account: &utils.Account{
				ID:        "acc_123",
				Name:      "Savings Account",
				AssetCode: "USD",
				Alias:     nil,
			},
			balance: &utils.Balance{
				ID:        "bal_123",
				AccountID: "acc_123",
				AssetCode: "USD",
				Available: 10000,
				OnHold:    500,
				Scale:     2,
			},
			expectedError: false,
			expectedResult: utils.AccountBalanceSummary{
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
			result, err := utils.GetAccountBalanceSummary(tc.account, tc.balance)
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
