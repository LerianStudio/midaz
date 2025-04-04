package utils_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/LerianStudio/midaz/sdks/go-sdk/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestValidateTransactionDSL(t *testing.T) {
	testCases := []struct {
		name          string
		input         *models.TransactionDSLInput
		expectedError bool
		errorContains string
	}{
		{
			name:          "Nil input",
			input:         nil,
			expectedError: true,
			errorContains: "transaction input cannot be nil",
		},
		{
			name:          "Nil send object",
			input:         &models.TransactionDSLInput{},
			expectedError: true,
			errorContains: "send object is required",
		},
		{
			name: "Empty asset code",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{},
			},
			expectedError: true,
			errorContains: "asset code is required",
		},
		{
			name: "Invalid asset code",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "us",
				},
			},
			expectedError: true,
			errorContains: "invalid asset code format",
		},
		{
			name: "Zero amount",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 0,
				},
			},
			expectedError: true,
			errorContains: "amount must be greater than zero",
		},
		{
			name: "No source accounts",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{
							{
								Account: "checking",
							},
						},
					},
				},
			},
			expectedError: true,
			errorContains: "at least one source account is required",
		},
		{
			name: "No destination accounts",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{
							{
								Account: "savings",
							},
						},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{},
					},
				},
			},
			expectedError: true,
			errorContains: "at least one destination account is required",
		},
		{
			name: "Asset mismatch in source account",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{
							{
								Account: "savings",
								Amount: &models.DSLAmount{
									Asset: "EUR",
								},
							},
						},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{
							{
								Account: "checking",
							},
						},
					},
				},
			},
			expectedError: true,
			errorContains: "asset mismatch",
		},
		{
			name: "Asset mismatch in destination account",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{
							{
								Account: "savings",
							},
						},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{
							{
								Account: "checking",
								Amount: &models.DSLAmount{
									Asset: "EUR",
								},
							},
						},
					},
				},
			},
			expectedError: true,
			errorContains: "asset mismatch",
		},
		{
			name: "Invalid external account format",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{
							{
								Account: "@external/invalid/format",
							},
						},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{
							{
								Account: "checking",
							},
						},
					},
				},
			},
			expectedError: true,
			errorContains: "invalid external account format",
		},
		{
			name: "External account asset mismatch",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{
							{
								Account: "@external/EUR",
							},
						},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{
							{
								Account: "checking",
							},
						},
					},
				},
			},
			expectedError: true,
			errorContains: "external account asset (EUR) must match transaction asset (USD)",
		},
		{
			name: "Invalid account alias format",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{
							{
								Account: "savings",
							},
						},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{
							{
								Account: "invalid account name with spaces",
							},
						},
					},
				},
			},
			expectedError: true,
			errorContains: "invalid account alias format",
		},
		{
			name: "Valid transaction DSL",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{
							{
								Account: "savings",
							},
						},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{
							{
								Account: "checking",
							},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Valid transaction with external accounts",
			input: &models.TransactionDSLInput{
				Send: &models.DSLSend{
					Asset: "USD",
					Value: 100,
					Source: &models.DSLSource{
						From: []models.DSLFromTo{
							{
								Account: "@external/USD",
							},
						},
					},
					Distribute: &models.DSLDistribute{
						To: []models.DSLFromTo{
							{
								Account: "checking",
							},
						},
					},
				},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := utils.ValidateTransactionDSL(tc.input)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAssetCode(t *testing.T) {
	testCases := []struct {
		name          string
		assetCode     string
		expectedError bool
		errorContains string
	}{
		{
			name:          "Empty asset code",
			assetCode:     "",
			expectedError: true,
			errorContains: "asset code cannot be empty",
		},
		{
			name:          "Too short asset code",
			assetCode:     "US",
			expectedError: true,
			errorContains: "invalid asset code format",
		},
		{
			name:          "Too long asset code",
			assetCode:     "USDOL",
			expectedError: true,
			errorContains: "invalid asset code format",
		},
		{
			name:          "Lowercase asset code",
			assetCode:     "usd",
			expectedError: true,
			errorContains: "invalid asset code format",
		},
		{
			name:          "Asset code with numbers",
			assetCode:     "US1",
			expectedError: true,
			errorContains: "invalid asset code format",
		},
		{
			name:          "Valid 3-letter asset code",
			assetCode:     "USD",
			expectedError: false,
		},
		{
			name:          "Valid 4-letter asset code",
			assetCode:     "USDT",
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := utils.ValidateAssetCode(tc.assetCode)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAccountAlias(t *testing.T) {
	testCases := []struct {
		name          string
		alias         string
		expectedError bool
		errorContains string
	}{
		{
			name:          "Empty alias",
			alias:         "",
			expectedError: true,
			errorContains: "account alias cannot be empty",
		},
		{
			name:          "Alias with spaces",
			alias:         "savings account",
			expectedError: true,
			errorContains: "invalid account alias format",
		},
		{
			name:          "Alias with special characters",
			alias:         "savings@account",
			expectedError: true,
			errorContains: "invalid account alias format",
		},
		{
			name:          "Valid alias with letters only",
			alias:         "savings",
			expectedError: false,
		},
		{
			name:          "Valid alias with numbers",
			alias:         "savings123",
			expectedError: false,
		},
		{
			name:          "Valid alias with underscore",
			alias:         "savings_account",
			expectedError: false,
		},
		{
			name:          "Valid alias with hyphen",
			alias:         "savings-account",
			expectedError: false,
		},
		{
			name:          "Valid alias with mixed case",
			alias:         "SavingsAccount",
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := utils.ValidateAccountAlias(tc.alias)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateTransactionCode(t *testing.T) {
	testCases := []struct {
		name          string
		code          string
		expectedError bool
		errorContains string
	}{
		{
			name:          "Empty code",
			code:          "",
			expectedError: false, // Empty is allowed
		},
		{
			name:          "Code with spaces",
			code:          "TX 123456",
			expectedError: true,
			errorContains: "invalid transaction code format",
		},
		{
			name:          "Code with special characters",
			code:          "TX@123456",
			expectedError: true,
			errorContains: "invalid transaction code format",
		},
		{
			name:          "Valid code with letters only",
			code:          "TXABCDEF",
			expectedError: false,
		},
		{
			name:          "Valid code with numbers",
			code:          "TX123456",
			expectedError: false,
		},
		{
			name:          "Valid code with underscore",
			code:          "TX_123456",
			expectedError: false,
		},
		{
			name:          "Valid code with hyphen",
			code:          "TX-123456",
			expectedError: false,
		},
		{
			name:          "Valid code with mixed case",
			code:          "TxAbCdEf",
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := utils.ValidateTransactionCode(tc.code)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMetadata(t *testing.T) {
	testCases := []struct {
		name          string
		metadata      map[string]any
		expectedError bool
		errorContains string
	}{
		{
			name:          "Nil metadata",
			metadata:      nil,
			expectedError: false,
		},
		{
			name:          "Empty metadata",
			metadata:      map[string]any{},
			expectedError: false,
		},
		{
			name: "Empty key",
			metadata: map[string]any{
				"": "value",
			},
			expectedError: true,
			errorContains: "metadata keys cannot be empty",
		},
		{
			name: "Key with spaces",
			metadata: map[string]any{
				"key with spaces": "value",
			},
			expectedError: true,
			errorContains: "invalid metadata key format",
		},
		{
			name: "Key with special characters",
			metadata: map[string]any{
				"key@with#chars": "value",
			},
			expectedError: true,
			errorContains: "invalid metadata key format",
		},
		{
			name: "Valid string value",
			metadata: map[string]any{
				"reference": "INV-123",
			},
			expectedError: false,
		},
		{
			name: "Valid numeric values",
			metadata: map[string]any{
				"amount_int":     123,
				"amount_int32":   int32(123),
				"amount_int64":   int64(123),
				"amount_float32": float32(123.45),
				"amount_float64": 123.45,
			},
			expectedError: false,
		},
		{
			name: "Valid boolean value",
			metadata: map[string]any{
				"is_recurring": true,
			},
			expectedError: false,
		},
		{
			name: "Valid time value",
			metadata: map[string]any{
				"created_at": time.Now(),
			},
			expectedError: false,
		},
		{
			name: "Valid nil value",
			metadata: map[string]any{
				"optional_field": nil,
			},
			expectedError: false,
		},
		{
			name: "Valid array of simple types",
			metadata: map[string]any{
				"tags": []any{"payment", "recurring", 123, true},
			},
			expectedError: false,
		},
		{
			name: "Invalid array with complex type",
			metadata: map[string]any{
				"tags": []any{"payment", map[string]string{"invalid": "value"}},
			},
			expectedError: true,
			errorContains: "invalid metadata value type in array",
		},
		{
			name: "Valid nested map",
			metadata: map[string]any{
				"customer": map[string]any{
					"id":   "CUST-123",
					"name": "John Doe",
					"age":  30,
				},
			},
			expectedError: false,
		},
		{
			name: "Invalid nested map with complex type",
			metadata: map[string]any{
				"customer": map[string]any{
					"id":      "CUST-123",
					"address": []map[string]string{{"city": "New York"}},
				},
			},
			expectedError: true,
			errorContains: "invalid metadata value type in nested map",
		},
		{
			name: "Unsupported type",
			metadata: map[string]any{
				"complex": struct{ Name string }{"test"},
			},
			expectedError: true,
			errorContains: "invalid metadata value type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := utils.ValidateMetadata(tc.metadata)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateDateRange(t *testing.T) {
	testCases := []struct {
		name          string
		start         time.Time
		end           time.Time
		expectedError bool
		errorContains string
	}{
		{
			name:          "Valid date range",
			start:         time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			end:           time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
			expectedError: false,
		},
		{
			name:          "Same start and end date",
			start:         time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			end:           time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedError: false,
		},
		{
			name:          "Start date after end date",
			start:         time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
			end:           time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedError: true,
			errorContains: "invalid date range: start date",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := utils.ValidateDateRange(tc.start, tc.end)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationSummary(t *testing.T) {
	t.Run("Initially valid", func(t *testing.T) {
		summary := utils.ValidationSummary{
			Valid:  true,
			Errors: []error{},
		}

		assert.True(t, summary.Valid)
		assert.Empty(t, summary.Errors)
		assert.Nil(t, summary.GetErrorMessages())
		assert.Empty(t, summary.GetErrorSummary())
	})

	t.Run("Add single error", func(t *testing.T) {
		summary := utils.ValidationSummary{
			Valid:  true,
			Errors: []error{},
		}

		summary.AddError(fmt.Errorf("test error"))

		assert.False(t, summary.Valid)
		assert.Len(t, summary.Errors, 1)
		assert.Equal(t, "test error", summary.Errors[0].Error())

		messages := summary.GetErrorMessages()
		assert.Len(t, messages, 1)
		assert.Equal(t, "test error", messages[0])

		errorSummary := summary.GetErrorSummary()
		assert.Equal(t, "test error", errorSummary)
	})

	t.Run("Add multiple errors", func(t *testing.T) {
		summary := utils.ValidationSummary{
			Valid:  true,
			Errors: []error{},
		}

		summary.AddError(fmt.Errorf("error 1"))
		summary.AddError(fmt.Errorf("error 2"))
		summary.AddError(fmt.Errorf("error 3"))

		assert.False(t, summary.Valid)
		assert.Len(t, summary.Errors, 3)

		messages := summary.GetErrorMessages()
		assert.Len(t, messages, 3)
		assert.Equal(t, "error 1", messages[0])
		assert.Equal(t, "error 2", messages[1])
		assert.Equal(t, "error 3", messages[2])

		errorSummary := summary.GetErrorSummary()
		assert.Contains(t, errorSummary, "3 validation errors:")
		assert.Contains(t, errorSummary, "- Error 1: error 1")
		assert.Contains(t, errorSummary, "- Error 2: error 2")
		assert.Contains(t, errorSummary, "- Error 3: error 3")
	})
}

func TestGetExternalAccountReference(t *testing.T) {
	testCases := []struct {
		name           string
		assetCode      string
		expectedResult string
	}{
		{
			name:           "USD asset code",
			assetCode:      "USD",
			expectedResult: "@external/USD",
		},
		{
			name:           "EUR asset code",
			assetCode:      "EUR",
			expectedResult: "@external/EUR",
		},
		{
			name:           "USDT asset code",
			assetCode:      "USDT",
			expectedResult: "@external/USDT",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.GetExternalAccountReference(tc.assetCode)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestValidateCreateTransactionInput(t *testing.T) {
	testCases := []struct {
		name          string
		input         *models.CreateTransactionInput
		expectedValid bool
		errorContains []string
	}{
		{
			name:          "Nil input",
			input:         nil,
			expectedValid: false,
			errorContains: []string{"transaction input cannot be nil"},
		},
		{
			name: "Empty asset code",
			input: &models.CreateTransactionInput{
				Amount:    1000,
				Scale:     2,
				AssetCode: "",
			},
			expectedValid: false,
			errorContains: []string{"asset code cannot be empty"},
		},
		{
			name: "Invalid asset code",
			input: &models.CreateTransactionInput{
				Amount:    1000,
				Scale:     2,
				AssetCode: "us",
			},
			expectedValid: false,
			errorContains: []string{"invalid asset code format"},
		},
		{
			name: "Zero amount",
			input: &models.CreateTransactionInput{
				Amount:    0,
				Scale:     2,
				AssetCode: "USD",
			},
			expectedValid: false,
			errorContains: []string{"amount must be greater than zero"},
		},
		{
			name: "Negative amount",
			input: &models.CreateTransactionInput{
				Amount:    -1000,
				Scale:     2,
				AssetCode: "USD",
			},
			expectedValid: false,
			errorContains: []string{"amount must be greater than zero"},
		},
		{
			name: "No operations",
			input: &models.CreateTransactionInput{
				Amount:     1000,
				Scale:      2,
				AssetCode:  "USD",
				Operations: []models.CreateOperationInput{},
			},
			expectedValid: false,
			errorContains: []string{"at least one operation is required"},
		},
		{
			name: "Operation missing account identifiers",
			input: &models.CreateTransactionInput{
				Amount:    1000,
				Scale:     2,
				AssetCode: "USD",
				Operations: []models.CreateOperationInput{
					{
						Type: "DEBIT",
					},
				},
			},
			expectedValid: false,
			errorContains: []string{"operation at index 0 must have either accountId or accountAlias"},
		},
		{
			name: "Operation missing type",
			input: &models.CreateTransactionInput{
				Amount:    1000,
				Scale:     2,
				AssetCode: "USD",
				Operations: []models.CreateOperationInput{
					{
						AccountID: "acc_123",
					},
				},
			},
			expectedValid: false,
			errorContains: []string{"operation at index 0 must have a type"},
		},
		{
			name: "Operation with invalid type",
			input: &models.CreateTransactionInput{
				Amount:    1000,
				Scale:     2,
				AssetCode: "USD",
				Operations: []models.CreateOperationInput{
					{
						Type:      "INVALID",
						AccountID: "acc_123",
					},
				},
			},
			expectedValid: false,
			errorContains: []string{"operation at index 0 has invalid type: INVALID"},
		},
		{
			name: "Operation with asset code mismatch",
			input: &models.CreateTransactionInput{
				Amount:    1000,
				Scale:     2,
				AssetCode: "USD",
				Operations: []models.CreateOperationInput{
					{
						Type:      "DEBIT",
						AccountID: "acc_123",
						AssetCode: "EUR",
					},
				},
			},
			expectedValid: false,
			errorContains: []string{"operation at index 0 has asset code EUR that doesn't match transaction asset code USD"},
		},
		{
			name: "Invalid chart of accounts group name",
			input: &models.CreateTransactionInput{
				Amount:                   1000,
				Scale:                    2,
				AssetCode:                "USD",
				ChartOfAccountsGroupName: "invalid group name with spaces",
				Operations: []models.CreateOperationInput{
					{
						Type:      "DEBIT",
						AccountID: "acc_123",
					},
				},
			},
			expectedValid: false,
			errorContains: []string{"invalid chart of accounts group name format"},
		},
		{
			name: "Invalid metadata",
			input: &models.CreateTransactionInput{
				Amount:    1000,
				Scale:     2,
				AssetCode: "USD",
				Operations: []models.CreateOperationInput{
					{
						Type:      "DEBIT",
						AccountID: "acc_123",
					},
				},
				Metadata: map[string]any{
					"key with spaces": "value",
				},
			},
			expectedValid: false,
			errorContains: []string{"invalid metadata key format"},
		},
		{
			name: "Valid transaction input",
			input: &models.CreateTransactionInput{
				Amount:    1000,
				Scale:     2,
				AssetCode: "USD",
				Operations: []models.CreateOperationInput{
					{
						Type:      "DEBIT",
						AccountID: "acc_source",
						Amount:    1000,
					},
					{
						Type:         "CREDIT",
						AccountAlias: strPtr("checking"),
						Amount:       1000,
					},
				},
				Metadata: map[string]any{
					"reference": "TX-123456",
					"tags":      "payment,recurring",
				},
			},
			expectedValid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			summary := utils.ValidateCreateTransactionInput(tc.input)

			assert.Equal(t, tc.expectedValid, summary.Valid)

			if !tc.expectedValid {
				errorMessages := summary.GetErrorMessages()
				for _, expectedError := range tc.errorContains {
					found := false
					for _, msg := range errorMessages {
						if assert.Contains(t, msg, expectedError) {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected error containing '%s' not found", expectedError)
				}
			}
		})
	}
}

func TestValidateAccountType(t *testing.T) {
	tests := []struct {
		name        string
		accountType string
		wantErr     bool
	}{
		{
			name:        "Valid deposit account type",
			accountType: "deposit",
			wantErr:     false,
		},
		{
			name:        "Valid external account type",
			accountType: "external",
			wantErr:     false,
		},
		{
			name:        "Valid liability account type",
			accountType: "liability",
			wantErr:     false,
		},
		{
			name:        "Valid uppercase account type",
			accountType: "DEPOSIT",
			wantErr:     false,
		},
		{
			name:        "Invalid account type",
			accountType: "invalid",
			wantErr:     true,
		},
		{
			name:        "Empty account type",
			accountType: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidateAccountType(tt.accountType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAccountType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAssetType(t *testing.T) {
	tests := []struct {
		name      string
		assetType string
		wantErr   bool
	}{
		{
			name:      "Valid currency asset type",
			assetType: "currency",
			wantErr:   false,
		},
		{
			name:      "Valid crypto asset type",
			assetType: "crypto",
			wantErr:   false,
		},
		{
			name:      "Valid commodities asset type",
			assetType: "commodities",
			wantErr:   false,
		},
		{
			name:      "Valid others asset type",
			assetType: "others",
			wantErr:   false,
		},
		{
			name:      "Valid uppercase asset type",
			assetType: "CURRENCY",
			wantErr:   false,
		},
		{
			name:      "Invalid asset type",
			assetType: "invalid",
			wantErr:   true,
		},
		{
			name:      "Empty asset type",
			assetType: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidateAssetType(tt.assetType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAssetType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCurrencyCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "Valid USD currency code",
			code:    "USD",
			wantErr: false,
		},
		{
			name:    "Valid EUR currency code",
			code:    "EUR",
			wantErr: false,
		},
		{
			name:    "Invalid currency code",
			code:    "XYZ",
			wantErr: true,
		},
		{
			name:    "Empty currency code",
			code:    "",
			wantErr: true,
		},
		{
			name:    "Lowercase currency code",
			code:    "usd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidateCurrencyCode(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCurrencyCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCountryCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "Valid US country code",
			code:    "US",
			wantErr: false,
		},
		{
			name:    "Valid GB country code",
			code:    "GB",
			wantErr: false,
		},
		{
			name:    "Invalid country code",
			code:    "XX",
			wantErr: true,
		},
		{
			name:    "Empty country code",
			code:    "",
			wantErr: true,
		},
		{
			name:    "Lowercase country code",
			code:    "us",
			wantErr: true,
		},
		{
			name:    "Too long country code",
			code:    "USA",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidateCountryCode(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCountryCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		address *utils.Address
		wantErr bool
	}{
		{
			name: "Valid address",
			address: &utils.Address{
				Line1:   "123 Main St",
				ZipCode: "12345",
				City:    "New York",
				State:   "NY",
				Country: "US",
			},
			wantErr: false,
		},
		{
			name:    "Nil address",
			address: nil,
			wantErr: true,
		},
		{
			name: "Missing line1",
			address: &utils.Address{
				ZipCode: "12345",
				City:    "New York",
				State:   "NY",
				Country: "US",
			},
			wantErr: true,
		},
		{
			name: "Missing zipCode",
			address: &utils.Address{
				Line1:   "123 Main St",
				City:    "New York",
				State:   "NY",
				Country: "US",
			},
			wantErr: true,
		},
		{
			name: "Missing city",
			address: &utils.Address{
				Line1:   "123 Main St",
				ZipCode: "12345",
				State:   "NY",
				Country: "US",
			},
			wantErr: true,
		},
		{
			name: "Missing state",
			address: &utils.Address{
				Line1:   "123 Main St",
				ZipCode: "12345",
				City:    "New York",
				Country: "US",
			},
			wantErr: true,
		},
		{
			name: "Missing country",
			address: &utils.Address{
				Line1:   "123 Main St",
				ZipCode: "12345",
				City:    "New York",
				State:   "NY",
			},
			wantErr: true,
		},
		{
			name: "Invalid country code",
			address: &utils.Address{
				Line1:   "123 Main St",
				ZipCode: "12345",
				City:    "New York",
				State:   "NY",
				Country: "XX", // Invalid country code
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidateAddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
