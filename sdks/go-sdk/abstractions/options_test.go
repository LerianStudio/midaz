// Package abstractions provides high-level transaction operations for the Midaz platform.
//
// This package contains functions and options for creating and managing financial transactions
// like deposits, withdrawals, and transfers.
package abstractions

import (
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/assert"
)

func TestWithMetadata(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Apply the option
	metadata := map[string]any{
		"reference": "TEST-123",
		"customer": map[string]any{
			"id":   "CUST-456",
			"tier": "premium",
		},
	}
	WithMetadata(metadata)(input)

	// Verify the option was applied correctly
	assert.Equal(t, metadata, input.Metadata)
}

func TestWithChartOfAccountsGroupName(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Apply the option
	WithChartOfAccountsGroupName("revenue:subscription")(input)

	// Verify the option was applied correctly
	assert.Equal(t, "revenue:subscription", input.ChartOfAccountsGroupName)
}

func TestWithCode(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Apply the option
	WithCode("SUBS-RENEW")(input)

	// Verify the option was applied correctly
	assert.Equal(t, "SUBS-RENEW", input.Code)
}

func TestWithPending(t *testing.T) {
	// Test cases
	tests := []struct {
		name     string
		pending  bool
		expected bool
	}{
		{
			name:     "Set pending true",
			pending:  true,
			expected: true,
		},
		{
			name:     "Set pending false",
			pending:  false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test input
			input := &models.TransactionDSLInput{}

			// Apply the option
			WithPending(tt.pending)(input)

			// Verify the option was applied correctly
			assert.Equal(t, tt.expected, input.Pending)
		})
	}
}

func TestWithIdempotencyKey(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Apply the option
	WithIdempotencyKey("payment-2023-03-15-12345")(input)

	// Verify the option was applied correctly
	assert.NotNil(t, input.Metadata)
	assert.Equal(t, "payment-2023-03-15-12345", input.Metadata["idempotencyKey"])
}

func TestWithExternalID(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Apply the option
	WithExternalID("PO-12345")(input)

	// Verify the option was applied correctly
	assert.NotNil(t, input.Metadata)
	assert.Equal(t, "PO-12345", input.Metadata["externalID"])
}

func TestWithNotes(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Apply the option
	WithNotes("Customer requested refund due to damaged product.")(input)

	// Verify the option was applied correctly
	assert.NotNil(t, input.Metadata)
	assert.Equal(t, "Customer requested refund due to damaged product.", input.Metadata["notes"])
}

func TestWithRequestID(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Apply the option
	WithRequestID("req-abc-123-xyz")(input)

	// Verify the option was applied correctly
	assert.NotNil(t, input.Metadata)
	assert.Equal(t, "req-abc-123-xyz", input.Metadata["requestID"])
}

func TestWithSendingOptions(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Create test data
	asset := "USD"
	value := int64(10000)
	scale := int64(2)
	source := &models.DSLSource{
		From: []models.DSLFromTo{
			{
				Account: "customer:john.doe",
				Amount: &models.DSLAmount{
					Value: 10000,
					Scale: 2,
					Asset: "USD",
				},
			},
		},
	}
	distribute := &models.DSLDistribute{
		To: []models.DSLFromTo{
			{
				Account: "merchant:acme",
				Amount: &models.DSLAmount{
					Value: 10000,
					Scale: 2,
					Asset: "USD",
				},
			},
		},
	}

	// Apply the option
	WithSendingOptions(asset, value, scale, source, distribute)(input)

	// Verify the option was applied correctly
	assert.Equal(t, asset, input.Send.Asset)
	assert.Equal(t, value, input.Send.Value)
	assert.Equal(t, scale, input.Send.Scale)
	assert.Equal(t, source, input.Send.Source)
	assert.Equal(t, distribute, input.Send.Distribute)
}

