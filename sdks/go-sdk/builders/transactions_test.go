package builders

import (
	"context"
	"strings"
	"testing"

	"fmt"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockClient is a simple implementation of ClientInterface for testing
type mockClient struct {
	createTransactionFunc func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error)
}

// func (m *mockClient) CreateTransaction(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) { performs an operation
func (m *mockClient) CreateTransaction(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
	if m.createTransactionFunc != nil {
		return m.createTransactionFunc(ctx, orgID, ledgerID, input)
	}
	return &models.Transaction{ID: "tx_123"}, nil
}

// \1 performs an operation
func TestDepositBuilder(t *testing.T) {
	client := &mockClient{}
	builder := NewDeposit(client)

	// Test method chaining
	builder = builder.
		WithOrganization("org-123").
		WithLedger("ledger-456").
		WithAmount(10000, 2).
		WithAssetCode("USD").
		WithDescription("Test deposit").
		ToAccount("customer-account").
		WithMetadata(map[string]any{"reference": "REF123"}).
		WithTag("deposit").
		WithExternalID("ext-123").
		WithIdempotencyKey("idempotency-123")

	// Test validation
	t.Run("Validation", func(t *testing.T) {
		tests := []struct {
			name          string
			modify        func(DepositBuilder) DepositBuilder
			expectedError string
		}{
			{
				name: "Missing organization",
				modify: func(b DepositBuilder) DepositBuilder {
					// Create a new builder with everything except organization
					return NewDeposit(client).
						WithLedger("ledger-456").
						WithAmount(10000, 2).
						WithAssetCode("USD").
						WithDescription("Test deposit").
						ToAccount("customer-account")
				},
				expectedError: "organization ID is required",
			},
			{
				name: "Missing ledger",
				modify: func(b DepositBuilder) DepositBuilder {
					return NewDeposit(client).
						WithOrganization("org-123").
						WithAmount(10000, 2).
						WithAssetCode("USD").
						WithDescription("Test deposit").
						ToAccount("customer-account")
				},
				expectedError: "ledger ID is required",
			},
			{
				name: "Missing amount",
				modify: func(b DepositBuilder) DepositBuilder {
					return NewDeposit(client).
						WithOrganization("org-123").
						WithLedger("ledger-456").
						WithAssetCode("USD").
						WithDescription("Test deposit").
						ToAccount("customer-account")
				},
				expectedError: "amount must be greater than zero",
			},
			{
				name: "Negative amount",
				modify: func(b DepositBuilder) DepositBuilder {
					return NewDeposit(client).
						WithOrganization("org-123").
						WithLedger("ledger-456").
						WithAmount(-100, 2).
						WithAssetCode("USD").
						WithDescription("Test deposit").
						ToAccount("customer-account")
				},
				expectedError: "amount must be greater than zero",
			},
			{
				name: "Missing asset code",
				modify: func(b DepositBuilder) DepositBuilder {
					return NewDeposit(client).
						WithOrganization("org-123").
						WithLedger("ledger-456").
						WithAmount(10000, 2).
						WithDescription("Test deposit").
						ToAccount("customer-account")
				},
				expectedError: "asset code is required",
			},
			{
				name: "Missing target account",
				modify: func(b DepositBuilder) DepositBuilder {
					return NewDeposit(client).
						WithOrganization("org-123").
						WithLedger("ledger-456").
						WithAmount(10000, 2).
						WithAssetCode("USD").
						WithDescription("Test deposit")
				},
				expectedError: "target account is required for a deposit",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				b := tc.modify(builder)
				_, err := b.Execute(context.Background())

				if err == nil {
					t.Fatalf("expected error but got nil")
				}

				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("unexpected error message, got: %s, want to contain: %s", err.Error(), tc.expectedError)
				}
			})
		}
	})

	// Test successful execution
	t.Run("Successful execution", func(t *testing.T) {
		client := &mockClient{
			createTransactionFunc: func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
				// Verify input values are correctly set
				if orgID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", orgID)
				}

				if ledgerID != "ledger-456" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-456", ledgerID)
				}

				if input.Description != "Test deposit" {
					t.Errorf("unexpected description, got: %s, want: Test deposit", input.Description)
				}

				if input.Send.Asset != "USD" {
					t.Errorf("unexpected asset, got: %s, want: USD", input.Send.Asset)
				}

				if input.Send.Value != 10000 {
					t.Errorf("unexpected amount, got: %d, want: 10000", input.Send.Value)
				}

				if input.Send.Scale != 2 {
					t.Errorf("unexpected scale, got: %d, want: 2", input.Send.Scale)
				}

				if input.Send.Distribute.To[0].Account != "customer-account" {
					t.Errorf("unexpected target account, got: %s, want: customer-account", input.Send.Distribute.To[0].Account)
				}

				if input.Send.Source.From[0].Account != "@external/USD" {
					t.Errorf("unexpected source account, got: %s, want: @external/USD", input.Send.Source.From[0].Account)
				}

				if input.Metadata["reference"] != "REF123" {
					t.Errorf("unexpected metadata reference, got: %v, want: REF123", input.Metadata["reference"])
				}

				if input.Metadata["tags"] != "deposit" {
					t.Errorf("unexpected metadata tags, got: %v, want: deposit", input.Metadata["tags"])
				}

				if input.Metadata["externalId"] != "ext-123" {
					t.Errorf("unexpected metadata externalId, got: %v, want: ext-123", input.Metadata["externalId"])
				}

				if input.Metadata["idempotencyKey"] != "idempotency-123" {
					t.Errorf("unexpected metadata idempotencyKey, got: %v, want: idempotency-123", input.Metadata["idempotencyKey"])
				}

				return &models.Transaction{
					ID:       "tx_123",
					Status:   models.NewStatus("COMPLETED"),
					Metadata: map[string]any{"description": input.Description},
				}, nil
			},
		}

		builder := NewDeposit(client).
			WithOrganization("org-123").
			WithLedger("ledger-456").
			WithAmount(10000, 2).
			WithAssetCode("USD").
			WithDescription("Test deposit").
			ToAccount("customer-account").
			WithMetadata(map[string]any{"reference": "REF123"}).
			WithTag("deposit").
			WithExternalID("ext-123").
			WithIdempotencyKey("idempotency-123")

		tx, err := builder.Execute(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tx.ID != "tx_123" {
			t.Errorf("unexpected transaction ID, got: %s, want: tx_123", tx.ID)
		}

		if desc, ok := tx.Metadata["description"].(string); !ok || desc != "Test deposit" {
			t.Errorf("unexpected transaction description in metadata, got: %v, want: Test deposit", tx.Metadata["description"])
		}

		if tx.Status.Code != "COMPLETED" {
			t.Errorf("unexpected transaction status, got: %s, want: COMPLETED", tx.Status.Code)
		}
	})
}

