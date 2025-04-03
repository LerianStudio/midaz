// Package abstractions provides high-level transaction operations for the Midaz platform.
//
// This package contains functions and options for creating and managing financial transactions
// like deposits, withdrawals, and transfers.
package abstractions

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateTransfer(t *testing.T) {
	// Test cases
	tests := []struct {
		name               string
		sourceAccountAlias string
		targetAccountAlias string
		amount             int64
		scale              int
		assetCode          string
		description        string
		options            []Option
		validateInput      func(t *testing.T, input *models.TransactionDSLInput)
		mockReturn         *models.Transaction
		mockError          error
		expectedError      string
	}{
		{
			name:               "Valid transfer",
			sourceAccountAlias: "customer_john_doe",
			targetAccountAlias: "merchant_acme",
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test transfer",
			options:            []Option{WithPending(true)},
			validateInput: func(t *testing.T, input *models.TransactionDSLInput) {
				// Validate description
				assert.Equal(t, "Test transfer", input.Description)

				// Validate send
				assert.NotNil(t, input.Send)
				assert.Equal(t, "USD", input.Send.Asset)
				assert.Equal(t, int64(10000), input.Send.Value)
				assert.Equal(t, int64(2), input.Send.Scale)

				// Validate source (internal account)
				assert.NotNil(t, input.Send.Source)
				assert.Len(t, input.Send.Source.From, 1)
				assert.Equal(t, "customer_john_doe", input.Send.Source.From[0].Account)
				assert.Equal(t, int64(10000), input.Send.Source.From[0].Amount.Value)
				assert.Equal(t, int64(2), input.Send.Source.From[0].Amount.Scale)
				assert.Equal(t, "USD", input.Send.Source.From[0].Amount.Asset)

				// Validate target (internal account)
				assert.NotNil(t, input.Send.Distribute)
				assert.Len(t, input.Send.Distribute.To, 1)
				assert.Equal(t, "merchant_acme", input.Send.Distribute.To[0].Account)
				assert.Equal(t, int64(10000), input.Send.Distribute.To[0].Amount.Value)
				assert.Equal(t, int64(2), input.Send.Distribute.To[0].Amount.Scale)
				assert.Equal(t, "USD", input.Send.Distribute.To[0].Amount.Asset)

				// Validate pending
				assert.True(t, input.Pending)
			},
			mockReturn: &models.Transaction{
				ID:        "tx123",
				Amount:    10000,
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
			name:               "Missing source account",
			sourceAccountAlias: "",
			targetAccountAlias: "merchant_acme",
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test transfer",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "source account alias is required",
		},
		{
			name:               "Missing target account",
			sourceAccountAlias: "customer_john_doe",
			targetAccountAlias: "",
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test transfer",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "target account alias is required",
		},
		{
			name:               "Zero amount",
			sourceAccountAlias: "customer_john_doe",
			targetAccountAlias: "merchant_acme",
			amount:             0,
			scale:              2,
			assetCode:          "USD",
			description:        "Test transfer",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "amount must be greater than zero",
		},
		{
			name:               "Negative amount",
			sourceAccountAlias: "customer_john_doe",
			targetAccountAlias: "merchant_acme",
			amount:             -100,
			scale:              2,
			assetCode:          "USD",
			description:        "Test transfer",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "amount must be greater than zero",
		},
		{
			name:               "Missing asset code",
			sourceAccountAlias: "customer_john_doe",
			targetAccountAlias: "merchant_acme",
			amount:             10000,
			scale:              2,
			assetCode:          "",
			description:        "Test transfer",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "asset code is required",
		},
		{
			name:               "With metadata option",
			sourceAccountAlias: "customer_john_doe",
			targetAccountAlias: "merchant_acme",
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test transfer with metadata",
			options: []Option{
				WithMetadata(map[string]any{
					"reference": "TEST-123",
				}),
			},
			validateInput: func(t *testing.T, input *models.TransactionDSLInput) {
				assert.Equal(t, "Test transfer with metadata", input.Description)
				assert.Equal(t, "TEST-123", input.Metadata["reference"])
			},
			mockReturn: &models.Transaction{
				ID:        "tx123",
				Amount:    10000,
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
			sourceAccountAlias: "customer_john_doe",
			targetAccountAlias: "merchant_acme",
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test pending transfer",
			options:            []Option{WithPending(true)},
			validateInput: func(t *testing.T, input *models.TransactionDSLInput) {
				assert.Equal(t, "Test pending transfer", input.Description)
				assert.True(t, input.Pending)
			},
			mockReturn: &models.Transaction{
				ID:        "tx123",
				Amount:    10000,
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
			sourceAccountAlias: "customer_john_doe",
			targetAccountAlias: "merchant_acme",
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test transfer",
			options:            []Option{},
			validateInput:      nil,
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

			// Set up the mock expectations if source and target accounts are provided
			if tt.sourceAccountAlias != "" && tt.targetAccountAlias != "" {
				if tt.expectedError == "DSL validation error" {
					// For the validation error test case, we need to ensure the mock returns an error
					mockAbs.On("createTransactionWithDSL", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, tt.mockError)
				} else if tt.expectedError == "" {
					// For successful test cases, validate the input and return the mock transaction
					mockAbs.On("createTransactionWithDSL", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(input *models.TransactionDSLInput) bool {
						// Validate the input if a validation function is provided
						if tt.validateInput != nil {
							tt.validateInput(t, input)
						}
						return true
					})).Return(tt.mockReturn, tt.mockError)
				}
				// For other validation errors (like zero amount), we don't set up the mock as the function should return early
			}

			// Call the function
			result, err := abs.Transfers.Create(
				context.Background(),
				"org123", "ledger123",
				tt.sourceAccountAlias, tt.targetAccountAlias,
				tt.amount, int64(tt.scale), tt.assetCode,
				tt.description, tt.options...,
			)

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
