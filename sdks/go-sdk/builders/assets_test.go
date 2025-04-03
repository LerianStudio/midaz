package builders

import (
	"context"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockAssetClient is a simple implementation of AssetClientInterface for testing
type mockAssetClient struct {
	createAssetFunc func(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error)
	updateAssetFunc func(ctx context.Context, organizationID, ledgerID, assetID string, input *models.UpdateAssetInput) (*models.Asset, error)
}

// func (m *mockAssetClient) CreateAsset(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error) { performs an operation
func (m *mockAssetClient) CreateAsset(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error) {
	if m.createAssetFunc != nil {
		return m.createAssetFunc(ctx, organizationID, ledgerID, input)
	}
	return &models.Asset{ID: "asset-123"}, nil
}

// func (m *mockAssetClient) UpdateAsset(ctx context.Context, organizationID, ledgerID, assetID string, input *models.UpdateAssetInput) (*models.Asset, error) { performs an operation
func (m *mockAssetClient) UpdateAsset(ctx context.Context, organizationID, ledgerID, assetID string, input *models.UpdateAssetInput) (*models.Asset, error) {
	if m.updateAssetFunc != nil {
		return m.updateAssetFunc(ctx, organizationID, ledgerID, assetID, input)
	}
	return &models.Asset{ID: assetID}, nil
}

// \1 performs an operation
func TestAssetBuilder(t *testing.T) {
	t.Run("Create with basic fields", func(t *testing.T) {
		client := &mockAssetClient{
			createAssetFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error) {
				// Verify input values are correctly set
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if input.Name != "Test Asset" {
					t.Errorf("unexpected name, got: %s, want: Test Asset", input.Name)
				}

				if input.Code != "TST" {
					t.Errorf("unexpected code, got: %s, want: TST", input.Code)
				}

				if input.Status.Code != "ACTIVE" {
					t.Errorf("unexpected status, got: %s, want: ACTIVE", input.Status.Code)
				}

				return &models.Asset{
					ID:             "asset-123",
					Name:           input.Name,
					Code:           input.Code,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewAsset(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Asset").
			WithCode("TST")

		asset, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if asset.ID != "asset-123" {
			t.Errorf("unexpected ID, got: %s, want: asset-123", asset.ID)
		}

		if asset.Name != "Test Asset" {
			t.Errorf("unexpected name, got: %s, want: Test Asset", asset.Name)
		}

		if asset.Code != "TST" {
			t.Errorf("unexpected code, got: %s, want: TST", asset.Code)
		}

		if asset.OrganizationID != "org-123" {
			t.Errorf("unexpected organization ID, got: %s, want: org-123", asset.OrganizationID)
		}

		if asset.LedgerID != "ledger-123" {
			t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", asset.LedgerID)
		}
	})

	t.Run("Create with type and custom status", func(t *testing.T) {
		client := &mockAssetClient{
			createAssetFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error) {
				// Verify type and status fields
				if input.Type != "CURRENCY" {
					t.Errorf("unexpected type, got: %s, want: CURRENCY", input.Type)
				}

				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Asset{
					ID:             "asset-123",
					Name:           input.Name,
					Code:           input.Code,
					Type:           input.Type,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewAsset(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Asset").
			WithCode("TST").
			WithType("CURRENCY").
			WithStatus("INACTIVE")

		asset, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if asset.Type != "CURRENCY" {
			t.Errorf("unexpected type, got: %s, want: CURRENCY", asset.Type)
		}

		if asset.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", asset.Status.Code)
		}
	})

	t.Run("Create with metadata and tags", func(t *testing.T) {
		client := &mockAssetClient{
			createAssetFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Asset{
					ID:             "asset-123",
					Name:           input.Name,
					Code:           input.Code,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewAsset(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Asset").
			WithCode("TST").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		asset, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if asset.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", asset.Metadata["key1"])
		}

		if asset.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", asset.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockAssetClient{}

		// Missing organization ID
		builder1 := NewAsset(client).
			WithLedger("ledger-123").
			WithName("Test Asset").
			WithCode("TST")

		_, err := builder1.Create(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewAsset(client).
			WithOrganization("org-123").
			WithName("Test Asset").
			WithCode("TST")

		_, err = builder2.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing name
		builder3 := NewAsset(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithCode("TST")

		_, err = builder3.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing name, got nil")
		}

		if !strings.Contains(err.Error(), "name is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: name is required", err.Error())
		}

		// Missing code
		builder4 := NewAsset(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Asset")

		_, err = builder4.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing code, got nil")
		}

		if !strings.Contains(err.Error(), "code is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: code is required", err.Error())
		}
	})
}

// \1 performs an operation
func TestAssetUpdateBuilder(t *testing.T) {
	t.Run("Update asset", func(t *testing.T) {
		client := &mockAssetClient{
			updateAssetFunc: func(ctx context.Context, organizationID, ledgerID, assetID string, input *models.UpdateAssetInput) (*models.Asset, error) {
				// Verify IDs
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if assetID != "asset-123" {
					t.Errorf("unexpected asset ID, got: %s, want: asset-123", assetID)
				}

				// Verify update fields
				if input.Name != "Updated Asset" {
					t.Errorf("unexpected name, got: %s, want: Updated Asset", input.Name)
				}

				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Asset{
					ID:             assetID,
					Name:           input.Name,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewAssetUpdate(client, "org-123", "ledger-123", "asset-123").
			WithName("Updated Asset").
			WithStatus("INACTIVE")

		asset, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if asset.ID != "asset-123" {
			t.Errorf("unexpected ID, got: %s, want: asset-123", asset.ID)
		}

		if asset.Name != "Updated Asset" {
			t.Errorf("unexpected name, got: %s, want: Updated Asset", asset.Name)
		}

		if asset.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", asset.Status.Code)
		}
	})

	t.Run("Update with metadata", func(t *testing.T) {
		client := &mockAssetClient{
			updateAssetFunc: func(ctx context.Context, organizationID, ledgerID, assetID string, input *models.UpdateAssetInput) (*models.Asset, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Asset{
					ID:             assetID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewAssetUpdate(client, "org-123", "ledger-123", "asset-123").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		asset, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if asset.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", asset.Metadata["key1"])
		}

		if asset.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", asset.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockAssetClient{}

		// Missing organization ID
		builder1 := NewAssetUpdate(client, "", "ledger-123", "asset-123").
			WithName("Updated Asset")

		_, err := builder1.Update(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewAssetUpdate(client, "org-123", "", "asset-123").
			WithName("Updated Asset")

		_, err = builder2.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing asset ID
		builder3 := NewAssetUpdate(client, "org-123", "ledger-123", "").
			WithName("Updated Asset")

		_, err = builder3.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing asset ID, got nil")
		}

		if !strings.Contains(err.Error(), "asset ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: asset ID is required", err.Error())
		}

		// No fields to update
		builder4 := NewAssetUpdate(client, "org-123", "ledger-123", "asset-123")

		_, err = builder4.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for no fields to update, got nil")
		}

		if !strings.Contains(err.Error(), "no fields specified for update") {
			t.Errorf("unexpected error message, got: %s, want to contain: no fields specified for update", err.Error())
		}
	})
}
