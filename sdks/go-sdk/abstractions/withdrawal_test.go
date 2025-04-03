package abstractions

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateWithdrawal(t *testing.T) {
	// Test cases
	tests := []struct {
		name               string
		sourceAccountAlias string
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
			name:               "Valid withdrawal",
			sourceAccountAlias: "customer_john_doe",
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test withdrawal",
			options:            []Option{WithPending(true)},
			validateInput: func(t *testing.T, input *models.TransactionDSLInput) {
				// Validate description
				assert.Equal(t, "Test withdrawal", input.Description)

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
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test withdrawal",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "source account alias is required",
		},
		{
			name:               "Zero amount",
			sourceAccountAlias: "customer_john_doe",
			amount:             0,
			scale:              2,
			assetCode:          "USD",
			description:        "Test withdrawal",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "amount must be greater than zero",
		},
		{
			name:               "Negative amount",
			sourceAccountAlias: "customer_john_doe",
			amount:             -100,
			scale:              2,
			assetCode:          "USD",
			description:        "Test withdrawal",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "amount must be greater than zero",
		},
		{
			name:               "Missing asset code",
			sourceAccountAlias: "customer_john_doe",
			amount:             10000,
			scale:              2,
			assetCode:          "",
			description:        "Test withdrawal",
			options:            []Option{},
			validateInput:      nil,
			mockReturn:         nil,
			mockError:          nil,
			expectedError:      "asset code is required",
		},
		{
			name:               "With metadata option",
			sourceAccountAlias: "customer_john_doe",
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test withdrawal",
			options: []Option{
				WithMetadata(map[string]any{
					"reference": "TEST-123",
				}),
			},
			validateInput: func(t *testing.T, input *models.TransactionDSLInput) {
				assert.Equal(t, "Test withdrawal", input.Description)
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
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test withdrawal",
			options:            []Option{WithPending(true)},
			validateInput: func(t *testing.T, input *models.TransactionDSLInput) {
				assert.Equal(t, "Test withdrawal", input.Description)
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
			amount:             10000,
			scale:              2,
			assetCode:          "USD",
			description:        "Test withdrawal",
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
			abs := NewAbstraction(mockAbs.createTransactionWithDSL)

			// Set up the mock expectations if source account is provided and we're not expecting a validation error
			if tt.sourceAccountAlias != "" && tt.expectedError == "" {
				mockAbs.On("createTransactionWithDSL", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(input *models.TransactionDSLInput) bool {
					// Validate the input if a validation function is provided
					if tt.validateInput != nil {
						tt.validateInput(t, input)
					}
					return true
				})).Return(tt.mockReturn, tt.mockError)
			} else if tt.sourceAccountAlias != "" && tt.expectedError == "DSL validation error" {
				mockAbs.On("createTransactionWithDSL", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, tt.mockError)
			}

			// Call the function
			result, err := abs.Withdrawals.CreateWithdrawal(context.Background(), "org123", "ledger123", tt.sourceAccountAlias, tt.amount, int64(tt.scale), tt.assetCode, tt.description, tt.options...)

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
