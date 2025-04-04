package utils_test

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/LerianStudio/midaz/sdks/go-sdk/pkg/conversion"
	"github.com/LerianStudio/midaz/sdks/go-sdk/pkg/utils"
	"github.com/stretchr/testify/assert"
)

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

func TestParseAmount(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedAmount int64
		expectedScale  int32
		expectedError  bool
	}{
		{
			name:           "Valid integer amount",
			input:          "100",
			expectedAmount: 100,
			expectedScale:  0,
			expectedError:  false,
		},
		{
			name:           "Valid decimal amount",
			input:          "123.45",
			expectedAmount: 12345,
			expectedScale:  2,
			expectedError:  false,
		},
		{
			name:           "Valid negative amount",
			input:          "-50.75",
			expectedAmount: -5075,
			expectedScale:  2,
			expectedError:  false,
		},
		{
			name:           "Valid amount with trailing zeros",
			input:          "10.50",
			expectedAmount: 1050,
			expectedScale:  2,
			expectedError:  false,
		},
		{
			name:           "Valid amount with many decimal places",
			input:          "0.0012345",
			expectedAmount: 12345,
			expectedScale:  7,
			expectedError:  false,
		},
		{
			name:           "Empty string",
			input:          "",
			expectedAmount: 0,
			expectedScale:  0,
			expectedError:  true,
		},
		{
			name:           "Invalid format",
			input:          "abc",
			expectedAmount: 0,
			expectedScale:  0,
			expectedError:  true,
		},
		{
			name:           "Multiple decimal points",
			input:          "10.20.30",
			expectedAmount: 0,
			expectedScale:  0,
			expectedError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			amount, scale, err := utils.ParseAmount(tc.input)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedAmount, amount)
				assert.Equal(t, tc.expectedScale, scale)
			}
		})
	}
}

