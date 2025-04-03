package builders

import (
	"context"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockSegmentClient is a simple implementation of SegmentClientInterface for testing
type mockSegmentClient struct {
	createSegmentFunc func(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error)
	updateSegmentFunc func(ctx context.Context, organizationID, ledgerID, portfolioID, segmentID string, input *models.UpdateSegmentInput) (*models.Segment, error)
}

// func (m *mockSegmentClient) CreateSegment(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error) { performs an operation
func (m *mockSegmentClient) CreateSegment(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error) {
	if m.createSegmentFunc != nil {
		return m.createSegmentFunc(ctx, organizationID, ledgerID, portfolioID, input)
	}
	return &models.Segment{ID: "segment-123"}, nil
}

// func (m *mockSegmentClient) UpdateSegment(ctx context.Context, organizationID, ledgerID, portfolioID, segmentID string, input *models.UpdateSegmentInput) (*models.Segment, error) { performs an operation
func (m *mockSegmentClient) UpdateSegment(ctx context.Context, organizationID, ledgerID, portfolioID, segmentID string, input *models.UpdateSegmentInput) (*models.Segment, error) {
	if m.updateSegmentFunc != nil {
		return m.updateSegmentFunc(ctx, organizationID, ledgerID, portfolioID, segmentID, input)
	}
	return &models.Segment{ID: segmentID}, nil
}

// \1 performs an operation
func TestSegmentBuilder(t *testing.T) {
	t.Run("Create with basic fields", func(t *testing.T) {
		client := &mockSegmentClient{
			createSegmentFunc: func(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error) {
				// Verify input values are correctly set
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if portfolioID != "portfolio-123" {
					t.Errorf("unexpected portfolio ID, got: %s, want: portfolio-123", portfolioID)
				}

				if input.Name != "Test Segment" {
					t.Errorf("unexpected name, got: %s, want: Test Segment", input.Name)
				}

				if input.Status.Code != "ACTIVE" {
					t.Errorf("unexpected status, got: %s, want: ACTIVE", input.Status.Code)
				}

				return &models.Segment{
					ID:             "segment-123",
					Name:           input.Name,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewSegment(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithPortfolio("portfolio-123").
			WithName("Test Segment")

		segment, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if segment.ID != "segment-123" {
			t.Errorf("unexpected ID, got: %s, want: segment-123", segment.ID)
		}

		if segment.Name != "Test Segment" {
			t.Errorf("unexpected name, got: %s, want: Test Segment", segment.Name)
		}

		if segment.OrganizationID != "org-123" {
			t.Errorf("unexpected organization ID, got: %s, want: org-123", segment.OrganizationID)
		}

		if segment.LedgerID != "ledger-123" {
			t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", segment.LedgerID)
		}
	})

	t.Run("Create with custom status", func(t *testing.T) {
		client := &mockSegmentClient{
			createSegmentFunc: func(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error) {
				// Verify status field
				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Segment{
					ID:             "segment-123",
					Name:           input.Name,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewSegment(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithPortfolio("portfolio-123").
			WithName("Test Segment").
			WithStatus("INACTIVE")

		segment, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if segment.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", segment.Status.Code)
		}
	})

	t.Run("Create with metadata and tags", func(t *testing.T) {
		client := &mockSegmentClient{
			createSegmentFunc: func(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Segment{
					ID:             "segment-123",
					Name:           input.Name,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewSegment(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithPortfolio("portfolio-123").
			WithName("Test Segment").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		segment, err := builder.Create(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if segment.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", segment.Metadata["key1"])
		}

		if segment.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", segment.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockSegmentClient{}

		// Missing organization ID
		builder1 := NewSegment(client).
			WithLedger("ledger-123").
			WithPortfolio("portfolio-123").
			WithName("Test Segment")

		_, err := builder1.Create(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewSegment(client).
			WithOrganization("org-123").
			WithPortfolio("portfolio-123").
			WithName("Test Segment")

		_, err = builder2.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing portfolio ID
		builder3 := NewSegment(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithName("Test Segment")

		_, err = builder3.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing portfolio ID, got nil")
		}

		if !strings.Contains(err.Error(), "portfolio ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: portfolio ID is required", err.Error())
		}

		// Missing name
		builder4 := NewSegment(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithPortfolio("portfolio-123")

		_, err = builder4.Create(context.Background())
		if err == nil {
			t.Fatal("expected error for missing name, got nil")
		}

		if !strings.Contains(err.Error(), "segment name is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: segment name is required", err.Error())
		}
	})
}

// \1 performs an operation
func TestSegmentUpdateBuilder(t *testing.T) {
	t.Run("Update segment", func(t *testing.T) {
		client := &mockSegmentClient{
			updateSegmentFunc: func(ctx context.Context, organizationID, ledgerID, portfolioID, segmentID string, input *models.UpdateSegmentInput) (*models.Segment, error) {
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

				if segmentID != "segment-123" {
					t.Errorf("unexpected segment ID, got: %s, want: segment-123", segmentID)
				}

				// Verify update fields
				if input.Name != "Updated Segment" {
					t.Errorf("unexpected name, got: %s, want: Updated Segment", input.Name)
				}

				if input.Status.Code != "INACTIVE" {
					t.Errorf("unexpected status, got: %s, want: INACTIVE", input.Status.Code)
				}

				return &models.Segment{
					ID:             segmentID,
					Name:           input.Name,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status:         input.Status,
				}, nil
			},
		}

		builder := NewSegmentUpdate(client, "org-123", "ledger-123", "portfolio-123", "segment-123").
			WithName("Updated Segment").
			WithStatus("INACTIVE")

		segment, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if segment.ID != "segment-123" {
			t.Errorf("unexpected ID, got: %s, want: segment-123", segment.ID)
		}

		if segment.Name != "Updated Segment" {
			t.Errorf("unexpected name, got: %s, want: Updated Segment", segment.Name)
		}

		if segment.Status.Code != "INACTIVE" {
			t.Errorf("unexpected status, got: %s, want: INACTIVE", segment.Status.Code)
		}
	})

	t.Run("Update with metadata", func(t *testing.T) {
		client := &mockSegmentClient{
			updateSegmentFunc: func(ctx context.Context, organizationID, ledgerID, portfolioID, segmentID string, input *models.UpdateSegmentInput) (*models.Segment, error) {
				// Verify metadata
				if input.Metadata["key1"] != "value1" {
					t.Errorf("unexpected metadata value, got: %v, want: value1", input.Metadata["key1"])
				}

				if input.Metadata["tags"] != "tag1,tag2" {
					t.Errorf("unexpected tags, got: %v, want: tag1,tag2", input.Metadata["tags"])
				}

				return &models.Segment{
					ID:             segmentID,
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Metadata:       input.Metadata,
				}, nil
			},
		}

		builder := NewSegmentUpdate(client, "org-123", "ledger-123", "portfolio-123", "segment-123").
			WithMetadata(map[string]any{"key1": "value1"}).
			WithTag("tag1").
			WithTag("tag2")

		segment, err := builder.Update(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if segment.Metadata["key1"] != "value1" {
			t.Errorf("unexpected metadata value, got: %v, want: value1", segment.Metadata["key1"])
		}

		if segment.Metadata["tags"] != "tag1,tag2" {
			t.Errorf("unexpected tags, got: %v, want: tag1,tag2", segment.Metadata["tags"])
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockSegmentClient{}

		// Missing organization ID
		builder1 := NewSegmentUpdate(client, "", "ledger-123", "portfolio-123", "segment-123").
			WithName("Updated Segment")

		_, err := builder1.Update(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewSegmentUpdate(client, "org-123", "", "portfolio-123", "segment-123").
			WithName("Updated Segment")

		_, err = builder2.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing portfolio ID
		builder3 := NewSegmentUpdate(client, "org-123", "ledger-123", "", "segment-123").
			WithName("Updated Segment")

		_, err = builder3.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing portfolio ID, got nil")
		}

		if !strings.Contains(err.Error(), "portfolio ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: portfolio ID is required", err.Error())
		}

		// Missing segment ID
		builder4 := NewSegmentUpdate(client, "org-123", "ledger-123", "portfolio-123", "").
			WithName("Updated Segment")

		_, err = builder4.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for missing segment ID, got nil")
		}

		if !strings.Contains(err.Error(), "segment ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: segment ID is required", err.Error())
		}

		// No fields to update
		builder5 := NewSegmentUpdate(client, "org-123", "ledger-123", "portfolio-123", "segment-123")

		_, err = builder5.Update(context.Background())
		if err == nil {
			t.Fatal("expected error for no fields to update, got nil")
		}

		if !strings.Contains(err.Error(), "no fields to update") {
			t.Errorf("unexpected error message, got: %s, want to contain: no fields to update", err.Error())
		}
	})
}
