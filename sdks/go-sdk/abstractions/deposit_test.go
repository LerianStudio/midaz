package abstractions

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreate(t *testing.T) {
	// Test cases
	tests := []struct {
		name               string
		organizationID     string
		ledgerID           string
		targetAccountAlias string
		amount             int64
		scale              int
		assetCode          string
		description        string
		options            []Option
		mockReturn         *models.Transaction
		mockError          error
		expectedError      string
	}{
		{
			name:               "Valid deposit",
			organizationID:     "org123",
			ledgerID:           "ledger123",
			targetAccountAlias: "customer_john_doe",
			amount:             100,
			scale:              2,
			assetCode:          "USD",
			description:        "Test deposit",
			options:            []Option{WithPending(true)},
			mockReturn: &models.Transaction{
				ID:        "tx123",
				Amount:    100,
				Scale:     2,
				AssetCode: "USD",
				Status: models.Status{
					Code: "pending",
				},
			},
			mockError:     nil,
			expectedError: "",
		},
		{
			name:               "Missing target account",
			organizationID:     "org123",
			ledgerID:           "ledger123",
			targetAccountAlias: "",
			amount:             100,
			scale:              2,
			assetCode:          "USD",
			description:        "Test deposit",
			options:            []Option{},
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "target account alias is required",
		},
		{
			name:               "Zero amount",
			organizationID:     "org123",
			ledgerID:           "ledger123",
			targetAccountAlias: "customer_john_doe",
			amount:             0,
			scale:              2,
			assetCode:          "USD",
			description:        "Test deposit",
			options:            []Option{},
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "amount must be greater than zero",
		},
		{
			name:               "Negative amount",
			organizationID:     "org123",
			ledgerID:           "ledger123",
			targetAccountAlias: "customer_john_doe",
			amount:             -100,
			scale:              2,
			assetCode:          "USD",
			description:        "Test deposit",
			options:            []Option{},
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "amount must be greater than zero",
		},
		{
			name:               "Missing asset code",
			organizationID:     "org123",
			ledgerID:           "ledger123",
			targetAccountAlias: "customer_john_doe",
			amount:             100,
			scale:              2,
			assetCode:          "",
			description:        "Test deposit",
			options:            []Option{},
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "asset code is required",
		},
		{
			name:               "With metadata option",
			organizationID:     "org123",
			ledgerID:           "ledger123",
			targetAccountAlias: "customer_john_doe",
			amount:             100,
			scale:              2,
			assetCode:          "USD",
			description:        "Test deposit with metadata",
			options:            []Option{WithMetadata(map[string]any{"reference": "DEP12345"})},
			mockReturn: &models.Transaction{
				ID:        "tx123",
				Amount:    100,
				Scale:     2,
				AssetCode: "USD",
				Status: models.Status{
					Code: "completed",
				},
			},
			mockError:     nil,
			expectedError: "",
		},
		{
			name:               "With pending option",
			organizationID:     "org123",
			ledgerID:           "ledger123",
			targetAccountAlias: "customer_john_doe",
			amount:             100,
			scale:              2,
			assetCode:          "USD",
			description:        "Test pending deposit",
			options:            []Option{WithPending(true)},
			mockReturn: &models.Transaction{
				ID:        "tx123",
				Amount:    100,
				Scale:     2,
				AssetCode: "USD",
				Status: models.Status{
					Code: "pending",
				},
			},
			mockError:     nil,
			expectedError: "",
		},
		{
			name:               "DSL validation error",
			organizationID:     "org123",
			ledgerID:           "ledger123",
			targetAccountAlias: "customer_john_doe",
			amount:             100,
			scale:              2,
			assetCode:          "USD",
			description:        "Test deposit",
			options:            []Option{},
			mockReturn:         nil,
			mockError:          errors.New("DSL validation error"),
			expectedError:      "DSL validation error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock abstraction
			mockAbs := new(MockAbstraction)

			// Create the abstraction with the mock function
			abs := NewAbstraction(
				mockAbs.createTransactionWithDSL,
				&MockTransactionsService{
					ReturnTransaction: &models.Transaction{
						ID:     "tx-mock-123",
						Status: models.Status{Code: models.TransactionStatusCompleted},
					},
				},
			)

			// Set up the mock expectations if target account is provided
			if tt.targetAccountAlias != "" {
				// For the validation error test case, we need to ensure the mock returns an error
				if tt.expectedError == "DSL validation error" {
					mockAbs.On("createTransactionWithDSL", mock.Anything, tt.organizationID, tt.ledgerID, mock.Anything).Return(nil, tt.mockError)
				} else if tt.expectedError == "" {
					// For successful test cases, return the mock transaction
					mockAbs.On("createTransactionWithDSL", mock.Anything, tt.organizationID, tt.ledgerID, mock.MatchedBy(func(input *models.TransactionDSLInput) bool {
						// Validate description is set correctly
						assert.Equal(t, tt.description, input.Description)
						return true
					})).Return(tt.mockReturn, nil)
				}
				// For other validation errors (like zero amount), we don't set up the mock as the function should return early
			}

			// Call the function
			result, err := abs.Deposits.Create(context.Background(), tt.organizationID, tt.ledgerID, tt.targetAccountAlias, tt.amount, int64(tt.scale), tt.assetCode, tt.description, tt.options...)

			// Check the error
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.mockReturn.ID, result.ID)
				assert.Equal(t, tt.mockReturn.Amount, result.Amount)
				assert.Equal(t, tt.mockReturn.Scale, result.Scale)
				assert.Equal(t, tt.mockReturn.AssetCode, result.AssetCode)
				assert.Equal(t, tt.mockReturn.Status.Code, result.Status.Code)
			}

			// Verify all expectations were met
			mockAbs.AssertExpectations(t)
		})
	}
}

