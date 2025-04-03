package builders

import (
	"context"
	"strings"
	"testing"

	"fmt"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockLedgerClient is a simple implementation of LedgerClientInterface for testing
type mockLedgerClient struct {
	createLedgerFunc func(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error)
	updateLedgerFunc func(ctx context.Context, organizationID, ledgerID string, input *models.UpdateLedgerInput) (*models.Ledger, error)
}

// func (m *mockLedgerClient) CreateLedger(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error) { performs an operation
func (m *mockLedgerClient) CreateLedger(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error) {
	if m.createLedgerFunc != nil {
		return m.createLedgerFunc(ctx, organizationID, input)
	}
	return &models.Ledger{ID: "ledger-123"}, nil
}

// func (m *mockLedgerClient) UpdateLedger(ctx context.Context, organizationID, ledgerID string, input *models.UpdateLedgerInput) (*models.Ledger, error) { performs an operation
func (m *mockLedgerClient) UpdateLedger(ctx context.Context, organizationID, ledgerID string, input *models.UpdateLedgerInput) (*models.Ledger, error) {
	if m.updateLedgerFunc != nil {
		return m.updateLedgerFunc(ctx, organizationID, ledgerID, input)
	}
	return &models.Ledger{ID: ledgerID}, nil
}

// \1 performs an operation
func TestLedgerBuilder(t *testing.T) {
	t.Run("Create with basic fields", func(t *testing.T) {
		client := &mockLedgerClient{
			createLedgerFunc: func(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error) {
				// Verify input values are correctly set
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if input.Name != "Test Ledger" {
					t.Errorf("unexpected name, got: %s, want: Test Ledger", input.Name)
				}

				if input.Status.Code != "ACTIVE" {
					t.Errorf("unexpected status, got: %s, want: ACTIVE", input.Status.Code)
				}

				return &models.Ledger{
					ID:             "ledger-123",
					Name:           input.Name,
					OrganizationID: organizationID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewLedger(client).
			WithOrganization("org-123").
			WithName("Test Ledger")

		ledger, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ledger.ID != "ledger-123" {
			t.Errorf("unexpected ID, got: %s, want: ledger-123", ledger.ID)
		}

		if ledger.Name != "Test Ledger" {
			t.Errorf("unexpected name, got: %s, want: Test Ledger", ledger.Name)
		}

		if ledger.OrganizationID != "org-123" {
			t.Errorf("unexpected organization ID, got: %s, want: org-123", ledger.OrganizationID)
		}
	})

	t.Run("Create with custom status", func(t *testing.T) {
		client := &mockLedgerClient{
			createLedgerFunc: func(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error) {
				// Verify status field
				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Ledger{
					ID:             "ledger-123",
					Name:           input.Name,
					OrganizationID: organizationID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewLedger(client).
			WithOrganization("org-123").
			WithName("Test Ledger").
			WithStatus("INACTIVE")

		ledger, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ledger.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", ledger.Status.Code)
		}
	})

	t.Run("Create with metadata and tags", func(t *testing.T) {
		client := &mockLedgerClient{
			createLedgerFunc: func(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Ledger{
					ID:             "ledger-123",
					Name:           input.Name,
					OrganizationID: organizationID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewLedger(client).
			WithOrganization("org-123").
			WithName("Test Ledger").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		ledger, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ledger.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", ledger.Metadata["key1"])
		}

		if ledger.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", ledger.Metadata["tags"])
		}
	})

	t.Run("Create with tags array", func(t *testing.T) {
		client := &mockLedgerClient{
			createLedgerFunc: func(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error) {
				// Verify tags in metadata
				tagsStr, ok := input.Metadata["tags"].(string)

				if !ok {
					t.Errorf("expected tags as string in metadata, got: %T", input.Metadata["tags"])
				} else if tagsStr != "tag1,tag2,tag3" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2,tag3", tagsStr)
				}

				return &models.Ledger{
					ID:             "ledger-123",
					Name:           input.Name,
					OrganizationID: organizationID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewLedger(client).
			WithOrganization("org-123").
			WithName("Test Ledger").
			WithTags([]string{"tag1", "tag2", "tag3"})

		ledger, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ledger.Metadata["tags"] != "tag1,tag2,tag3" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2,tag3", ledger.Metadata["tags"])
		}
	})
}

// \1 performs an operation
func TestLedgerUpdateBuilder(t *testing.T) {
	t.Run("Update ledger", func(t *testing.T) {
		client := &mockLedgerClient{
			updateLedgerFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.UpdateLedgerInput) (*models.Ledger, error) {
				// Verify IDs
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				// Verify update fields
				if input.Name != "Updated Ledger" {
					t.Errorf("unexpected name, got: %s, want: Updated Ledger", input.Name)
				}

				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Ledger{
					ID:             ledgerID,
					Name:           input.Name,
					OrganizationID: organizationID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewLedgerUpdate(client, "org-123", "ledger-123").
			WithName("Updated Ledger").
			WithStatus("INACTIVE")

		ledger, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ledger.ID != "ledger-123" {
			t.Errorf("unexpected ID, got: %s, want: ledger-123", ledger.ID)
		}

		if ledger.Name != "Updated Ledger" {
			t.Errorf("unexpected name, got: %s, want: Updated Ledger", ledger.Name)
		}

		if ledger.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", ledger.Status.Code)
		}
	})

	t.Run("Update with metadata", func(t *testing.T) {
		client := &mockLedgerClient{
			updateLedgerFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.UpdateLedgerInput) (*models.Ledger, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Ledger{
					ID:             ledgerID,
					OrganizationID: organizationID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewLedgerUpdate(client, "org-123", "ledger-123").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		ledger, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ledger.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", ledger.Metadata["key1"])
		}

		if ledger.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", ledger.Metadata["tags"])
		}
	})

	t.Run("Update with different organization", func(t *testing.T) {
		client := &mockLedgerClient{
			updateLedgerFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.UpdateLedgerInput) (*models.Ledger, error) {
				// Verify organization ID was updated
				if organizationID != "org-456" {
					t.Errorf("unexpected organization ID, got: %s, want: org-456", organizationID)
				}

				return &models.Ledger{
					ID:             ledgerID,
					OrganizationID: organizationID,
					Name:           input.Name,
				}, nil
			},
		}

		builder := NewLedgerUpdate(client, "org-123", "ledger-123").
			WithOrganization("org-456").
			WithName("Updated Ledger")

		ledger, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ledger.OrganizationID != "org-456" {
			t.Errorf("unexpected organization ID, got: %s, want: org-456", ledger.OrganizationID)
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockLedgerClient{
			updateLedgerFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.UpdateLedgerInput) (*models.Ledger, error) {
				// Return error for empty organization ID
				if organizationID == "" {
					return nil, fmt.Errorf("organization ID is required")
				}

				// Return error for empty ledger ID
				if ledgerID == "" {
					return nil, fmt.Errorf("ledger ID is required")
				}

				return &models.Ledger{ID: ledgerID}, nil
			},
		}

		// Missing organization ID
		builder1 := NewLedgerUpdate(client, "", "ledger-123").
			WithName("Updated Ledger")

		_, err := builder1.Update(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewLedgerUpdate(client, "org-123", "").
			WithName("Updated Ledger")

		_, err = builder2.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// No fields to update
		builder3 := NewLedgerUpdate(client, "org-123", "ledger-123")

		_, err = builder3.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for no fields to update, got nil")
		}

		if !strings.Contains(err.Error(), "no fields specified for update") {
			t.Errorf("unexpected error message, got: %s, want to contain: no fields specified for update", err.Error())
		}
	})
}