// \1 performs an operation
func TestWithdrawalBuilder(t *testing.T) {
	// Test successful execution
	t.Run("Successful execution", func(t *testing.T) {
		client := &mockClient{
			createTransactionFunc: func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
				// Verify source and destination accounts
				if input.Send.Source.From[0].Account != "source-account" {
					t.Errorf("unexpected source account, got: %s, want: source-account", input.Send.Source.From[0].Account)
				}

				if input.Send.Distribute.To[0].Account != "@external/USD" {
					t.Errorf("unexpected target account, got: %s, want: @external/USD", input.Send.Distribute.To[0].Account)
				}

				return &models.Transaction{
					ID:       "tx_withdrawal",
					Status:   models.NewStatus("COMPLETED"),
					Metadata: map[string]any{"description": input.Description},
				}, nil
			},
		}

		builder := NewWithdrawal(client).
			WithOrganization("org-123").
			WithLedger("ledger-456").
			WithAmount(10000, 2).
			WithAssetCode("USD").
			WithDescription("Test withdrawal").
			FromAccount("source-account").
			WithTag("withdrawal")

		tx, err := builder.Execute(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tx.ID != "tx_withdrawal" {
			t.Errorf("unexpected transaction ID, got: %s, want: tx_withdrawal", tx.ID)
		}
	})

	// Test validation error
	t.Run("Missing source account", func(t *testing.T) {
		client := &mockClient{}
		builder := NewWithdrawal(client).
			WithOrganization("org-123").
			WithLedger("ledger-456").
			WithAmount(10000, 2).
			WithAssetCode("USD").
			WithDescription("Test withdrawal")

		_, err := builder.Execute(context.Background())

		if err == nil {
			t.Fatalf("expected error but got nil")
		}

		if !strings.Contains(err.Error(), "source account is required for a withdrawal") {
			t.Errorf("unexpected error message, got: %s, want to contain: source account is required for a withdrawal", err.Error())
		}
	})
}

