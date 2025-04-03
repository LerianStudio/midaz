package builders

import (
	"context"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockOrganizationClient is a simple implementation of OrganizationClientInterface for testing
type mockOrganizationClient struct {
	createFunc func(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error)
	updateFunc func(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error)
}

// func (m *mockOrganizationClient) CreateOrganization(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error) { performs an operation
func (m *mockOrganizationClient) CreateOrganization(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, input)
	}
	return &models.Organization{ID: "org-123"}, nil
}

// func (m *mockOrganizationClient) UpdateOrganization(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error) { performs an operation
func (m *mockOrganizationClient) UpdateOrganization(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, id, input)
	}
	return &models.Organization{ID: id}, nil
}

// \1 performs an operation
func TestOrganizationBuilder(t *testing.T) {
	t.Run("Create with basic fields", func(t *testing.T) {
		client := &mockOrganizationClient{
			createFunc: func(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error) {
				// Verify input values are correctly set
				if input.LegalName != "Test Organization" {
					t.Errorf("unexpected legal name, got: %s, want: Test Organization", input.LegalName)
				}

				if input.LegalDocument != "12345678" {
					t.Errorf("unexpected legal document, got: %s, want: 12345678", input.LegalDocument)
				}

				if input.Status.Code != "ACTIVE" {
					t.Errorf("unexpected status, got: %s, want: ACTIVE", input.Status.Code)
				}

				return &models.Organization{
					ID:            "org-123",
					LegalName:     input.LegalName,
					LegalDocument: input.LegalDocument,
					Status:        input.Status,
				}, nil
			},
		}

		builder := NewOrganization(client).
			WithLegalName("Test Organization").
			WithLegalDocument("12345678")

		org, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if org.ID != "org-123" {
			t.Errorf("unexpected ID, got: %s, want: org-123", org.ID)
		}

		if org.LegalName != "Test Organization" {
			t.Errorf("unexpected legal name, got: %s, want: Test Organization", org.LegalName)
		}
	})

	t.Run("Create with address", func(t *testing.T) {
		client := &mockOrganizationClient{
			createFunc: func(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error) {
				// Verify address fields
				if input.Address.Line1 != "123 Main St" {
					t.Errorf("unexpected street, got: %s, want: 123 Main St", input.Address.Line1)
				}

				if input.Address.ZipCode != "12345" {
					t.Errorf("unexpected postal code, got: %s, want: 12345", input.Address.ZipCode)
				}

				return &models.Organization{
					ID:            "org-123",
					LegalName:     input.LegalName,
					LegalDocument: input.LegalDocument,
					Address:       input.Address,
				}, nil
			},
		}

		builder := NewOrganization(client).
			WithLegalName("Test Organization").
			WithLegalDocument("12345678").
			WithAddress("123 Main St", "12345", "Anytown", "State", "Country")

		org, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if org.Address.Line1 != "123 Main St" {
			t.Errorf("unexpected street, got: %s, want: 123 Main St", org.Address.Line1)
		}
	})

	t.Run("Create with metadata and tags", func(t *testing.T) {
		client := &mockOrganizationClient{
			createFunc: func(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Organization{
					ID:       "org-123",
					Metadata: input.Metadata,
				}, nil
			},
		}

		builder := NewOrganization(client).
			WithLegalName("Test Organization").
			WithLegalDocument("12345678").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		org, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if org.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", org.Metadata["key1"])
		}

		if org.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", org.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockOrganizationClient{}

		// Missing legal name
		builder1 := NewOrganization(client).
			WithLegalDocument("12345678")

		_, err := builder1.Create(context.Background())

		if err == nil {
			t.Fatal("expected error for missing legal name, got nil")
		}

		if !strings.Contains(err.Error(), "legal name is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: legal name is required", err.Error())
		}

		// Missing legal document
		builder2 := NewOrganization(client).
			WithLegalName("Test Organization")

		_, err = builder2.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing legal document, got nil")
		}

		if !strings.Contains(err.Error(), "legal document is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: legal document is required", err.Error())
		}
	})
}

// \1 performs an operation
func TestOrganizationUpdateBuilder(t *testing.T) {
	t.Run("Update organization", func(t *testing.T) {
		client := &mockOrganizationClient{
			updateFunc: func(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error) {
				// Verify ID
				if id != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", id)
				}

				// Verify update fields
				if input.LegalName != "Updated Name" {
					t.Errorf("unexpected legal name, got: %s, want: Updated Name", input.LegalName)
				}

				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Organization{
					ID:        id,
					LegalName: input.LegalName,
					Status:    input.Status,
				}, nil
			},
		}

		builder := NewOrganizationUpdate(client, "org-123").
			WithLegalName("Updated Name").
			WithStatus("INACTIVE")

		org, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if org.ID != "org-123" {
			t.Errorf("unexpected ID, got: %s, want: org-123", org.ID)
		}

		if org.LegalName != "Updated Name" {
			t.Errorf("unexpected legal name, got: %s, want: Updated Name", org.LegalName)
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockOrganizationClient{}

		// Missing organization ID
		builder1 := NewOrganizationUpdate(client, "").
			WithLegalName("Updated Name")

		_, err := builder1.Update(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// No fields to update
		builder2 := NewOrganizationUpdate(client, "org-123")

		_, err = builder2.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for no fields to update, got nil")
		}

		if !strings.Contains(err.Error(), "no fields to update") {
			t.Errorf("unexpected error message, got: %s, want to contain: no fields to update", err.Error())
		}
	})
}
