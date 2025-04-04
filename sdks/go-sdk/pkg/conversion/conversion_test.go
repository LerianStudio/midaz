package conversion_test

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/LerianStudio/midaz/sdks/go-sdk/pkg/conversion"
	"github.com/stretchr/testify/assert"
)

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

func TestFormatAmount(t *testing.T) {
	testCases := []struct {
		name     string
		amount   int64
		scale    int
		expected string
	}{
		{
			name:     "Positive integer",
			amount:   100,
			scale:    0,
			expected: "100",
		},
		{
			name:     "Positive decimal",
			amount:   12345,
			scale:    2,
			expected: "123.45",
		},
		{
			name:     "Negative decimal",
			amount:   -5075,
			scale:    2,
			expected: "-50.75",
		},
		{
			name:     "Zero amount",
			amount:   0,
			scale:    2,
			expected: "0.00",
		},
		{
			name:     "Small decimal",
			amount:   5,
			scale:    2,
			expected: "0.05",
		},
		{
			name:     "Very small decimal",
			amount:   1,
			scale:    5,
			expected: "0.00001",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := conversion.FormatAmount(tc.amount, tc.scale)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestConvertToISODate(t *testing.T) {
	// Create a fixed time for testing
	testTime := time.Date(2025, 4, 2, 15, 4, 5, 0, time.UTC)
	expected := "2025-04-02"

	result := conversion.ConvertToISODate(testTime)
	assert.Equal(t, expected, result)
}

func TestConvertToISODateTime(t *testing.T) {
	// Create a fixed time for testing
	testTime := time.Date(2025, 4, 2, 15, 4, 5, 0, time.UTC)
	expected := "2025-04-02T15:04:05Z"

	result := conversion.ConvertToISODateTime(testTime)
	assert.Equal(t, expected, result)
}

func TestConvertTransactionToSummary(t *testing.T) {
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
				Status:    models.Status{Code: "COMPLETED"},
				Operations: []models.Operation{
					{
						Type:         "DEBIT",
						AccountID:    "acc_source",
						AccountAlias: stringPtr("savings"),
					},
					{
						Type:         "CREDIT",
						AccountID:    "acc_dest",
						AccountAlias: stringPtr("checking"),
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
				Status:    models.Status{Code: "PENDING"},
				Operations: []models.Operation{
					{
						Type:         "DEBIT",
						AccountID:    "acc_external",
						AccountAlias: stringPtr("@external/USD"),
					},
					{
						Type:         "CREDIT",
						AccountID:    "acc_dest",
						AccountAlias: stringPtr("checking"),
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
				Status:    models.Status{Code: "FAILED"},
				Operations: []models.Operation{
					{
						Type:         "DEBIT",
						AccountID:    "acc_source",
						AccountAlias: stringPtr("savings"),
					},
					{
						Type:         "CREDIT",
						AccountID:    "acc_external",
						AccountAlias: stringPtr("@external/USD"),
					},
				},
			},
			expected: "Withdrawal: 75.00 USD from savings (Failed)",
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
			name: "Metadata with tags containing whitespace",
			metadata: map[string]any{
				"tags": " payment , recurring , subscription ",
			},
			expected: []string{"payment", "recurring", "subscription"},
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
			name: "Tags with whitespace",
			metadata: map[string]any{
				"reference": "INV-123",
			},
			tags: []string{" payment ", " recurring ", " subscription "},
			expected: map[string]any{
				"reference": "INV-123",
				"tags":      "payment,recurring,subscription",
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
