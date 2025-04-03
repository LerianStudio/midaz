package builders

import (
	"context"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockAccountClient is a simple implementation of AccountClientInterface for testing
type mockAccountClient struct {
	createAccountFunc func(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error)
	updateAccountFunc func(ctx context.Context, organizationID, ledgerID, accountID string, input *models.UpdateAccountInput) (*models.Account, error)
}

// func (m *mockAccountClient) CreateAccount(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error) { performs an operation
func (m *mockAccountClient) CreateAccount(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error) {
	if m.createAccountFunc != nil {
		return m.createAccountFunc(ctx, organizationID, ledgerID, input)
	}
	return &models.Account{ID: "account-123"}, nil
}

// func (m *mockAccountClient) UpdateAccount(ctx context.Context, organizationID, ledgerID, accountID string, input *models.UpdateAccountInput) (*models.Account, error) { performs an operation
func (m *mockAccountClient) UpdateAccount(ctx context.Context, organizationID, ledgerID, accountID string, input *models.UpdateAccountInput) (*models.Account, error) {
	if m.updateAccountFunc != nil {
		return m.updateAccountFunc(ctx, organizationID, ledgerID, accountID, input)
	}
	return &models.Account{ID: accountID}, nil
}

// \1 performs an operation
func TestAccountBuilder(t *testing.T) {
	t.Run("Create with basic fields", func(t *testing.T) {
		client := &mockAccountClient{
			createAccountFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error) {
				// Verify input values are correctly set
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if input.Name != "Test Account" {
					t.Errorf("unexpected name, got: %s, want: Test Account", input.Name)
				}

				if input.AssetCode != "USD" {
					t.Errorf("unexpected asset code, got: %s, want: USD", input.AssetCode)
				}

				if input.Type != "ASSET" {
					t.Errorf("unexpected type, got: %s, want: ASSET", input.Type)
				}

				if input.Status.Code != "ACTIVE" {
					t.Errorf("unexpected status, got: %s, want: ACTIVE", input.Status.Code)
				}

				return &models.Account{
					ID:             "account-123",
					Name:           input.Name,
					AssetCode:      input.AssetCode,
					Type:           input.Type,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewAccount(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Account").
			WithAssetCode("USD").
			WithType("ASSET")

		account, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if account.ID != "account-123" {
			t.Errorf("unexpected ID, got: %s, want: account-123", account.ID)
		}

		if account.Name != "Test Account" {
			t.Errorf("unexpected name, got: %s, want: Test Account", account.Name)
		}

		if account.AssetCode != "USD" {
			t.Errorf("unexpected asset code, got: %s, want: USD", account.AssetCode)
		}

		if account.Type != "ASSET" {
			t.Errorf("unexpected type, got: %s, want: ASSET", account.Type)
		}

		if account.OrganizationID != "org-123" {
			t.Errorf("unexpected organization ID, got: %s, want: org-123", account.OrganizationID)
		}

		if account.LedgerID != "ledger-123" {
			t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", account.LedgerID)
		}
	})

	t.Run("Create with all optional fields", func(t *testing.T) {
		client := &mockAccountClient{
			createAccountFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error) {
				// Verify optional fields
				if input.ParentAccountID == nil || *input.ParentAccountID != "parent-123" {
					t.Errorf("unexpected parent account ID, got: %v, want: parent-123", input.ParentAccountID)
				}

				if input.EntityID == nil || *input.EntityID != "entity-123" {
					t.Errorf("unexpected entity ID, got: %v, want: entity-123", input.EntityID)
				}

				if input.PortfolioID == nil || *input.PortfolioID != "portfolio-123" {
					t.Errorf("unexpected portfolio ID, got: %v, want: portfolio-123", input.PortfolioID)
				}

				if input.SegmentID == nil || *input.SegmentID != "segment-123" {
					t.Errorf("unexpected segment ID, got: %v, want: segment-123", input.SegmentID)
				}

				if input.Alias == nil || *input.Alias != "test-alias" {
					t.Errorf("unexpected alias, got: %v, want: test-alias", input.Alias)
				}

				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				parentAccountID := "parent-123"
				entityID := "entity-123"
				portfolioID := "portfolio-123"
				segmentID := "segment-123"
				alias := "test-alias"

				return &models.Account{
					ID:              "account-123",
					Name:            input.Name,
					AssetCode:       input.AssetCode,
					Type:            input.Type,
					ParentAccountID: &parentAccountID,
					EntityID:        &entityID,
					PortfolioID:     &portfolioID,
					SegmentID:       &segmentID,
					Alias:           &alias,
					OrganizationID:  organizationID,
					LedgerID:        ledgerID,
					Status:          input.Status,
				}, nil
			},
		}

		builder := NewAccount(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Account").
			WithAssetCode("USD").
			WithType("ASSET").
			WithParentAccount("parent-123").
			WithEntityID("entity-123").
			WithPortfolio("portfolio-123").
			WithSegment("segment-123").
			WithAlias("test-alias").
			WithStatus("INACTIVE")

		account, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if account.ID != "account-123" {
			t.Errorf("unexpected ID, got: %s, want: account-123", account.ID)
		}

		if account.ParentAccountID == nil || *account.ParentAccountID != "parent-123" {
			t.Errorf("unexpected parent account ID, got: %v, want: parent-123", account.ParentAccountID)
		}

		if account.EntityID == nil || *account.EntityID != "entity-123" {
			t.Errorf("unexpected entity ID, got: %v, want: entity-123", account.EntityID)
		}

		if account.PortfolioID == nil || *account.PortfolioID != "portfolio-123" {
			t.Errorf("unexpected portfolio ID, got: %v, want: portfolio-123", account.PortfolioID)
		}

		if account.SegmentID == nil || *account.SegmentID != "segment-123" {
			t.Errorf("unexpected segment ID, got: %v, want: segment-123", account.SegmentID)
		}

		if account.Alias == nil || *account.Alias != "test-alias" {
			t.Errorf("unexpected alias, got: %v, want: test-alias", account.Alias)
		}

		if account.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", account.Status.Code)
		}
	})

	t.Run("Create with metadata and tags", func(t *testing.T) {
		client := &mockAccountClient{
			createAccountFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Account{
					ID:             "account-123",
					Name:           input.Name,
					AssetCode:      input.AssetCode,
					Type:           input.Type,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewAccount(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Account").
			WithAssetCode("USD").
			WithType("ASSET").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		account, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if account.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", account.Metadata["key1"])
		}

		if account.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", account.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockAccountClient{}

		// Missing organization ID
		builder1 := NewAccount(client).
			WithLedger("ledger-123").
			WithName("Test Account").
			WithAssetCode("USD").
			WithType("ASSET")

		_, err := builder1.Create(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewAccount(client).
			WithOrganization("org-123").
			WithName("Test Account").
			WithAssetCode("USD").
			WithType("ASSET")

		_, err = builder2.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing name
		builder3 := NewAccount(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithAssetCode("USD").
			WithType("ASSET")

		_, err = builder3.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing name, got nil")
		}

		if !strings.Contains(err.Error(), "name is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: name is required", err.Error())
		}

		// Missing asset code
		builder4 := NewAccount(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Account").
			WithType("ASSET")

		_, err = builder4.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing asset code, got nil")
		}

		if !strings.Contains(err.Error(), "asset code is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: asset code is required", err.Error())
		}

		// Missing account type
		builder5 := NewAccount(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Account").
			WithAssetCode("USD")

		_, err = builder5.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing account type, got nil")
		}

		if !strings.Contains(err.Error(), "type is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: type is required", err.Error())
		}
	})
}

// \1 performs an operation
func TestAccountUpdateBuilder(t *testing.T) {
	t.Run("Update account", func(t *testing.T) {
		client := &mockAccountClient{
			updateAccountFunc: func(ctx context.Context, organizationID, ledgerID, accountID string, input *models.UpdateAccountInput) (*models.Account, error) {
				// Verify IDs
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if accountID != "account-123" {
					t.Errorf("unexpected account ID, got: %s, want: account-123", accountID)
				}

				// Verify update fields
				if input.Name != "Updated Account" {
					t.Errorf("unexpected name, got: %s, want: Updated Account", input.Name)
				}

				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Account{
					ID:             accountID,
					Name:           input.Name,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewAccountUpdate(client, "org-123", "ledger-123", "account-123").
			WithName("Updated Account").
			WithStatus("INACTIVE")

		account, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if account.ID != "account-123" {
			t.Errorf("unexpected ID, got: %s, want: account-123", account.ID)
		}

		if account.Name != "Updated Account" {
			t.Errorf("unexpected name, got: %s, want: Updated Account", account.Name)
		}

		if account.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", account.Status.Code)
		}
	})

	t.Run("Update with segment and portfolio", func(t *testing.T) {
		client := &mockAccountClient{
			updateAccountFunc: func(ctx context.Context, organizationID, ledgerID, accountID string, input *models.UpdateAccountInput) (*models.Account, error) {
				// Verify optional fields
				if input.SegmentID == nil || *input.SegmentID != "segment-456" {
					t.Errorf("unexpected segment ID, got: %v, want: segment-456", input.SegmentID)
				}

				if input.PortfolioID == nil || *input.PortfolioID != "portfolio-456" {
					t.Errorf("unexpected portfolio ID, got: %v, want: portfolio-456", input.PortfolioID)
				}

				portfolioID := "portfolio-456"
				segmentID := "segment-456"

				return &models.Account{
					ID:             accountID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					PortfolioID:    &portfolioID,
					SegmentID:      &segmentID,
				}, nil
			},
		}

		builder := NewAccountUpdate(client, "org-123", "ledger-123", "account-123").
			WithSegment("segment-456").
			WithPortfolio("portfolio-456")

		account, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if account.PortfolioID == nil || *account.PortfolioID != "portfolio-456" {
			t.Errorf("unexpected portfolio ID, got: %v, want: portfolio-456", account.PortfolioID)
		}

		if account.SegmentID == nil || *account.SegmentID != "segment-456" {
			t.Errorf("unexpected segment ID, got: %v, want: segment-456", account.SegmentID)
		}
	})

	t.Run("Update with metadata", func(t *testing.T) {
		client := &mockAccountClient{
			updateAccountFunc: func(ctx context.Context, organizationID, ledgerID, accountID string, input *models.UpdateAccountInput) (*models.Account, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Account{
					ID:             accountID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewAccountUpdate(client, "org-123", "ledger-123", "account-123").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		account, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if account.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", account.Metadata["key1"])
		}

		if account.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", account.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockAccountClient{}

		// Missing organization ID
		builder1 := NewAccountUpdate(client, "", "ledger-123", "account-123").
			WithName("Updated Account")

		_, err := builder1.Update(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewAccountUpdate(client, "org-123", "", "account-123").
			WithName("Updated Account")

		_, err = builder2.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing account ID
		builder3 := NewAccountUpdate(client, "org-123", "ledger-123", "").
			WithName("Updated Account")

		_, err = builder3.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing account ID, got nil")
		}

		if !strings.Contains(err.Error(), "account ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: account ID is required", err.Error())
		}

		// No fields to update
		builder4 := NewAccountUpdate(client, "org-123", "ledger-123", "account-123")

		_, err = builder4.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for no fields to update, got nil")
		}

		if !strings.Contains(err.Error(), "no fields specified for update") {
			t.Errorf("unexpected error message, got: %s, want to contain: no fields specified for update", err.Error())
		}
	})
}