func TestDepositService_List(t *testing.T) {
	// Create a mock transactions service
	mockTxService := &MockTransactionsService{
		ReturnTransactionList: &models.ListResponse[models.Transaction]{
			Items: []models.Transaction{
				{
					ID:     "tx-deposit-123",
					Status: models.Status{Code: models.TransactionStatusCompleted},
					Operations: []models.Operation{
						{Type: "credit", AccountID: "acc-123"},
					},
				},
			},
			Pagination: models.Pagination{
				Limit:  10,
				Offset: 0,
				Total:  1,
			},
		},
	}

	// Create the abstraction with the mock service
	abs := NewAbstraction(
		func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
			return nil, nil // Not used in this test
		},
		mockTxService,
	)

	// Call the List method
	result, err := abs.Deposits.List(
		context.Background(),
		"org-123",
		"ledger-456",
		&models.ListOptions{},
	)

	// Verify the result
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, "tx-deposit-123", result.Items[0].ID)
}

func TestDepositService_Get(t *testing.T) {
	// Create a deposit transaction with credit operations
	depositTx := &models.Transaction{
		ID:     "tx-deposit-123",
		Status: models.Status{Code: models.TransactionStatusCompleted},
		Operations: []models.Operation{
			{Type: "credit", AccountID: "acc-123"},
		},
	}

	// Create a mock transactions service
	mockTxService := &MockTransactionsService{
		ReturnTransaction: depositTx,
	}

	// Create the abstraction with the mock service
	abs := NewAbstraction(
		func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
			return nil, nil // Not used in this test
		},
		mockTxService,
	)

	// Call the Get method
	result, err := abs.Deposits.Get(
		context.Background(),
		"org-123",
		"ledger-456",
		"tx-deposit-123",
	)

	// Verify the result
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "tx-deposit-123", result.ID)
}

func TestDepositService_Update(t *testing.T) {
	// Create a deposit transaction with credit operations
	depositTx := &models.Transaction{
		ID:     "tx-deposit-123",
		Status: models.Status{Code: models.TransactionStatusCompleted},
		Operations: []models.Operation{
			{Type: "credit", AccountID: "acc-123"},
		},
	}

	// Create a mock transactions service
	mockTxService := &MockTransactionsService{
		ReturnTransaction: depositTx,
	}

	// Create the abstraction with the mock service
	abs := NewAbstraction(
		func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
			return nil, nil // Not used in this test
		},
		mockTxService,
	)

	// Create an update input
	updateInput := &models.UpdateTransactionInput{
		Metadata: map[string]any{
			"updated_by": "test",
			"note":       "Updated for testing",
		},
	}

	// Call the Update method
	result, err := abs.Deposits.Update(
		context.Background(),
		"org-123",
		"ledger-456",
		"tx-deposit-123",
		updateInput,
	)

	// Verify the result
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "tx-deposit-123", result.ID)
}