func TestFormatAmount(t *testing.T) {
	testCases := []struct {
		name           string
		amount         int64
		scale          int
		expectedResult string
	}{
		{
			name:           "Zero amount",
			amount:         0,
			scale:          2,
			expectedResult: "0.00",
		},
		{
			name:           "Positive integer amount",
			amount:         100,
			scale:          0,
			expectedResult: "100",
		},
		{
			name:           "Negative integer amount",
			amount:         -100,
			scale:          0,
			expectedResult: "-100",
		},
		{
			name:           "Positive decimal amount",
			amount:         12345,
			scale:          2,
			expectedResult: "123.45",
		},
		{
			name:           "Negative decimal amount",
			amount:         -12345,
			scale:          2,
			expectedResult: "-123.45",
		},
		{
			name:           "Small decimal amount",
			amount:         5,
			scale:          2,
			expectedResult: "0.05",
		},
		{
			name:           "Small negative decimal amount",
			amount:         -5,
			scale:          2,
			expectedResult: "-0.05",
		},
		{
			name:           "Zero scale",
			amount:         12345,
			scale:          0,
			expectedResult: "12345",
		},
		{
			name:           "High scale",
			amount:         12345,
			scale:          5,
			expectedResult: "0.12345",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FormatAmount(tc.amount, tc.scale)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestConvertToISODateTime(t *testing.T) {
	// Create a fixed time for testing
	testTime := time.Date(2023, 4, 15, 10, 30, 0, 0, time.UTC)
	expected := "2023-04-15T10:30:00Z"

	result := utils.ConvertToISODateTime(testTime)
	assert.Equal(t, expected, result)
}

func TestConvertTransactionToSummary(t *testing.T) {
	// Helper function to create string pointers
	strPtr := func(s string) *string {
		return &s
	}

	testCases := []struct {
		name     string
		tx       *models.Transaction
		expected string
	}{
		{
			name:     "Nil transaction",
			tx:       nil,
			expected: "Invalid transaction: nil",
		},
		{
			name: "Simple transfer between internal accounts",
			tx: &models.Transaction{
				ID:        "tx_123",
				Amount:    10000,
				Scale:     2,
				AssetCode: "USD",
				Status:    models.Status{Code: "completed"},
				Operations: []models.Operation{
					{
						Type:         "DEBIT",
						AccountID:    "acc_source",
						AccountAlias: strPtr("savings"),
					},
					{
						Type:         "CREDIT",
						AccountID:    "acc_dest",
						AccountAlias: strPtr("checking"),
					},
				},
			},
			expected: "Transfer: 100.00 USD from savings to checking (Completed)",
		},
		{
			name: "Deposit from external account",
			tx: &models.Transaction{
				ID:        "tx_456",
				Amount:    5000,
				Scale:     2,
				AssetCode: "USD",
				Status:    models.Status{Code: "pending"},
				Operations: []models.Operation{
					{
						Type:         "DEBIT",
						AccountID:    "external_source",
						AccountAlias: strPtr("@external/USD"),
					},
					{
						Type:         "CREDIT",
						AccountID:    "acc_dest",
						AccountAlias: strPtr("checking"),
					},
				},
			},
			expected: "Deposit: 50.00 USD to checking (Pending)",
		},
		{
			name: "Withdrawal to external account",
			tx: &models.Transaction{
				ID:        "tx_789",
				Amount:    7500,
				Scale:     2,
				AssetCode: "USD",
				Status:    models.Status{Code: "failed"},
				Operations: []models.Operation{
					{
						Type:         "DEBIT",
						AccountID:    "acc_source",
						AccountAlias: strPtr("savings"),
					},
					{
						Type:         "CREDIT",
						AccountID:    "external_dest",
						AccountAlias: strPtr("@external/USD"),
					},
				},
			},
			expected: "Withdrawal: 75.00 USD from savings (Failed)",
		},
		{
			name: "Transaction with account IDs only",
			tx: &models.Transaction{
				ID:        "tx_101",
				Amount:    2000,
				Scale:     2,
				AssetCode: "EUR",
				Status:    models.Status{Code: "completed"},
				Operations: []models.Operation{
					{
						Type:      "DEBIT",
						AccountID: "acc_source",
					},
					{
						Type:      "CREDIT",
						AccountID: "acc_dest",
					},
				},
			},
			expected: "Transfer: 20.00 EUR from acc_source to acc_dest (Completed)",
		},
		{
			name: "Transaction with multiple source accounts",
			tx: &models.Transaction{
				ID:        "tx_102",
				Amount:    3000,
				Scale:     2,
				AssetCode: "USD",
				Status:    models.Status{Code: "completed"},
				Operations: []models.Operation{
					{
						Type:         "DEBIT",
						AccountID:    "acc_source1",
						AccountAlias: strPtr("savings"),
					},
					{
						Type:         "DEBIT",
						AccountID:    "acc_source2",
						AccountAlias: strPtr("checking"),
					},
					{
						Type:         "CREDIT",
						AccountID:    "acc_dest",
						AccountAlias: strPtr("investment"),
					},
				},
			},
			expected: "Transfer: 30.00 USD from multiple accounts (2) to investment (Completed)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := conversion.ConvertTransactionToSummary(tc.tx)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestConvertMetadataToTags(t *testing.T) {
	testCases := []struct {
		name     string
		metadata map[string]any
		expected []string
	}{
		{
			name:     "Nil metadata",
			metadata: nil,
			expected: nil,
		},
		{
			name:     "Empty metadata",
			metadata: map[string]any{},
			expected: nil,
		},
		{
			name: "Metadata without tags",
			metadata: map[string]any{
				"reference":  "INV-123",
				"customerId": "CUST-456",
			},
			expected: nil,
		},
		{
			name: "Metadata with empty tags",
			metadata: map[string]any{
				"tags": "",
			},
			expected: []string{},
		},
		{
			name: "Metadata with single tag",
			metadata: map[string]any{
				"tags": "payment",
			},
			expected: []string{"payment"},
		},
		{
			name: "Metadata with multiple tags",
			metadata: map[string]any{
				"tags": "payment,recurring,subscription",
			},
			expected: []string{"payment", "recurring", "subscription"},
		},
		{
			name: "Metadata with tags and other fields",
			metadata: map[string]any{
				"reference": "INV-123",
				"tags":      "payment,recurring",
				"amount":    100.50,
			},
			expected: []string{"payment", "recurring"},
		},
		{
			name: "Metadata with tags containing whitespace",
			metadata: map[string]any{
				"tags": " payment , recurring , subscription ",
			},
			expected: []string{"payment", "recurring", "subscription"},
		},
		{
			name: "Metadata with tags containing empty elements",
			metadata: map[string]any{
				"tags": "payment,,subscription",
			},
			expected: []string{"payment", "subscription"},
		},
		{
			name: "Metadata with non-string tags field",
			metadata: map[string]any{
				"tags": 123,
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := conversion.ConvertMetadataToTags(tc.metadata)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestConvertTagsToMetadata(t *testing.T) {
	testCases := []struct {
		name     string
		metadata map[string]any
		tags     []string
		expected map[string]any
	}{
		{
			name:     "Empty tags with nil metadata",
			metadata: nil,
			tags:     []string{},
			expected: nil,
		},
		{
			name: "Empty tags with existing metadata",
			metadata: map[string]any{
				"reference": "INV-123",
			},
			tags: []string{},
			expected: map[string]any{
				"reference": "INV-123",
			},
		},
		{
			name:     "Single tag with nil metadata",
			metadata: nil,
			tags:     []string{"payment"},
			expected: map[string]any{"tags": "payment"},
		},
		{
			name: "Single tag with existing metadata",
			metadata: map[string]any{
				"reference": "INV-123",
			},
			tags: []string{"payment"},
			expected: map[string]any{
				"reference": "INV-123",
				"tags":      "payment",
			},
		},
		{
			name:     "Multiple tags with nil metadata",
			metadata: nil,
			tags:     []string{"payment", "recurring", "subscription"},
			expected: map[string]any{"tags": "payment,recurring,subscription"},
		},
		{
			name: "Multiple tags with existing metadata",
			metadata: map[string]any{
				"reference": "INV-123",
				"amount":    100.50,
			},
			tags: []string{"payment", "recurring", "subscription"},
			expected: map[string]any{
				"reference": "INV-123",
				"amount":    100.50,
				"tags":      "payment,recurring,subscription",
			},
		},
		{
			name: "Tags overriding existing tags field",
			metadata: map[string]any{
				"reference": "INV-123",
				"tags":      "old-tag",
			},
			tags: []string{"payment", "recurring"},
			expected: map[string]any{
				"reference": "INV-123",
				"tags":      "payment,recurring",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := conversion.ConvertTagsToMetadata(tc.metadata, tc.tags)
			assert.Equal(t, tc.expected, result)
		})
	}
}
