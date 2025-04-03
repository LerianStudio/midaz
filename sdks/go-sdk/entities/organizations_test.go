package entities

import (
	"context"
	"testing"

	"fmt"
	"github.com/LerianStudio/midaz/sdks/go-sdk/entities/mocks"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func TestListOrganizations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock organizations service
	mockService := mocks.NewMockOrganizationsService(ctrl)

	// Test data
	ctx := context.Background()

	// Create test organization list response
	orgsList := &models.ListResponse[models.Organization]{
		Items: []models.Organization{
			{
				ID:        "org-123",
				LegalName: "Test Org 1",
				Status: models.Status{
					Code: "ACTIVE",
				},
			},
			{
				ID:        "org-456",
				LegalName: "Test Org 2",
				Status: models.Status{
					Code: "ACTIVE",
				},
			},
		},
		Pagination: models.Pagination{
			Total:  2,
			Limit:  10,
			Offset: 0,
		},
	}

	// Setup expectations for default options
	mockService.EXPECT().
		ListOrganizations(gomock.Any(), gomock.Nil()).
		Return(orgsList, nil)

	// Test listing organizations with default options
	result, err := mockService.ListOrganizations(ctx, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "org-123", result.Items[0].ID)
	assert.Equal(t, "Test Org 1", result.Items[0].LegalName)
	assert.Equal(t, "org-456", result.Items[1].ID)
	assert.Equal(t, "Test Org 2", result.Items[1].LegalName)

	// Test with options
	opts := &models.ListOptions{
		Page:           2,
		Limit:          5,
		OrderBy:        "created_at",
		OrderDirection: "desc",
	}

	mockService.EXPECT().
		ListOrganizations(gomock.Any(), opts).
		Return(orgsList, nil)

	_, err = mockService.ListOrganizations(ctx, opts)
	assert.NoError(t, err)
}

// \1 performs an operation
func TestGetOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock organizations service
	mockService := mocks.NewMockOrganizationsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"

	// Create test organization
	org := &models.Organization{
		ID:            orgID,
		LegalName:     "Test Org 1",
		LegalDocument: "123456789",
		Status: models.Status{
			Code: "ACTIVE",
		},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetOrganization(gomock.Any(), orgID).
		Return(org, nil)

	// Test getting an organization
	result, err := mockService.GetOrganization(ctx, orgID)
	assert.NoError(t, err)
	assert.Equal(t, orgID, result.ID)
	assert.Equal(t, "Test Org 1", result.LegalName)
	assert.Equal(t, "123456789", result.LegalDocument)
	assert.Equal(t, "ACTIVE", result.Status.Code)

	// Test with empty ID
	mockService.EXPECT().
		GetOrganization(gomock.Any(), "").
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetOrganization(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with not found
	mockService.EXPECT().
		GetOrganization(gomock.Any(), "not-found").
		Return(nil, fmt.Errorf("Organization not found"))

	_, err = mockService.GetOrganization(ctx, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCreateOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock organizations service
	mockService := mocks.NewMockOrganizationsService(ctrl)

	// Test data
	ctx := context.Background()

	// Create test input
	input := &models.CreateOrganizationInput{
		LegalName:     "New Org",
		LegalDocument: "987654321",
		Status:        models.NewStatus("ACTIVE"),
		Metadata: map[string]any{
			"key": "value",
		},
	}

	// Create expected output
	org := &models.Organization{
		ID:            "org-new",
		LegalName:     "New Org",
		LegalDocument: "987654321",
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata: map[string]any{
			"key": "value",
		},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreateOrganization(gomock.Any(), input).
		Return(org, nil)

	// Test creating a new organization
	result, err := mockService.CreateOrganization(ctx, input)
	assert.NoError(t, err)
	assert.Equal(t, "org-new", result.ID)
	assert.Equal(t, "New Org", result.LegalName)
	assert.Equal(t, "987654321", result.LegalDocument)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, "value", result.Metadata["key"])

	// Test with nil input
	mockService.EXPECT().
		CreateOrganization(gomock.Any(), nil).
		Return(nil, fmt.Errorf("organization input cannot be nil"))

	_, err = mockService.CreateOrganization(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization input cannot be nil")
}

// \1 performs an operation
func TestUpdateOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock organizations service
	mockService := mocks.NewMockOrganizationsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"

	// Create test input
	input := &models.UpdateOrganizationInput{
		LegalName: "Updated Org",
		Status:    models.NewStatus("INACTIVE"),
		Metadata: map[string]any{
			"key": "updated",
		},
	}

	// Create expected output
	org := &models.Organization{
		ID:            orgID,
		LegalName:     "Updated Org",
		LegalDocument: "123456789", // Original value
		Status: models.Status{
			Code: "INACTIVE",
		},
		Metadata: map[string]any{
			"key": "updated",
		},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		UpdateOrganization(gomock.Any(), orgID, input).
		Return(org, nil)

	// Test updating an organization
	result, err := mockService.UpdateOrganization(ctx, orgID, input)
	assert.NoError(t, err)
	assert.Equal(t, orgID, result.ID)
	assert.Equal(t, "Updated Org", result.LegalName)
	assert.Equal(t, "INACTIVE", result.Status.Code)
	assert.Equal(t, "updated", result.Metadata["key"])

	// Test with empty ID
	mockService.EXPECT().
		UpdateOrganization(gomock.Any(), "", input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.UpdateOrganization(ctx, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with nil input
	mockService.EXPECT().
		UpdateOrganization(gomock.Any(), orgID, nil).
		Return(nil, fmt.Errorf("organization input cannot be nil"))

	_, err = mockService.UpdateOrganization(ctx, orgID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization input cannot be nil")
}

// \1 performs an operation
func TestDeleteOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock organizations service
	mockService := mocks.NewMockOrganizationsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"

	// Setup expectation for successful case
	mockService.EXPECT().
		DeleteOrganization(gomock.Any(), orgID).
		Return(nil)

	// Test deleting an organization
	err := mockService.DeleteOrganization(ctx, orgID)
	assert.NoError(t, err)

	// Test with empty ID
	mockService.EXPECT().
		DeleteOrganization(gomock.Any(), "").
		Return(fmt.Errorf("organization ID is required"))

	err = mockService.DeleteOrganization(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")
}
