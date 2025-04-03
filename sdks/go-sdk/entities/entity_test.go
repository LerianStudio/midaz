package entities

import (
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/entities/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewEntity(t *testing.T) {
	// Test cases
	testCases := []struct {
		name      string
		authToken string
		baseURLs  map[string]string
		options   []Option
		expectErr bool
	}{
		{
			name:      "Valid configuration",
			authToken: "test-token",
			baseURLs: map[string]string{
				"onboarding":  "https://api.example.com/onboarding",
				"transaction": "https://api.example.com/transaction",
			},
			options:   nil,
			expectErr: false,
		},
		{
			name:      "With debug option",
			authToken: "test-token",
			baseURLs: map[string]string{
				"onboarding":  "https://api.example.com/onboarding",
				"transaction": "https://api.example.com/transaction",
			},
			options: []Option{
				WithDebug(true),
			},
			expectErr: false,
		},
		{
			name:      "With invalid option",
			authToken: "test-token",
			baseURLs: map[string]string{
				"onboarding":  "https://api.example.com/onboarding",
				"transaction": "https://api.example.com/transaction",
			},
			options: []Option{
				func(e *Entity) error {
					return assert.AnError
				},
			},
			expectErr: true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create entity
			entity, err := NewEntity(http.DefaultClient, tc.authToken, tc.baseURLs, tc.options...)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, entity)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, entity)

				// Check that all services are initialized
				assert.NotNil(t, entity.Accounts)
				assert.NotNil(t, entity.Assets)
				assert.NotNil(t, entity.AssetRates)
				assert.NotNil(t, entity.Balances)
				assert.NotNil(t, entity.Ledgers)
				assert.NotNil(t, entity.Operations)
				assert.NotNil(t, entity.Organizations)
				assert.NotNil(t, entity.Portfolios)
				assert.NotNil(t, entity.Segments)
				assert.NotNil(t, entity.Transactions)
			}
		})
	}
}

func TestEntityWithMocks(t *testing.T) {
	// Create controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock services
	mockAccounts := mocks.NewMockAccountsService(ctrl)
	mockAssets := mocks.NewMockAssetsService(ctrl)
	mockBalances := mocks.NewMockBalancesService(ctrl)
	mockLedgers := mocks.NewMockLedgersService(ctrl)
	mockOperations := mocks.NewMockOperationsService(ctrl)
	mockOrganizations := mocks.NewMockOrganizationsService(ctrl)
	mockPortfolios := mocks.NewMockPortfoliosService(ctrl)
	mockSegments := mocks.NewMockSegmentsService(ctrl)
	mockTransactions := mocks.NewMockTransactionsService(ctrl)

	// Create entity with base configuration
	entity, err := NewEntity(http.DefaultClient, "test-token", map[string]string{
		"onboarding":  "https://api.example.com/onboarding",
		"transaction": "https://api.example.com/transaction",
	})
	assert.NoError(t, err)
	assert.NotNil(t, entity)

	// Replace services with mocks
	entity.Accounts = mockAccounts
	entity.Assets = mockAssets
	entity.Balances = mockBalances
	entity.Ledgers = mockLedgers
	entity.Operations = mockOperations
	entity.Organizations = mockOrganizations
	entity.Portfolios = mockPortfolios
	entity.Segments = mockSegments
	entity.Transactions = mockTransactions

	// Verify that the services are correctly set
	assert.Equal(t, mockAccounts, entity.Accounts)
	assert.Equal(t, mockAssets, entity.Assets)
	assert.Equal(t, mockBalances, entity.Balances)
	assert.Equal(t, mockLedgers, entity.Ledgers)
	assert.Equal(t, mockOperations, entity.Operations)
	assert.Equal(t, mockOrganizations, entity.Organizations)
	assert.Equal(t, mockPortfolios, entity.Portfolios)
	assert.Equal(t, mockSegments, entity.Segments)
	assert.Equal(t, mockTransactions, entity.Transactions)
}
