package builders

import (
	"context"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockBalanceClient is a simple implementation of BalanceClientInterface for testing
type mockBalanceClient struct {
	updateBalanceFunc func(ctx context.Context, organizationID, ledgerID, balanceID string, input *UpdateBalance) (*models.Balance, error)
}

// func (m *mockBalanceClient) UpdateBalance(ctx context.Context, organizationID, ledgerID, balanceID string, input *UpdateBalance) (*models.Balance, error) { performs an operation
func (m *mockBalanceClient) UpdateBalance(ctx context.Context, organizationID, ledgerID, balanceID string, input *UpdateBalance) (*models.Balance, error) {
	if m.updateBalanceFunc != nil {
		return m.updateBalanceFunc(ctx, organizationID, ledgerID, balanceID, input)
	}
	return &models.Balance{ID: balanceID}, nil
}

// \1 performs an operation
func TestBalanceUpdateBuilder(t *testing.T) {
	t.Run("Update allowSending and allowReceiving", func(t *testing.T) {
		allowSending := true
		allowReceiving := false

		client := &mockBalanceClient{
			updateBalanceFunc: func(ctx context.Context, organizationID, ledgerID, balanceID string, input *UpdateBalance) (*models.Balance, error) {
				// Verify IDs
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if balanceID != "balance-123" {
					t.Errorf("unexpected balance ID, got: %s, want: balance-123", balanceID)
				}

				// Verify update fields
				if input.AllowSending == nil || *input.AllowSending != allowSending {
					t.Errorf("unexpected allowSending, got: %v, want: %v", input.AllowSending, allowSending)
				}

				if input.AllowReceiving == nil || *input.AllowReceiving != allowReceiving {
					t.Errorf("unexpected allowReceiving, got: %v, want: %v", input.AllowReceiving, allowReceiving)
				}

				return &models.Balance{
					ID:             balanceID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					AllowSending:   allowSending,
					AllowReceiving: allowReceiving,
				}, nil
			},
		}

		builder := NewBalanceUpdate(client, "org-123", "ledger-123", "balance-123").
			WithAllowSending(allowSending).
			WithAllowReceiving(allowReceiving)

		balance, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if balance.ID != "balance-123" {
			t.Errorf("unexpected ID, got: %s, want: balance-123", balance.ID)
		}

		if balance.AllowSending != allowSending {
			t.Errorf("unexpected allowSending, got: %v, want: %v", balance.AllowSending, allowSending)
		}

		if balance.AllowReceiving != allowReceiving {
			t.Errorf("unexpected allowReceiving, got: %v, want: %v", balance.AllowReceiving, allowReceiving)
		}
	})

	t.Run("Update only allowSending", func(t *testing.T) {
		allowSending := false

		client := &mockBalanceClient{
			updateBalanceFunc: func(ctx context.Context, organizationID, ledgerID, balanceID string, input *UpdateBalance) (*models.Balance, error) {
				// Verify update fields
				if input.AllowSending == nil || *input.AllowSending != allowSending {
					t.Errorf("unexpected allowSending, got: %v, want: %v", input.AllowSending, allowSending)
				}

				if input.AllowReceiving != nil {
					t.Errorf("unexpected allowReceiving, got: %v, want: nil", input.AllowReceiving)
				}

				return &models.Balance{
					ID:           balanceID,
					AllowSending: allowSending,
				}, nil
			},
		}

		builder := NewBalanceUpdate(client, "org-123", "ledger-123", "balance-123").
			WithAllowSending(allowSending)

		balance, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if balance.AllowSending != allowSending {
			t.Errorf("unexpected allowSending, got: %v, want: %v", balance.AllowSending, allowSending)
		}
	})

	t.Run("Update only allowReceiving", func(t *testing.T) {
		allowReceiving := true

		client := &mockBalanceClient{
			updateBalanceFunc: func(ctx context.Context, organizationID, ledgerID, balanceID string, input *UpdateBalance) (*models.Balance, error) {
				// Verify update fields
				if input.AllowSending != nil {
					t.Errorf("unexpected allowSending, got: %v, want: nil", input.AllowSending)
				}

				if input.AllowReceiving == nil || *input.AllowReceiving != allowReceiving {
					t.Errorf("unexpected allowReceiving, got: %v, want: %v", input.AllowReceiving, allowReceiving)
				}

				return &models.Balance{
					ID:             balanceID,
					AllowReceiving: allowReceiving,
				}, nil
			},
		}

		builder := NewBalanceUpdate(client, "org-123", "ledger-123", "balance-123").
			WithAllowReceiving(allowReceiving)

		balance, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if balance.AllowReceiving != allowReceiving {
			t.Errorf("unexpected allowReceiving, got: %v, want: %v", balance.AllowReceiving, allowReceiving)
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockBalanceClient{}

		// Missing organization ID
		builder1 := NewBalanceUpdate(client, "", "ledger-123", "balance-123").
			WithAllowSending(true)

		_, err := builder1.Update(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewBalanceUpdate(client, "org-123", "", "balance-123").
			WithAllowSending(true)

		_, err = builder2.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing balance ID
		builder3 := NewBalanceUpdate(client, "org-123", "ledger-123", "").
			WithAllowSending(true)

		_, err = builder3.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing balance ID, got nil")
		}

		if !strings.Contains(err.Error(), "balance ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: balance ID is required", err.Error())
		}

		// No fields to update
		builder4 := NewBalanceUpdate(client, "org-123", "ledger-123", "balance-123")

		_, err = builder4.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for no fields to update, got nil")
		}

		if !strings.Contains(err.Error(), "no fields to update") {
			t.Errorf("unexpected error message, got: %s, want to contain: no fields to update", err.Error())
		}
	})
}