func TestWithFromTo(t *testing.T) {
	// Test cases
	tests := []struct {
		name            string
		account         string
		amount          *models.DSLAmount
		isSource        bool
		description     string
		chartOfAccounts string
		metadata        map[string]any
		validateInput   func(t *testing.T, input *models.TransactionDSLInput)
	}{
		{
			name:    "Add source entry",
			account: "customer:john.doe",
			amount: &models.DSLAmount{
				Value: 10000,
				Scale: 2,
				Asset: "USD",
			},
			isSource:        true,
			description:     "Test source",
			chartOfAccounts: "assets:cash",
			metadata: map[string]any{
				"reference": "SRC-123",
			},
			validateInput: func(t *testing.T, input *models.TransactionDSLInput) {
				// Ensure Send and Source are initialized
				assert.NotNil(t, input.Send)
				assert.NotNil(t, input.Send.Source)

				// Validate the From entry
				assert.Len(t, input.Send.Source.From, 1)
				assert.Equal(t, "customer:john.doe", input.Send.Source.From[0].Account)
				assert.Equal(t, int64(10000), input.Send.Source.From[0].Amount.Value)
				assert.Equal(t, int64(2), input.Send.Source.From[0].Amount.Scale)
				assert.Equal(t, "USD", input.Send.Source.From[0].Amount.Asset)
				assert.Equal(t, "Test source", input.Send.Source.From[0].Description)
				assert.Equal(t, "assets:cash", input.Send.Source.From[0].ChartOfAccounts)
				assert.Equal(t, "SRC-123", input.Send.Source.From[0].Metadata["reference"])
			},
		},
		{
			name:    "Add target entry",
			account: "merchant:acme",
			amount: &models.DSLAmount{
				Value: 10000,
				Scale: 2,
				Asset: "USD",
			},
			isSource:        false,
			description:     "Test target",
			chartOfAccounts: "liabilities:accounts_payable",
			metadata: map[string]any{
				"reference": "TGT-123",
			},
			validateInput: func(t *testing.T, input *models.TransactionDSLInput) {
				// Ensure Send and Distribute are initialized
				assert.NotNil(t, input.Send)
				assert.NotNil(t, input.Send.Distribute)

				// Validate the To entry
				assert.Len(t, input.Send.Distribute.To, 1)
				assert.Equal(t, "merchant:acme", input.Send.Distribute.To[0].Account)
				assert.Equal(t, int64(10000), input.Send.Distribute.To[0].Amount.Value)
				assert.Equal(t, int64(2), input.Send.Distribute.To[0].Amount.Scale)
				assert.Equal(t, "USD", input.Send.Distribute.To[0].Amount.Asset)
				assert.Equal(t, "Test target", input.Send.Distribute.To[0].Description)
				assert.Equal(t, "liabilities:accounts_payable", input.Send.Distribute.To[0].ChartOfAccounts)
				assert.Equal(t, "TGT-123", input.Send.Distribute.To[0].Metadata["reference"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test input
			input := &models.TransactionDSLInput{
				Send: &models.DSLSend{},
			}

			// Apply the option
			WithFromTo(
				tt.account,
				tt.amount,
				tt.isSource,
				tt.description,
				tt.chartOfAccounts,
				tt.metadata,
			)(input)

			// Validate the input
			tt.validateInput(t, input)
		})
	}
}

func TestWithShare(t *testing.T) {
	// Create a test input with a FromTo entry
	input := &models.TransactionDSLInput{
		Send: &models.DSLSend{
			Distribute: &models.DSLDistribute{
				To: []models.DSLFromTo{
					{
						Account: "merchant:acme",
						Amount: &models.DSLAmount{
							Value: 10000,
							Scale: 2,
							Asset: "USD",
						},
					},
				},
			},
		},
	}

	// Apply the option
	percentage := int64(50)
	percentageOfPercentage := int64(100)
	WithShare(percentage, percentageOfPercentage)(input)

	// Verify the option was applied correctly
	assert.NotNil(t, input.Send.Distribute.To[0].Share)
	assert.Equal(t, percentage, input.Send.Distribute.To[0].Share.Percentage)
	assert.Equal(t, percentageOfPercentage, input.Send.Distribute.To[0].Share.PercentageOfPercentage)
}

func TestWithRate(t *testing.T) {
	// Create a test input with a FromTo entry
	input := &models.TransactionDSLInput{
		Send: &models.DSLSend{
			Distribute: &models.DSLDistribute{
				To: []models.DSLFromTo{
					{
						Account: "merchant:acme",
						Amount: &models.DSLAmount{
							Value: 10000,
							Scale: 2,
							Asset: "USD",
						},
					},
				},
			},
		},
	}

	// Apply the option
	from := "USD"
	to := "EUR"
	value := int64(85)
	scale := int64(2)
	externalID := "RATE-123"
	WithRate(from, to, value, scale, externalID)(input)

	// Verify the option was applied correctly
	assert.NotNil(t, input.Send.Distribute.To[0].Rate)
	assert.Equal(t, from, input.Send.Distribute.To[0].Rate.From)
	assert.Equal(t, to, input.Send.Distribute.To[0].Rate.To)
	assert.Equal(t, value, input.Send.Distribute.To[0].Rate.Value)
	assert.Equal(t, scale, input.Send.Distribute.To[0].Rate.Scale)
	assert.Equal(t, externalID, input.Send.Distribute.To[0].Rate.ExternalID)
}

func TestMultipleOptions(t *testing.T) {
	// Create a test input
	input := &models.TransactionDSLInput{}

	// Apply multiple options
	options := []Option{
		WithMetadata(map[string]any{"reference": "TEST-123"}),
		WithIdempotencyKey("payment-2023-03-15-12345"),
		WithExternalID("PO-12345"),
		WithPending(true),
		WithNotes("Test transaction"),
		WithCode("TEST"),
		WithChartOfAccountsGroupName("revenue:sales"),
		WithRequestID("req-abc-123"),
	}

	// Apply all options
	for _, option := range options {
		option(input)
	}

	// Verify all options were applied correctly
	assert.Equal(t, "TEST-123", input.Metadata["reference"])
	assert.Equal(t, "payment-2023-03-15-12345", input.Metadata["idempotencyKey"])
	assert.Equal(t, "PO-12345", input.Metadata["externalID"])
	assert.True(t, input.Pending)
	assert.Equal(t, "Test transaction", input.Metadata["notes"])
	assert.Equal(t, "TEST", input.Code)
	assert.Equal(t, "revenue:sales", input.ChartOfAccountsGroupName)
	assert.Equal(t, "req-abc-123", input.Metadata["requestID"])
}
