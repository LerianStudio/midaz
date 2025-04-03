package builders

import (
	"context"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockPortfolioClient is a simple implementation of PortfolioClientInterface for testing
type mockPortfolioClient struct {
	createPortfolioFunc func(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error)
	updatePortfolioFunc func(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.UpdatePortfolioInput) (*models.Portfolio, error)
}

// func (m *mockPortfolioClient) CreatePortfolio(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error) { performs an operation
func (m *mockPortfolioClient) CreatePortfolio(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error) {
	if m.createPortfolioFunc != nil {
		return m.createPortfolioFunc(ctx, organizationID, ledgerID, input)
	}
	return &models.Portfolio{ID: "portfolio-123"}, nil
}

// func (m *mockPortfolioClient) UpdatePortfolio(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.UpdatePortfolioInput) (*models.Portfolio, error) { performs an operation
func (m *mockPortfolioClient) UpdatePortfolio(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.UpdatePortfolioInput) (*models.Portfolio, error) {
	if m.updatePortfolioFunc != nil {
		return m.updatePortfolioFunc(ctx, organizationID, ledgerID, portfolioID, input)
	}
	return &models.Portfolio{ID: portfolioID}, nil
}

// \1 performs an operation
func TestPortfolioBuilder(t *testing.T) {
	t.Run("Create with basic fields", func(t *testing.T) {
		client := &mockPortfolioClient{
			createPortfolioFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error) {
				// Verify input values are correctly set
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if input.Name != "Test Portfolio" {
					t.Errorf("unexpected name, got: %s, want: Test Portfolio", input.Name)
				}

				if input.EntityID != "entity-123" {
					t.Errorf("unexpected entity ID, got: %s, want: entity-123", input.EntityID)
				}

				if input.Status.Code != "ACTIVE" {
					t.Errorf("unexpected status, got: %s, want: ACTIVE", input.Status.Code)
				}

				return &models.Portfolio{
					ID:             "portfolio-123",
					Name:           input.Name,
					EntityID:       input.EntityID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewPortfolio(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Portfolio").
			WithEntityID("entity-123")

		portfolio, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if portfolio.ID != "portfolio-123" {
			t.Errorf("unexpected ID, got: %s, want: portfolio-123", portfolio.ID)
		}

		if portfolio.Name != "Test Portfolio" {
			t.Errorf("unexpected name, got: %s, want: Test Portfolio", portfolio.Name)
		}

		if portfolio.EntityID != "entity-123" {
			t.Errorf("unexpected entity ID, got: %s, want: entity-123", portfolio.EntityID)
		}

		if portfolio.OrganizationID != "org-123" {
			t.Errorf("unexpected organization ID, got: %s, want: org-123", portfolio.OrganizationID)
		}

		if portfolio.LedgerID != "ledger-123" {
			t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", portfolio.LedgerID)
		}
	})

	t.Run("Create with custom status", func(t *testing.T) {
		client := &mockPortfolioClient{
			createPortfolioFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error) {
				// Verify status field
				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Portfolio{
					ID:             "portfolio-123",
					Name:           input.Name,
					EntityID:       input.EntityID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewPortfolio(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Portfolio").
			WithEntityID("entity-123").
			WithStatus("INACTIVE")

		portfolio, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if portfolio.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", portfolio.Status.Code)
		}
	})

	t.Run("Create with metadata and tags", func(t *testing.T) {
		client := &mockPortfolioClient{
			createPortfolioFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Portfolio{
					ID:             "portfolio-123",
					Name:           input.Name,
					EntityID:       input.EntityID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewPortfolio(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Portfolio").
			WithEntityID("entity-123").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		portfolio, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if portfolio.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", portfolio.Metadata["key1"])
		}

		if portfolio.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", portfolio.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockPortfolioClient{}

		// Missing organization ID
		builder1 := NewPortfolio(client).
			WithLedger("ledger-123").
			WithName("Test Portfolio").
			WithEntityID("entity-123")

		_, err := builder1.Create(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewPortfolio(client).
			WithOrganization("org-123").
			WithName("Test Portfolio").
			WithEntityID("entity-123")

		_, err = builder2.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing name
		builder3 := NewPortfolio(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithEntityID("entity-123")

		_, err = builder3.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing name, got nil")
		}

		if !strings.Contains(err.Error(), "portfolio name is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: portfolio name is required", err.Error())
		}

		// Missing entity ID
		builder4 := NewPortfolio(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Portfolio")

		_, err = builder4.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing entity ID, got nil")
		}

		if !strings.Contains(err.Error(), "entity ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: entity ID is required", err.Error())
		}
	})
}

// \1 performs an operation
func TestPortfolioUpdateBuilder(t *testing.T) {
	t.Run("Update portfolio", func(t *testing.T) {
		client := &mockPortfolioClient{
			updatePortfolioFunc: func(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.UpdatePortfolioInput) (*models.Portfolio, error) {
				// Verify IDs
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if portfolioID != "portfolio-123" {
					t.Errorf("unexpected portfolio ID, got: %s, want: portfolio-123", portfolioID)
				}

				// Verify update fields
				if input.Name != "Updated Portfolio" {
					t.Errorf("unexpected name, got: %s, want: Updated Portfolio", input.Name)
				}

				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Portfolio{
					ID:             portfolioID,
					Name:           input.Name,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewPortfolioUpdate(client, "org-123", "ledger-123", "portfolio-123").
			WithName("Updated Portfolio").
			WithStatus("INACTIVE")

		portfolio, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if portfolio.ID != "portfolio-123" {
			t.Errorf("unexpected ID, got: %s, want: portfolio-123", portfolio.ID)
		}

		if portfolio.Name != "Updated Portfolio" {
			t.Errorf("unexpected name, got: %s, want: Updated Portfolio", portfolio.Name)
		}

		if portfolio.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", portfolio.Status.Code)
		}
	})

	t.Run("Update with metadata", func(t *testing.T) {
		client := &mockPortfolioClient{
			updatePortfolioFunc: func(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.UpdatePortfolioInput) (*models.Portfolio, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Portfolio{
					ID:             portfolioID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewPortfolioUpdate(client, "org-123", "ledger-123", "portfolio-123").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		portfolio, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if portfolio.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", portfolio.Metadata["key1"])
		}

		if portfolio.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", portfolio.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockPortfolioClient{}

		// Missing organization ID
		builder1 := NewPortfolioUpdate(client, "", "ledger-123", "portfolio-123").
			WithName("Updated Portfolio")

		_, err := builder1.Update(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewPortfolioUpdate(client, "org-123", "", "portfolio-123").
			WithName("Updated Portfolio")

		_, err = builder2.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing portfolio ID
		builder3 := NewPortfolioUpdate(client, "org-123", "ledger-123", "").
			WithName("Updated Portfolio")

		_, err = builder3.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing portfolio ID, got nil")
		}

		if !strings.Contains(err.Error(), "portfolio ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: portfolio ID is required", err.Error())
		}

		// No fields to update
		builder4 := NewPortfolioUpdate(client, "org-123", "ledger-123", "portfolio-123")

		_, err = builder4.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for no fields to update, got nil")
		}

		if !strings.Contains(err.Error(), "no fields to update") {
			t.Errorf("unexpected error message, got: %s, want to contain: no fields to update", err.Error())
		}
	})
}
