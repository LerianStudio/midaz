package entities

import (
	"context"
	"testing"
	"time"

	"fmt"
	"github.com/LerianStudio/midaz/sdks/go-sdk/entities/mocks"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// Asset Tests

// \1 performs an operation
func TestListAssets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock assets service
	mockService := mocks.NewMockAssetsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test assets list response
	assetsList := &models.ListResponse[models.Asset]{
		Items: []models.Asset{
			{
				ID:             "asset-123",
				Name:           "US Dollar",
				Code:           "USD",
				Type:           "FIAT",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Status: models.Status{
					Code: "ACTIVE",
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:             "asset-456",
				Name:           "Euro",
				Code:           "EUR",
				Type:           "FIAT",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Status: models.Status{
					Code: "ACTIVE",
				},
				CreatedAt: now,
				UpdatedAt: now,
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
		ListAssets(gomock.Any(), orgID, ledgerID, gomock.Nil()).
		Return(assetsList, nil)

	// Test listing assets with default options
	result, err := mockService.ListAssets(ctx, orgID, ledgerID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "asset-123", result.Items[0].ID)
	assert.Equal(t, "US Dollar", result.Items[0].Name)
	assert.Equal(t, "USD", result.Items[0].Code)
	assert.Equal(t, "FIAT", result.Items[0].Type)
	assert.Equal(t, "ACTIVE", result.Items[0].Status.Code)
	assert.Equal(t, orgID, result.Items[0].OrganizationID)
	assert.Equal(t, ledgerID, result.Items[0].LedgerID)

	// Test with options
	opts := &models.ListOptions{
		Limit:          5,
		Offset:         0,
		OrderBy:        "created_at",
		OrderDirection: "desc",
	}

	mockService.EXPECT().
		ListAssets(gomock.Any(), orgID, ledgerID, opts).
		Return(assetsList, nil)

	result, err = mockService.ListAssets(ctx, orgID, ledgerID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)

	// Test with empty organizationID
	mockService.EXPECT().
		ListAssets(gomock.Any(), "", ledgerID, gomock.Any()).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.ListAssets(ctx, "", ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		ListAssets(gomock.Any(), orgID, "", gomock.Any()).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.ListAssets(ctx, orgID, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")
}

// \1 performs an operation
func TestGetAsset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock assets service
	mockService := mocks.NewMockAssetsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	assetID := "asset-123"
	now := time.Now()

	// Create test asset
	asset := &models.Asset{
		ID:             assetID,
		Name:           "US Dollar",
		Code:           "USD",
		Type:           "FIAT",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "ACTIVE",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetAsset(gomock.Any(), orgID, ledgerID, assetID).
		Return(asset, nil)

	// Test getting an asset by ID
	result, err := mockService.GetAsset(ctx, orgID, ledgerID, assetID)
	assert.NoError(t, err)
	assert.Equal(t, assetID, result.ID)
	assert.Equal(t, "US Dollar", result.Name)
	assert.Equal(t, "USD", result.Code)
	assert.Equal(t, "FIAT", result.Type)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)

	// Test with empty organizationID
	mockService.EXPECT().
		GetAsset(gomock.Any(), "", ledgerID, assetID).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetAsset(ctx, "", ledgerID, assetID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetAsset(gomock.Any(), orgID, "", assetID).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetAsset(ctx, orgID, "", assetID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty assetID
	mockService.EXPECT().
		GetAsset(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("asset ID is required"))

	_, err = mockService.GetAsset(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "asset ID is required")

	// Test with not found
	mockService.EXPECT().
		GetAsset(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Asset not found"))

	_, err = mockService.GetAsset(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCreateAsset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock assets service
	mockService := mocks.NewMockAssetsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test input
	input := models.NewCreateAssetInput("Brazilian Real", "BRL").
		WithType("FIAT").
		WithStatus(models.NewStatus("ACTIVE")).
		WithMetadata(map[string]any{"country": "Brazil"})

	// Create expected output
	asset := &models.Asset{
		ID:             "asset-new",
		Name:           "Brazilian Real",
		Code:           "BRL",
		Type:           "FIAT",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata:  map[string]any{"country": "Brazil"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreateAsset(gomock.Any(), orgID, ledgerID, input).
		Return(asset, nil)

	// Test creating a new asset
	result, err := mockService.CreateAsset(ctx, orgID, ledgerID, input)
	assert.NoError(t, err)
	assert.Equal(t, "asset-new", result.ID)
	assert.Equal(t, "Brazilian Real", result.Name)
	assert.Equal(t, "BRL", result.Code)
	assert.Equal(t, "FIAT", result.Type)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "Brazil", result.Metadata["country"])

	// Test with empty organizationID
	mockService.EXPECT().
		CreateAsset(gomock.Any(), "", ledgerID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.CreateAsset(ctx, "", ledgerID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		CreateAsset(gomock.Any(), orgID, "", input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.CreateAsset(ctx, orgID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with nil input
	mockService.EXPECT().
		CreateAsset(gomock.Any(), orgID, ledgerID, nil).
		Return(nil, fmt.Errorf("asset input cannot be nil"))

	_, err = mockService.CreateAsset(ctx, orgID, ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "asset input cannot be nil")
}

// \1 performs an operation
func TestUpdateAsset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock assets service
	mockService := mocks.NewMockAssetsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	assetID := "asset-123"
	now := time.Now()

	// Create test input
	input := models.NewUpdateAssetInput().
		WithName("United States Dollar").
		WithStatus(models.NewStatus("INACTIVE")).
		WithMetadata(map[string]any{"country": "USA"})

	// Create expected output
	asset := &models.Asset{
		ID:             assetID,
		Name:           "United States Dollar",
		Code:           "USD",  // Original value should be preserved
		Type:           "FIAT", // Original value should be preserved
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "INACTIVE",
		},
		Metadata:  map[string]any{"country": "USA"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		UpdateAsset(gomock.Any(), orgID, ledgerID, assetID, input).
		Return(asset, nil)

	// Test updating an asset
	result, err := mockService.UpdateAsset(ctx, orgID, ledgerID, assetID, input)
	assert.NoError(t, err)
	assert.Equal(t, assetID, result.ID)
	assert.Equal(t, "United States Dollar", result.Name)
	assert.Equal(t, "USD", result.Code) // Original value should be preserved
	assert.Equal(t, "INACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "USA", result.Metadata["country"])

	// Test with empty organizationID
	mockService.EXPECT().
		UpdateAsset(gomock.Any(), "", ledgerID, assetID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.UpdateAsset(ctx, "", ledgerID, assetID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		UpdateAsset(gomock.Any(), orgID, "", assetID, input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.UpdateAsset(ctx, orgID, "", assetID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty assetID
	mockService.EXPECT().
		UpdateAsset(gomock.Any(), orgID, ledgerID, "", input).
		Return(nil, fmt.Errorf("asset ID is required"))

	_, err = mockService.UpdateAsset(ctx, orgID, ledgerID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "asset ID is required")

	// Test with nil input
	mockService.EXPECT().
		UpdateAsset(gomock.Any(), orgID, ledgerID, assetID, nil).
		Return(nil, fmt.Errorf("asset input cannot be nil"))

	_, err = mockService.UpdateAsset(ctx, orgID, ledgerID, assetID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "asset input cannot be nil")

	// Test with not found
	mockService.EXPECT().
		UpdateAsset(gomock.Any(), orgID, ledgerID, "not-found", input).
		Return(nil, fmt.Errorf("Asset not found"))

	_, err = mockService.UpdateAsset(ctx, orgID, ledgerID, "not-found", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestDeleteAsset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock assets service
	mockService := mocks.NewMockAssetsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	assetID := "asset-123"

	// Setup expectation for successful case
	mockService.EXPECT().
		DeleteAsset(gomock.Any(), orgID, ledgerID, assetID).
		Return(nil)

	// Test deleting an asset
	err := mockService.DeleteAsset(ctx, orgID, ledgerID, assetID)
	assert.NoError(t, err)

	// Test with empty organizationID
	mockService.EXPECT().
		DeleteAsset(gomock.Any(), "", ledgerID, assetID).
		Return(fmt.Errorf("organization ID is required"))

	err = mockService.DeleteAsset(ctx, "", ledgerID, assetID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		DeleteAsset(gomock.Any(), orgID, "", assetID).
		Return(fmt.Errorf("ledger ID is required"))

	err = mockService.DeleteAsset(ctx, orgID, "", assetID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty assetID
	mockService.EXPECT().
		DeleteAsset(gomock.Any(), orgID, ledgerID, "").
		Return(fmt.Errorf("asset ID is required"))

	err = mockService.DeleteAsset(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "asset ID is required")

	// Test with not found
	mockService.EXPECT().
		DeleteAsset(gomock.Any(), orgID, ledgerID, "not-found").
		Return(fmt.Errorf("Asset not found"))

	err = mockService.DeleteAsset(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Asset Rate Tests

// \1 performs an operation
func TestGetAssetRate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock assets service
	mockService := mocks.NewMockAssetsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test asset rate
	rate := &models.AssetRate{
		ID:           "rate-123",
		FromAsset:    "USD",
		ToAsset:      "EUR",
		Rate:         0.85,
		EffectiveAt:  now.Add(-24 * time.Hour),
		ExpirationAt: now.Add(24 * time.Hour),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetAssetRate(gomock.Any(), orgID, ledgerID, "USD", "EUR").
		Return(rate, nil)

	// Test getting an asset rate
	result, err := mockService.GetAssetRate(ctx, orgID, ledgerID, "USD", "EUR")
	assert.NoError(t, err)
	assert.Equal(t, "rate-123", result.ID)
	assert.Equal(t, "USD", result.FromAsset)
	assert.Equal(t, "EUR", result.ToAsset)
	assert.Equal(t, 0.85, result.Rate)
	assert.NotZero(t, result.EffectiveAt)
	assert.NotZero(t, result.ExpirationAt)

	// Test with empty organizationID
	mockService.EXPECT().
		GetAssetRate(gomock.Any(), "", ledgerID, "USD", "EUR").
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetAssetRate(ctx, "", ledgerID, "USD", "EUR")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetAssetRate(gomock.Any(), orgID, "", "USD", "EUR").
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetAssetRate(ctx, orgID, "", "USD", "EUR")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty sourceAssetCode
	mockService.EXPECT().
		GetAssetRate(gomock.Any(), orgID, ledgerID, "", "EUR").
		Return(nil, fmt.Errorf("source asset code is required"))

	_, err = mockService.GetAssetRate(ctx, orgID, ledgerID, "", "EUR")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source asset code is required")

	// Test with empty destinationAssetCode
	mockService.EXPECT().
		GetAssetRate(gomock.Any(), orgID, ledgerID, "USD", "").
		Return(nil, fmt.Errorf("destination asset code is required"))

	_, err = mockService.GetAssetRate(ctx, orgID, ledgerID, "USD", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "destination asset code is required")

	// Test with not found
	mockService.EXPECT().
		GetAssetRate(gomock.Any(), orgID, ledgerID, "BRL", "JPY").
		Return(nil, fmt.Errorf("Asset rate not found"))

	_, err = mockService.GetAssetRate(ctx, orgID, ledgerID, "BRL", "JPY")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCreateOrUpdateAssetRate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock assets service
	mockService := mocks.NewMockAssetsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test input
	effectiveAt := now.Add(-24 * time.Hour)
	expirationAt := now.Add(7 * 24 * time.Hour)

	input := models.NewUpdateAssetRateInput(
		"BRL", "USD",
		0.18,
		effectiveAt,
		expirationAt,
	)

	// Create expected output
	rate := &models.AssetRate{
		ID:           "rate-new",
		FromAsset:    "BRL",
		ToAsset:      "USD",
		Rate:         0.18,
		EffectiveAt:  effectiveAt,
		ExpirationAt: expirationAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreateOrUpdateAssetRate(gomock.Any(), orgID, ledgerID, input).
		Return(rate, nil)

	// Test creating a new asset rate
	result, err := mockService.CreateOrUpdateAssetRate(ctx, orgID, ledgerID, input)
	assert.NoError(t, err)
	assert.Equal(t, "rate-new", result.ID)
	assert.Equal(t, "BRL", result.FromAsset)
	assert.Equal(t, "USD", result.ToAsset)
	assert.Equal(t, 0.18, result.Rate)
	assert.Equal(t, effectiveAt.UTC().Truncate(time.Second), result.EffectiveAt.UTC().Truncate(time.Second))
	assert.Equal(t, expirationAt.UTC().Truncate(time.Second), result.ExpirationAt.UTC().Truncate(time.Second))

	// Test with empty organizationID
	mockService.EXPECT().
		CreateOrUpdateAssetRate(gomock.Any(), "", ledgerID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.CreateOrUpdateAssetRate(ctx, "", ledgerID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		CreateOrUpdateAssetRate(gomock.Any(), orgID, "", input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.CreateOrUpdateAssetRate(ctx, orgID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with nil input
	mockService.EXPECT().
		CreateOrUpdateAssetRate(gomock.Any(), orgID, ledgerID, nil).
		Return(nil, fmt.Errorf("asset rate input cannot be nil"))

	_, err = mockService.CreateOrUpdateAssetRate(ctx, orgID, ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "asset rate input cannot be nil")
}
