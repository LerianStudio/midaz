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
func TestListLedgers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock ledgers service
	mockService := mocks.NewMockLedgersService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"

	// Create test ledger list response
	ledgersList := &models.ListResponse[models.Ledger]{
		Items: []models.Ledger{
			{
				ID:             "ledger-123",
				Name:           "Test Ledger 1",
				OrganizationID: orgID,
				Status: models.Status{
					Code: "ACTIVE",
				},
			},
			{
				ID:             "ledger-456",
				Name:           "Test Ledger 2",
				OrganizationID: orgID,
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
		ListLedgers(gomock.Any(), orgID, gomock.Nil()).
		Return(ledgersList, nil)

	// Test listing ledgers with default options
	result, err := mockService.ListLedgers(ctx, orgID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "ledger-123", result.Items[0].ID)
	assert.Equal(t, "Test Ledger 1", result.Items[0].Name)
	assert.Equal(t, "ACTIVE", result.Items[0].Status.Code)
	assert.Equal(t, orgID, result.Items[0].OrganizationID)

	// Test with options
	opts := &models.ListOptions{
		Limit:          5,
		Offset:         0,
		OrderBy:        "created_at",
		OrderDirection: "desc",
	}

	mockService.EXPECT().
		ListLedgers(gomock.Any(), orgID, opts).
		Return(ledgersList, nil)

	result, err = mockService.ListLedgers(ctx, orgID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)

	// Test with empty organizationID
	mockService.EXPECT().
		ListLedgers(gomock.Any(), "", gomock.Any()).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.ListLedgers(ctx, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")
}

// \1 performs an operation
func TestGetLedger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock ledgers service
	mockService := mocks.NewMockLedgersService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"

	// Create test ledger
	ledger := &models.Ledger{
		ID:             ledgerID,
		Name:           "Test Ledger 1",
		OrganizationID: orgID,
		Status: models.Status{
			Code: "ACTIVE",
		},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetLedger(gomock.Any(), orgID, ledgerID).
		Return(ledger, nil)

	// Test getting a ledger by ID
	result, err := mockService.GetLedger(ctx, orgID, ledgerID)
	assert.NoError(t, err)
	assert.Equal(t, ledgerID, result.ID)
	assert.Equal(t, "Test Ledger 1", result.Name)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)

	// Test with empty organizationID
	mockService.EXPECT().
		GetLedger(gomock.Any(), "", ledgerID).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetLedger(ctx, "", ledgerID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetLedger(gomock.Any(), orgID, "").
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetLedger(ctx, orgID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with not found
	mockService.EXPECT().
		GetLedger(gomock.Any(), orgID, "not-found").
		Return(nil, fmt.Errorf("Ledger not found"))

	_, err = mockService.GetLedger(ctx, orgID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCreateLedger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock ledgers service
	mockService := mocks.NewMockLedgersService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"

	// Create test input
	input := models.NewCreateLedgerInput("New Ledger").
		WithStatus(models.NewStatus("ACTIVE")).
		WithMetadata(map[string]any{"key": "value"})

	// Create expected output
	ledger := &models.Ledger{
		ID:             "ledger-new",
		Name:           "New Ledger",
		OrganizationID: orgID,
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata: map[string]any{
			"key": "value",
		},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreateLedger(gomock.Any(), orgID, input).
		Return(ledger, nil)

	// Test creating a new ledger
	result, err := mockService.CreateLedger(ctx, orgID, input)
	assert.NoError(t, err)
	assert.Equal(t, "ledger-new", result.ID)
	assert.Equal(t, "New Ledger", result.Name)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, "value", result.Metadata["key"])

	// Test with empty organizationID
	mockService.EXPECT().
		CreateLedger(gomock.Any(), "", input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.CreateLedger(ctx, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with nil input
	mockService.EXPECT().
		CreateLedger(gomock.Any(), orgID, nil).
		Return(nil, fmt.Errorf("ledger input cannot be nil"))

	_, err = mockService.CreateLedger(ctx, orgID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger input cannot be nil")

	// Test with invalid input (empty name)
	invalidInput := models.NewCreateLedgerInput("").
		WithStatus(models.NewStatus("ACTIVE"))

	mockService.EXPECT().
		CreateLedger(gomock.Any(), orgID, invalidInput).
		Return(nil, fmt.Errorf("invalid ledger input: name cannot be empty"))

	_, err = mockService.CreateLedger(ctx, orgID, invalidInput)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name cannot be empty")

	// Test with invalid metadata
	invalidMetadataInput := models.NewCreateLedgerInput("Test Ledger").
		WithStatus(models.NewStatus("ACTIVE")).
		WithMetadata(map[string]any{
			"key": map[string]interface{}{
				"nested1": map[string]interface{}{
					"nested2": map[string]interface{}{
						"nested3": map[string]interface{}{
							"nested4": "too deep",
						},
					},
				},
			},
		})

	mockService.EXPECT().
		CreateLedger(gomock.Any(), orgID, invalidMetadataInput).
		Return(nil, fmt.Errorf("invalid ledger input: metadata exceeds maximum nesting depth"))

	_, err = mockService.CreateLedger(ctx, orgID, invalidMetadataInput)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metadata exceeds maximum nesting depth")
}

// \1 performs an operation
func TestUpdateLedger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock ledgers service
	mockService := mocks.NewMockLedgersService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"

	// Create test input
	input := models.NewUpdateLedgerInput().
		WithName("Updated Ledger").
		WithStatus(models.NewStatus("INACTIVE")).
		WithMetadata(map[string]any{"key": "updated"})

	// Create expected output
	ledger := &models.Ledger{
		ID:             ledgerID,
		Name:           "Updated Ledger",
		OrganizationID: orgID,
		Status: models.Status{
			Code: "INACTIVE",
		},
		Metadata: map[string]any{
			"key": "updated",
		},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		UpdateLedger(gomock.Any(), orgID, ledgerID, input).
		Return(ledger, nil)

	// Test updating a ledger
	result, err := mockService.UpdateLedger(ctx, orgID, ledgerID, input)
	assert.NoError(t, err)
	assert.Equal(t, ledgerID, result.ID)
	assert.Equal(t, "Updated Ledger", result.Name)
	assert.Equal(t, "INACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, "updated", result.Metadata["key"])

	// Test with empty organizationID
	mockService.EXPECT().
		UpdateLedger(gomock.Any(), "", ledgerID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.UpdateLedger(ctx, "", ledgerID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		UpdateLedger(gomock.Any(), orgID, "", input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.UpdateLedger(ctx, orgID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with nil input
	mockService.EXPECT().
		UpdateLedger(gomock.Any(), orgID, ledgerID, nil).
		Return(nil, fmt.Errorf("ledger input cannot be nil"))

	_, err = mockService.UpdateLedger(ctx, orgID, ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger input cannot be nil")

	// Test with invalid input (empty name when provided)
	invalidInput := models.NewUpdateLedgerInput().
		WithName("")

	mockService.EXPECT().
		UpdateLedger(gomock.Any(), orgID, ledgerID, invalidInput).
		Return(nil, fmt.Errorf("invalid ledger update input: name cannot be empty when provided"))

	_, err = mockService.UpdateLedger(ctx, orgID, ledgerID, invalidInput)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name cannot be empty when provided")

	// Test with invalid metadata
	invalidMetadataInput := models.NewUpdateLedgerInput().
		WithName("Updated Ledger").
		WithMetadata(map[string]any{
			"key": map[string]interface{}{
				"nested1": map[string]interface{}{
					"nested2": map[string]interface{}{
						"nested3": map[string]interface{}{
							"nested4": "too deep",
						},
					},
				},
			},
		})

	mockService.EXPECT().
		UpdateLedger(gomock.Any(), orgID, ledgerID, invalidMetadataInput).
		Return(nil, fmt.Errorf("invalid ledger update input: metadata exceeds maximum nesting depth"))

	_, err = mockService.UpdateLedger(ctx, orgID, ledgerID, invalidMetadataInput)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metadata exceeds maximum nesting depth")

	// Test with not found
	mockService.EXPECT().
		UpdateLedger(gomock.Any(), orgID, "not-found", input).
		Return(nil, fmt.Errorf("Ledger not found"))

	_, err = mockService.UpdateLedger(ctx, orgID, "not-found", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestDeleteLedger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock ledgers service
	mockService := mocks.NewMockLedgersService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"

	// Setup expectation for successful case
	mockService.EXPECT().
		DeleteLedger(gomock.Any(), orgID, ledgerID).
		Return(nil)

	// Test deleting a ledger
	err := mockService.DeleteLedger(ctx, orgID, ledgerID)
	assert.NoError(t, err)

	// Test with empty organizationID
	mockService.EXPECT().
		DeleteLedger(gomock.Any(), "", ledgerID).
		Return(fmt.Errorf("organization ID is required"))

	err = mockService.DeleteLedger(ctx, "", ledgerID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		DeleteLedger(gomock.Any(), orgID, "").
		Return(fmt.Errorf("ledger ID is required"))

	err = mockService.DeleteLedger(ctx, orgID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with not found
	mockService.EXPECT().
		DeleteLedger(gomock.Any(), orgID, "not-found").
		Return(fmt.Errorf("Ledger not found"))

	err = mockService.DeleteLedger(ctx, orgID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
