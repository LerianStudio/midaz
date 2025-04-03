package utils_test

import (
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/internal/utils"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/assert"
)

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}

func TestGetAccountIdentifier(t *testing.T) {
	testCases := []struct {
		name           string
		account        *models.Account
		expectedResult string
	}{
		{
			name:           "Nil account",
			account:        nil,
			expectedResult: "",
		},
		{
			name: "Account with alias",
			account: &models.Account{
				ID:    "acc_123",
				Alias: strPtr("savings"),
			},
			expectedResult: "savings",
		},
		{
			name: "Account with nil alias",
			account: &models.Account{
				ID:    "acc_123",
				Alias: nil,
			},
			expectedResult: "acc_123",
		},
		{
			name: "Account with empty alias",
			account: &models.Account{
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
	accounts := []models.Account{
		{
			ID:    "acc_123",
			Alias: strPtr("savings"),
		},
		{
			ID:    "acc_456",
			Alias: strPtr("checking"),
		},
		{
			ID:    "acc_789",
			Alias: strPtr("investment"),
		},
	}

	testCases := []struct {
		name           string
		accounts       []models.Account
		id             string
		expectedResult *models.Account
	}{
		{
			name:           "Empty accounts",
			accounts:       []models.Account{},
			id:             "acc_123",
			expectedResult: nil,
		},
		{
			name:           "Nil accounts",
			accounts:       nil,
			id:             "acc_123",
			expectedResult: nil,
		},
		{
			name:           "Account found",
			accounts:       accounts,
			id:             "acc_456",
			expectedResult: &accounts[1],
		},
		{
			name:           "Account not found",
			accounts:       accounts,
			id:             "acc_999",
			expectedResult: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FindAccountByID(tc.accounts, tc.id)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestFindAccountByAlias(t *testing.T) {
	accounts := []models.Account{
		{
			ID:    "acc_123",
			Alias: strPtr("savings"),
		},
		{
			ID:    "acc_456",
			Alias: strPtr("checking"),
		},
		{
			ID:    "acc_789",
			Alias: nil,
		},
	}

	testCases := []struct {
		name           string
		accounts       []models.Account
		alias          string
		expectedResult *models.Account
	}{
		{
			name:           "Empty accounts",
			accounts:       []models.Account{},
			alias:          "savings",
			expectedResult: nil,
		},
		{
			name:           "Nil accounts",
			accounts:       nil,
			alias:          "savings",
			expectedResult: nil,
		},
		{
			name:           "Account found",
			accounts:       accounts,
			alias:          "checking",
			expectedResult: &accounts[1],
		},
		{
			name:           "Account not found",
			accounts:       accounts,
			alias:          "nonexistent",
			expectedResult: nil,
		},
		{
			name:           "Empty alias",
			accounts:       accounts,
			alias:          "",
			expectedResult: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FindAccountByAlias(tc.accounts, tc.alias)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestFindAccountsByAssetCode(t *testing.T) {
	accounts := []models.Account{
		{
			ID:        "acc_123",
			Alias:     strPtr("savings_usd"),
			AssetCode: "USD",
		},
		{
			ID:        "acc_456",
			Alias:     strPtr("checking_usd"),
			AssetCode: "USD",
		},
		{
			ID:        "acc_789",
			Alias:     strPtr("savings_eur"),
			AssetCode: "EUR",
		},
		{
			ID:        "acc_012",
			Alias:     strPtr("investment_btc"),
			AssetCode: "BTC",
		},
	}

	testCases := []struct {
		name           string
		accounts       []models.Account
		assetCode      string
		expectedCount  int
		expectedResult []models.Account
	}{
		{
			name:           "Empty accounts",
			accounts:       []models.Account{},
			assetCode:      "USD",
			expectedCount:  0,
			expectedResult: []models.Account{},
		},
		{
			name:           "Nil accounts",
			accounts:       nil,
			assetCode:      "USD",
			expectedCount:  0,
			expectedResult: nil,
		},
		{
			name:           "Filter USD accounts",
			accounts:       accounts,
			assetCode:      "USD",
			expectedCount:  2,
			expectedResult: []models.Account{accounts[0], accounts[1]},
		},
		{
			name:           "Filter EUR accounts",
			accounts:       accounts,
			assetCode:      "EUR",
			expectedCount:  1,
			expectedResult: []models.Account{accounts[2]},
		},
		{
			name:           "Filter BTC accounts",
			accounts:       accounts,
			assetCode:      "BTC",
			expectedCount:  1,
			expectedResult: []models.Account{accounts[3]},
		},
		{
			name:           "Filter nonexistent asset code",
			accounts:       accounts,
			assetCode:      "XYZ",
			expectedCount:  0,
			expectedResult: []models.Account{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FindAccountsByAssetCode(tc.accounts, tc.assetCode)
			assert.Equal(t, tc.expectedCount, len(result))
			// For empty results, just check the length is 0 instead of comparing exact value
			// since the function might return nil instead of empty slice
			if tc.expectedCount == 0 {
				assert.Empty(t, result, "Expected empty result")
			} else {
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestFindAccountsByStatus(t *testing.T) {
	accounts := []models.Account{
		{
			ID:     "acc_123",
			Alias:  strPtr("savings"),
			Status: models.Status{Code: "ACTIVE"},
		},
		{
			ID:     "acc_456",
			Alias:  strPtr("checking"),
			Status: models.Status{Code: "ACTIVE"},
		},
		{
			ID:     "acc_789",
			Alias:  strPtr("closed"),
			Status: models.Status{Code: "CLOSED"},
		},
		{
			ID:     "acc_012",
			Alias:  strPtr("pending"),
			Status: models.Status{Code: "PENDING"},
		},
	}

	testCases := []struct {
		name           string
		accounts       []models.Account
		status         string
		expectedCount  int
		expectedResult []models.Account
	}{
		{
			name:           "Empty accounts",
			accounts:       []models.Account{},
			status:         "ACTIVE",
			expectedCount:  0,
			expectedResult: []models.Account{},
		},
		{
			name:           "Nil accounts",
			accounts:       nil,
			status:         "ACTIVE",
			expectedCount:  0,
			expectedResult: nil,
		},
		{
			name:           "Filter active accounts",
			accounts:       accounts,
			status:         "ACTIVE",
			expectedCount:  2,
			expectedResult: []models.Account{accounts[0], accounts[1]},
		},
		{
			name:           "Filter closed accounts",
			accounts:       accounts,
			status:         "CLOSED",
			expectedCount:  1,
			expectedResult: []models.Account{accounts[2]},
		},
		{
			name:           "Filter pending accounts",
			accounts:       accounts,
			status:         "PENDING",
			expectedCount:  1,
			expectedResult: []models.Account{accounts[3]},
		},
		{
			name:           "Filter nonexistent status",
			accounts:       accounts,
			status:         "NONEXISTENT",
			expectedCount:  0,
			expectedResult: []models.Account{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FindAccountsByStatus(tc.accounts, tc.status)
			assert.Equal(t, tc.expectedCount, len(result))
			// For empty results, just check the length is 0 instead of comparing exact value
			// since the function might return nil instead of empty slice
			if tc.expectedCount == 0 {
				assert.Empty(t, result, "Expected empty result")
			} else {
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestFilterAccounts(t *testing.T) {
	accounts := []models.Account{
		{
			ID:        "acc_123",
			Alias:     strPtr("savings_usd"),
			AssetCode: "USD",
			Type:      "SAVINGS",
			Status:    models.Status{Code: "ACTIVE"},
		},
		{
			ID:        "acc_456",
			Alias:     strPtr("checking_usd"),
			AssetCode: "USD",
			Type:      "CHECKING",
			Status:    models.Status{Code: "ACTIVE"},
		},
		{
			ID:        "acc_789",
			Alias:     strPtr("savings_eur"),
			AssetCode: "EUR",
			Type:      "SAVINGS",
			Status:    models.Status{Code: "ACTIVE"},
		},
		{
			ID:        "acc_012",
			Alias:     strPtr("closed_usd"),
			AssetCode: "USD",
			Type:      "SAVINGS",
			Status:    models.Status{Code: "CLOSED"},
		},
	}

	testCases := []struct {
		name           string
		accounts       []models.Account
		filters        map[string]string
		expectedCount  int
		expectedResult []models.Account
	}{
		{
			name:           "Empty accounts",
			accounts:       []models.Account{},
			filters:        map[string]string{"assetCode": "USD"},
			expectedCount:  0,
			expectedResult: []models.Account{},
		},
		{
			name:           "Nil accounts",
			accounts:       nil,
			filters:        map[string]string{"assetCode": "USD"},
			expectedCount:  0,
			expectedResult: nil,
		},
		{
			name:           "Empty filters",
			accounts:       accounts,
			filters:        map[string]string{},
			expectedCount:  4,
			expectedResult: accounts,
		},
		{
			name:           "Filter by asset code",
			accounts:       accounts,
			filters:        map[string]string{"assetCode": "USD"},
			expectedCount:  3,
			expectedResult: []models.Account{accounts[0], accounts[1], accounts[3]},
		},
		{
			name:           "Filter by status",
			accounts:       accounts,
			filters:        map[string]string{"status": "ACTIVE"},
			expectedCount:  3,
			expectedResult: []models.Account{accounts[0], accounts[1], accounts[2]},
		},
		{
			name:           "Filter by type",
			accounts:       accounts,
			filters:        map[string]string{"type": "SAVINGS"},
			expectedCount:  3,
			expectedResult: []models.Account{accounts[0], accounts[2], accounts[3]},
		},
		{
			name:           "Filter by multiple criteria",
			accounts:       accounts,
			filters:        map[string]string{"assetCode": "USD", "status": "ACTIVE", "type": "SAVINGS"},
			expectedCount:  1,
			expectedResult: []models.Account{accounts[0]},
		},
		{
			name:           "Filter by alias contains",
			accounts:       accounts,
			filters:        map[string]string{"aliasContains": "savings"},
			expectedCount:  2,
			expectedResult: []models.Account{accounts[0], accounts[2]},
		},
		{
			name:           "No matching accounts",
			accounts:       accounts,
			filters:        map[string]string{"assetCode": "XYZ"},
			expectedCount:  0,
			expectedResult: []models.Account{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FilterAccounts(tc.accounts, tc.filters)
			assert.Len(t, result, tc.expectedCount)
			// For empty results, just check the length is 0 instead of comparing exact value
			// since the function might return nil instead of empty slice
			if tc.expectedCount == 0 {
				assert.Empty(t, result, "Expected empty result")
			} else {
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestFormatAccountSummary(t *testing.T) {
	testCases := []struct {
		name           string
		account        *models.Account
		expectedResult string
	}{
		{
			name:           "Nil account",
			account:        nil,
			expectedResult: "Account: <nil>",
		},
		{
			name: "Account with all fields",
			account: &models.Account{
				ID:        "acc_123",
				Alias:     strPtr("savings"),
				AssetCode: "USD",
				Type:      "SAVINGS",
				Status:    models.Status{Code: "ACTIVE"},
			},
			expectedResult: "Account: savings (acc_123) - Type: SAVINGS - Asset: USD - Status: ACTIVE",
		},
		{
			name: "Account with nil alias",
			account: &models.Account{
				ID:        "acc_123",
				Alias:     nil,
				AssetCode: "USD",
				Type:      "SAVINGS",
				Status:    models.Status{Code: "ACTIVE"},
			},
			expectedResult: "Account: <no alias> (acc_123) - Type: SAVINGS - Asset: USD - Status: ACTIVE",
		},
		{
			name: "Account with empty status",
			account: &models.Account{
				ID:        "acc_123",
				Alias:     strPtr("savings"),
				AssetCode: "USD",
				Type:      "SAVINGS",
				Status:    models.Status{Code: ""},
			},
			expectedResult: "Account: savings (acc_123) - Type: SAVINGS - Asset: USD - Status: <no status>",
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
		account        *models.Account
		balance        *models.Balance
		expectedError  bool
		expectedResult utils.AccountBalanceSummary
	}{
		{
			name:          "Nil account",
			account:       nil,
			balance:       &models.Balance{AccountID: "acc_123", AssetCode: "USD", Available: 1000, OnHold: 500, Scale: 2},
			expectedError: true,
		},
		{
			name: "Valid account and balance",
			account: &models.Account{
				ID:        "acc_123",
				Alias:     strPtr("savings"),
				AssetCode: "USD",
			},
			balance: &models.Balance{
				AccountID: "acc_123",
				AssetCode: "USD",
				Available: 1000,
				OnHold:    500,
				Scale:     2,
			},
			expectedError: false,
			expectedResult: utils.AccountBalanceSummary{
				AccountID:    "acc_123",
				AccountAlias: "savings",
				AssetCode:    "USD",
				Available:    1000,
				AvailableStr: "10.00",
				OnHold:       500,
				OnHoldStr:    "5.00",
				Total:        1500,
				TotalStr:     "15.00",
				Scale:        2,
			},
		},
		{
			name: "Account with nil alias",
			account: &models.Account{
				ID:        "acc_123",
				Alias:     nil,
				AssetCode: "USD",
			},
			balance: &models.Balance{
				AccountID: "acc_123",
				AssetCode: "USD",
				Available: 1000,
				OnHold:    500,
				Scale:     2,
			},
			expectedError: false,
			expectedResult: utils.AccountBalanceSummary{
				AccountID:    "acc_123",
				AccountAlias: "",
				AssetCode:    "USD",
				Available:    1000,
				AvailableStr: "10.00",
				OnHold:       500,
				OnHoldStr:    "5.00",
				Total:        1500,
				TotalStr:     "15.00",
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
