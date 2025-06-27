package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllOperationRoutesSuccess tests getting all operation routes successfully
func TestGetAllOperationRoutesSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	pagination := libPostgres.Pagination{
		Page:      1,
		Limit:     10,
		SortOrder: "asc",
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Debit Route",
			Description:    "Test Debit Description",
			Type:           "debit",
		},
		{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Credit Route",
			Description:    "Test Credit Description",
			Type:           "credit",
		},
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(expectedOperationRoutes, nil).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Len(t, result, 2)
}

// TestGetAllOperationRoutesError tests getting all operation routes with database error
func TestGetAllOperationRoutesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	expectedError := errors.New("database connection error")

	pagination := libPostgres.Pagination{
		Page:      1,
		Limit:     10,
		SortOrder: "asc",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

// TestGetAllOperationRoutesEmpty tests getting all operation routes when no results found
func TestGetAllOperationRoutesEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	pagination := libPostgres.Pagination{
		Page:      1,
		Limit:     10,
		SortOrder: "asc",
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(expectedOperationRoutes, nil).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Len(t, result, 0)
}

// TestGetAllOperationRoutesWithDifferentPagination tests getting all operation routes with different pagination settings
func TestGetAllOperationRoutesWithDifferentPagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	pagination := libPostgres.Pagination{
		Page:      2,
		Limit:     5,
		SortOrder: "desc",
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Operation Route 1",
			Description:    "Description 1",
			Type:           "debit",
		},
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(expectedOperationRoutes, nil).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Len(t, result, 1)
}

// TestGetAllOperationRoutesWithDateRange tests getting all operation routes with date range pagination
func TestGetAllOperationRoutesWithDateRange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	startDate, _ := time.Parse("2006-01-02", "2024-01-01")
	endDate, _ := time.Parse("2006-01-02", "2024-12-31")

	pagination := libPostgres.Pagination{
		Page:      1,
		Limit:     10,
		SortOrder: "asc",
		StartDate: startDate,
		EndDate:   endDate,
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Filtered Route",
			Description:    "Filtered Description",
			Type:           "credit",
		},
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(expectedOperationRoutes, nil).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Len(t, result, 1)
}