// \1 performs an operation
func TestTransferBuilder(t *testing.T) {
	// Test successful execution
	t.Run("Successful execution", func(t *testing.T) {
		client := &mockClient{
			createTransactionFunc: func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
				// Verify source and destination accounts
				if input.Send.Source.From[0].Account != "source-account" {
					t.Errorf("unexpected source account, got: %s, want: source-account", input.Send.Source.From[0].Account)
				}

				if input.Send.Distribute.To[0].Account != "target-account" {
					t.Errorf("unexpected target account, got: %s, want: target-account", input.Send.Distribute.To[0].Account)
				}

				return &models.Transaction{
					ID:       "tx_transfer",
					Status:   models.NewStatus("COMPLETED"),
					Metadata: map[string]any{"description": input.Description},
				}, nil
			},
		}

		builder := NewTransfer(client).
			WithOrganization("org-123").
			WithLedger("ledger-456").
			WithAmount(10000, 2).
			WithAssetCode("USD").
			WithDescription("Test transfer").
			FromAccount("source-account").
			ToAccount("target-account").
			WithTag("transfer")

		tx, err := builder.Execute(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tx.ID != "tx_transfer" {
			t.Errorf("unexpected transaction ID, got: %s, want: tx_transfer", tx.ID)
		}
	})

	// Test validation errors
	t.Run("Missing accounts", func(t *testing.T) {
		tests := []struct {
			name          string
			builder       TransferBuilder
			expectedError string
		}{
			{
				name: "Missing source account",
				builder: NewTransfer(&mockClient{}).
					WithOrganization("org-123").
					WithLedger("ledger-456").
					WithAmount(10000, 2).
					WithAssetCode("USD").
					WithDescription("Test transfer").
					ToAccount("target-account"),
				expectedError: "source account is required for a transfer",
			},
			{
				name: "Missing target account",
				builder: NewTransfer(&mockClient{}).
					WithOrganization("org-123").
					WithLedger("ledger-456").
					WithAmount(10000, 2).
					WithAssetCode("USD").
					WithDescription("Test transfer").
					FromAccount("source-account"),
				expectedError: "target account is required for a transfer",
			},
			{
				name: "Same source and target account",
				builder: NewTransfer(&mockClient{}).
					WithOrganization("org-123").
					WithLedger("ledger-456").
					WithAmount(10000, 2).
					WithAssetCode("USD").
					WithDescription("Test transfer").
					FromAccount("same-account").
					ToAccount("same-account"),
				expectedError: "source and target accounts must be different",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := tc.builder.Execute(context.Background())

				if err == nil {
					t.Fatalf("expected error but got nil")
				}

				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("unexpected error message, got: %s, want to contain: %s", err.Error(), tc.expectedError)
				}
			})
		}
	})
}

// \1 performs an operation
func TestMultipleTagsCombining(t *testing.T) {
	client := &mockClient{
		createTransactionFunc: func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
			tags, ok := input.Metadata["tags"].(string)

			if !ok {
				t.Errorf("expected tags to be a string, got: %T", input.Metadata["tags"])
				return nil, fmt.Errorf("tags must be a string")
			}

			expectedTags := "tag1,tag2,tag3,tag4"

			if tags != expectedTags {
				t.Errorf("unexpected tags, got: %s, want: %s", tags, expectedTags)
			}

			return &models.Transaction{ID: "tx_123"}, nil
		},
	}

	builder := NewDeposit(client).
		WithOrganization("org-123").
		WithLedger("ledger-456").
		WithAmount(10000, 2).
		WithAssetCode("USD").
		ToAccount("account").
		WithTag("tag1").
		WithTag("tag2").
		WithTags([]string{"tag3", "tag4"})

	_, err := builder.Execute(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
