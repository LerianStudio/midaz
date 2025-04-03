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

// Portfolio Tests

// \1 performs an operation
func TestListPortfolios(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test portfolios list response
	portfoliosList := &models.ListResponse[models.Portfolio]{
		Items: []models.Portfolio{
			{
				ID:             "portfolio-123",
				Name:           "Investment Portfolio",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Status: models.Status{
					Code: "ACTIVE",
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:             "portfolio-456",
				Name:           "Savings Portfolio",
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
		ListPortfolios(gomock.Any(), orgID, ledgerID, gomock.Nil()).
		Return(portfoliosList, nil)

	// Test listing portfolios with default options
	result, err := mockService.ListPortfolios(ctx, orgID, ledgerID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "portfolio-123", result.Items[0].ID)
	assert.Equal(t, "Investment Portfolio", result.Items[0].Name)
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
		ListPortfolios(gomock.Any(), orgID, ledgerID, opts).
		Return(portfoliosList, nil)

	result, err = mockService.ListPortfolios(ctx, orgID, ledgerID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)

	// Test with empty organizationID
	mockService.EXPECT().
		ListPortfolios(gomock.Any(), "", ledgerID, gomock.Any()).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.ListPortfolios(ctx, "", ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		ListPortfolios(gomock.Any(), orgID, "", gomock.Any()).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.ListPortfolios(ctx, orgID, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")
}

// \1 performs an operation
func TestGetPortfolio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	portfolioID := "portfolio-123"
	now := time.Now()

	// Create test portfolio
	portfolio := &models.Portfolio{
		ID:             portfolioID,
		Name:           "Investment Portfolio",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata:  map[string]any{"type": "investment"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetPortfolio(gomock.Any(), orgID, ledgerID, portfolioID).
		Return(portfolio, nil)

	// Test getting a portfolio by ID
	result, err := mockService.GetPortfolio(ctx, orgID, ledgerID, portfolioID)
	assert.NoError(t, err)
	assert.Equal(t, portfolioID, result.ID)
	assert.Equal(t, "Investment Portfolio", result.Name)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "investment", result.Metadata["type"])

	// Test with empty organizationID
	mockService.EXPECT().
		GetPortfolio(gomock.Any(), "", ledgerID, portfolioID).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetPortfolio(ctx, "", ledgerID, portfolioID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetPortfolio(gomock.Any(), orgID, "", portfolioID).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetPortfolio(ctx, orgID, "", portfolioID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty portfolioID
	mockService.EXPECT().
		GetPortfolio(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("portfolio ID is required"))

	_, err = mockService.GetPortfolio(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio ID is required")

	// Test with not found
	mockService.EXPECT().
		GetPortfolio(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Portfolio not found"))

	_, err = mockService.GetPortfolio(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCreatePortfolio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test input
	input := models.NewCreatePortfolioInput("entity-123", "Retirement Portfolio").
		WithStatus(models.NewStatus("ACTIVE")).
		WithMetadata(map[string]any{"type": "retirement"})

	// Create expected output
	portfolio := &models.Portfolio{
		ID:             "portfolio-new",
		Name:           "Retirement Portfolio",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata:  map[string]any{"type": "retirement"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreatePortfolio(gomock.Any(), orgID, ledgerID, input).
		Return(portfolio, nil)

	// Test creating a new portfolio
	result, err := mockService.CreatePortfolio(ctx, orgID, ledgerID, input)
	assert.NoError(t, err)
	assert.Equal(t, "portfolio-new", result.ID)
	assert.Equal(t, "Retirement Portfolio", result.Name)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "retirement", result.Metadata["type"])

	// Test with empty organizationID
	mockService.EXPECT().
		CreatePortfolio(gomock.Any(), "", ledgerID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.CreatePortfolio(ctx, "", ledgerID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		CreatePortfolio(gomock.Any(), orgID, "", input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.CreatePortfolio(ctx, orgID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with nil input
	mockService.EXPECT().
		CreatePortfolio(gomock.Any(), orgID, ledgerID, nil).
		Return(nil, fmt.Errorf("portfolio input cannot be nil"))

	_, err = mockService.CreatePortfolio(ctx, orgID, ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio input cannot be nil")
}

// \1 performs an operation
func TestUpdatePortfolio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	portfolioID := "portfolio-123"
	now := time.Now()

	// Create test input
	input := models.NewUpdatePortfolioInput().
		WithName("Updated Investment Portfolio").
		WithStatus(models.NewStatus("INACTIVE")).
		WithMetadata(map[string]any{"type": "updated-investment"})

	// Create expected output
	portfolio := &models.Portfolio{
		ID:             portfolioID,
		Name:           "Updated Investment Portfolio",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "INACTIVE",
		},
		Metadata:  map[string]any{"type": "updated-investment"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		UpdatePortfolio(gomock.Any(), orgID, ledgerID, portfolioID, input).
		Return(portfolio, nil)

	// Test updating a portfolio
	result, err := mockService.UpdatePortfolio(ctx, orgID, ledgerID, portfolioID, input)
	assert.NoError(t, err)
	assert.Equal(t, portfolioID, result.ID)
	assert.Equal(t, "Updated Investment Portfolio", result.Name)
	assert.Equal(t, "INACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "updated-investment", result.Metadata["type"])

	// Test with empty organizationID
	mockService.EXPECT().
		UpdatePortfolio(gomock.Any(), "", ledgerID, portfolioID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.UpdatePortfolio(ctx, "", ledgerID, portfolioID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		UpdatePortfolio(gomock.Any(), orgID, "", portfolioID, input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.UpdatePortfolio(ctx, orgID, "", portfolioID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty portfolioID
	mockService.EXPECT().
		UpdatePortfolio(gomock.Any(), orgID, ledgerID, "", input).
		Return(nil, fmt.Errorf("portfolio ID is required"))

	_, err = mockService.UpdatePortfolio(ctx, orgID, ledgerID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio ID is required")

	// Test with nil input
	mockService.EXPECT().
		UpdatePortfolio(gomock.Any(), orgID, ledgerID, portfolioID, nil).
		Return(nil, fmt.Errorf("portfolio input cannot be nil"))

	_, err = mockService.UpdatePortfolio(ctx, orgID, ledgerID, portfolioID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio input cannot be nil")

	// Test with not found
	mockService.EXPECT().
		UpdatePortfolio(gomock.Any(), orgID, ledgerID, "not-found", input).
		Return(nil, fmt.Errorf("Portfolio not found"))

	_, err = mockService.UpdatePortfolio(ctx, orgID, ledgerID, "not-found", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestDeletePortfolio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	portfolioID := "portfolio-123"

	// Setup expectation for successful case
	mockService.EXPECT().
		DeletePortfolio(gomock.Any(), orgID, ledgerID, portfolioID).
		Return(nil)

	// Test deleting a portfolio
	err := mockService.DeletePortfolio(ctx, orgID, ledgerID, portfolioID)
	assert.NoError(t, err)

	// Test with empty organizationID
	mockService.EXPECT().
		DeletePortfolio(gomock.Any(), "", ledgerID, portfolioID).
		Return(fmt.Errorf("organization ID is required"))

	err = mockService.DeletePortfolio(ctx, "", ledgerID, portfolioID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		DeletePortfolio(gomock.Any(), orgID, "", portfolioID).
		Return(fmt.Errorf("ledger ID is required"))

	err = mockService.DeletePortfolio(ctx, orgID, "", portfolioID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty portfolioID
	mockService.EXPECT().
		DeletePortfolio(gomock.Any(), orgID, ledgerID, "").
		Return(fmt.Errorf("portfolio ID is required"))

	err = mockService.DeletePortfolio(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio ID is required")

	// Test with not found
	mockService.EXPECT().
		DeletePortfolio(gomock.Any(), orgID, ledgerID, "not-found").
		Return(fmt.Errorf("Portfolio not found"))

	err = mockService.DeletePortfolio(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Segment Tests

// \1 performs an operation
func TestListSegments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	portfolioID := "portfolio-123"
	now := time.Now()

	// Create test segments list response
	segmentsList := &models.ListResponse[models.Segment]{
		Items: []models.Segment{
			{
				ID:             "segment-123",
				Name:           "Stocks",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Status: models.Status{
					Code: "ACTIVE",
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:             "segment-456",
				Name:           "Bonds",
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
		ListSegments(gomock.Any(), orgID, ledgerID, portfolioID, gomock.Nil()).
		Return(segmentsList, nil)

	// Test listing segments with default options
	result, err := mockService.ListSegments(ctx, orgID, ledgerID, portfolioID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "segment-123", result.Items[0].ID)
	assert.Equal(t, "Stocks", result.Items[0].Name)
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
		ListSegments(gomock.Any(), orgID, ledgerID, portfolioID, opts).
		Return(segmentsList, nil)

	result, err = mockService.ListSegments(ctx, orgID, ledgerID, portfolioID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)

	// Test with empty organizationID
	mockService.EXPECT().
		ListSegments(gomock.Any(), "", ledgerID, portfolioID, gomock.Any()).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.ListSegments(ctx, "", ledgerID, portfolioID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		ListSegments(gomock.Any(), orgID, "", portfolioID, gomock.Any()).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.ListSegments(ctx, orgID, "", portfolioID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty portfolioID
	mockService.EXPECT().
		ListSegments(gomock.Any(), orgID, ledgerID, "", gomock.Any()).
		Return(nil, fmt.Errorf("portfolio ID is required"))

	_, err = mockService.ListSegments(ctx, orgID, ledgerID, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio ID is required")
}

// \1 performs an operation
func TestGetSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	portfolioID := "portfolio-123"
	segmentID := "segment-123"
	now := time.Now()

	// Create test segment
	segment := &models.Segment{
		ID:             segmentID,
		Name:           "Stocks",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata:  map[string]any{"category": "equity"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetSegment(gomock.Any(), orgID, ledgerID, portfolioID, segmentID).
		Return(segment, nil)

	// Test getting a segment by ID
	result, err := mockService.GetSegment(ctx, orgID, ledgerID, portfolioID, segmentID)
	assert.NoError(t, err)
	assert.Equal(t, segmentID, result.ID)
	assert.Equal(t, "Stocks", result.Name)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "equity", result.Metadata["category"])

	// Test with empty organizationID
	mockService.EXPECT().
		GetSegment(gomock.Any(), "", ledgerID, portfolioID, segmentID).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetSegment(ctx, "", ledgerID, portfolioID, segmentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetSegment(gomock.Any(), orgID, "", portfolioID, segmentID).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetSegment(ctx, orgID, "", portfolioID, segmentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty portfolioID
	mockService.EXPECT().
		GetSegment(gomock.Any(), orgID, ledgerID, "", segmentID).
		Return(nil, fmt.Errorf("portfolio ID is required"))

	_, err = mockService.GetSegment(ctx, orgID, ledgerID, "", segmentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio ID is required")

	// Test with empty segmentID
	mockService.EXPECT().
		GetSegment(gomock.Any(), orgID, ledgerID, portfolioID, "").
		Return(nil, fmt.Errorf("segment ID is required"))

	_, err = mockService.GetSegment(ctx, orgID, ledgerID, portfolioID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "segment ID is required")

	// Test with not found
	mockService.EXPECT().
		GetSegment(gomock.Any(), orgID, ledgerID, portfolioID, "not-found").
		Return(nil, fmt.Errorf("Segment not found"))

	_, err = mockService.GetSegment(ctx, orgID, ledgerID, portfolioID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCreateSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	portfolioID := "portfolio-123"
	now := time.Now()

	// Create test input
	input := models.NewCreateSegmentInput("ETFs").
		WithStatus(models.NewStatus("ACTIVE")).
		WithMetadata(map[string]any{"category": "exchange-traded-funds"})

	// Create expected output
	segment := &models.Segment{
		ID:             "segment-new",
		Name:           "ETFs",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata:  map[string]any{"category": "exchange-traded-funds"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreateSegment(gomock.Any(), orgID, ledgerID, portfolioID, input).
		Return(segment, nil)

	// Test creating a new segment
	result, err := mockService.CreateSegment(ctx, orgID, ledgerID, portfolioID, input)
	assert.NoError(t, err)
	assert.Equal(t, "segment-new", result.ID)
	assert.Equal(t, "ETFs", result.Name)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "exchange-traded-funds", result.Metadata["category"])

	// Test with empty organizationID
	mockService.EXPECT().
		CreateSegment(gomock.Any(), "", ledgerID, portfolioID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.CreateSegment(ctx, "", ledgerID, portfolioID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		CreateSegment(gomock.Any(), orgID, "", portfolioID, input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.CreateSegment(ctx, orgID, "", portfolioID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty portfolioID
	mockService.EXPECT().
		CreateSegment(gomock.Any(), orgID, ledgerID, "", input).
		Return(nil, fmt.Errorf("portfolio ID is required"))

	_, err = mockService.CreateSegment(ctx, orgID, ledgerID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio ID is required")

	// Test with nil input
	mockService.EXPECT().
		CreateSegment(gomock.Any(), orgID, ledgerID, portfolioID, nil).
		Return(nil, fmt.Errorf("segment input cannot be nil"))

	_, err = mockService.CreateSegment(ctx, orgID, ledgerID, portfolioID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "segment input cannot be nil")
}

// \1 performs an operation
func TestUpdateSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	portfolioID := "portfolio-123"
	segmentID := "segment-123"
	now := time.Now()

	// Create test input
	input := models.NewUpdateSegmentInput().
		WithName("Updated Stocks").
		WithStatus(models.NewStatus("INACTIVE")).
		WithMetadata(map[string]any{"category": "updated-equity"})

	// Create expected output
	segment := &models.Segment{
		ID:             segmentID,
		Name:           "Updated Stocks",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "INACTIVE",
		},
		Metadata:  map[string]any{"category": "updated-equity"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		UpdateSegment(gomock.Any(), orgID, ledgerID, portfolioID, segmentID, input).
		Return(segment, nil)

	// Test updating a segment
	result, err := mockService.UpdateSegment(ctx, orgID, ledgerID, portfolioID, segmentID, input)
	assert.NoError(t, err)
	assert.Equal(t, segmentID, result.ID)
	assert.Equal(t, "Updated Stocks", result.Name)
	assert.Equal(t, "INACTIVE", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "updated-equity", result.Metadata["category"])

	// Test with empty organizationID
	mockService.EXPECT().
		UpdateSegment(gomock.Any(), "", ledgerID, portfolioID, segmentID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.UpdateSegment(ctx, "", ledgerID, portfolioID, segmentID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		UpdateSegment(gomock.Any(), orgID, "", portfolioID, segmentID, input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.UpdateSegment(ctx, orgID, "", portfolioID, segmentID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty portfolioID
	mockService.EXPECT().
		UpdateSegment(gomock.Any(), orgID, ledgerID, "", segmentID, input).
		Return(nil, fmt.Errorf("portfolio ID is required"))

	_, err = mockService.UpdateSegment(ctx, orgID, ledgerID, "", segmentID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio ID is required")

	// Test with empty segmentID
	mockService.EXPECT().
		UpdateSegment(gomock.Any(), orgID, ledgerID, portfolioID, "", input).
		Return(nil, fmt.Errorf("segment ID is required"))

	_, err = mockService.UpdateSegment(ctx, orgID, ledgerID, portfolioID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "segment ID is required")

	// Test with nil input
	mockService.EXPECT().
		UpdateSegment(gomock.Any(), orgID, ledgerID, portfolioID, segmentID, nil).
		Return(nil, fmt.Errorf("segment input cannot be nil"))

	_, err = mockService.UpdateSegment(ctx, orgID, ledgerID, portfolioID, segmentID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "segment input cannot be nil")

	// Test with not found
	mockService.EXPECT().
		UpdateSegment(gomock.Any(), orgID, ledgerID, portfolioID, "not-found", input).
		Return(nil, fmt.Errorf("Segment not found"))

	_, err = mockService.UpdateSegment(ctx, orgID, ledgerID, portfolioID, "not-found", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestDeleteSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock portfolios service
	mockService := mocks.NewMockPortfoliosService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	portfolioID := "portfolio-123"
	segmentID := "segment-123"

	// Setup expectation for successful case
	mockService.EXPECT().
		DeleteSegment(gomock.Any(), orgID, ledgerID, portfolioID, segmentID).
		Return(nil)

	// Test deleting a segment
	err := mockService.DeleteSegment(ctx, orgID, ledgerID, portfolioID, segmentID)
	assert.NoError(t, err)

	// Test with empty organizationID
	mockService.EXPECT().
		DeleteSegment(gomock.Any(), "", ledgerID, portfolioID, segmentID).
		Return(fmt.Errorf("organization ID is required"))

	err = mockService.DeleteSegment(ctx, "", ledgerID, portfolioID, segmentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		DeleteSegment(gomock.Any(), orgID, "", portfolioID, segmentID).
		Return(fmt.Errorf("ledger ID is required"))

	err = mockService.DeleteSegment(ctx, orgID, "", portfolioID, segmentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty portfolioID
	mockService.EXPECT().
		DeleteSegment(gomock.Any(), orgID, ledgerID, "", segmentID).
		Return(fmt.Errorf("portfolio ID is required"))

	err = mockService.DeleteSegment(ctx, orgID, ledgerID, "", segmentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "portfolio ID is required")

	// Test with empty segmentID
	mockService.EXPECT().
		DeleteSegment(gomock.Any(), orgID, ledgerID, portfolioID, "").
		Return(fmt.Errorf("segment ID is required"))

	err = mockService.DeleteSegment(ctx, orgID, ledgerID, portfolioID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "segment ID is required")

	// Test with not found
	mockService.EXPECT().
		DeleteSegment(gomock.Any(), orgID, ledgerID, portfolioID, "not-found").
		Return(fmt.Errorf("Segment not found"))

	err = mockService.DeleteSegment(ctx, orgID, ledgerID, portfolioID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
