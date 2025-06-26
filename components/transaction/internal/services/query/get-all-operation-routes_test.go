package query

import (
	"context"
	"errors"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllOperationRoutesSuccess tests successful retrieval of all operation routes
func TestGetAllOperationRoutesSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	pagination := libPostgres.Pagination{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	expectedRoutes := []*mmodel.OperationRoute{
		{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Route 1",
			Description:    "Description 1",
			Type:           "debit",
		},
		{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Route 2",
			Description:    "Description 2",
			Type:           "credit",
		},
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(expectedRoutes, nil).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.NoError(t, err)
	assert.Equal(t, expectedRoutes, result)
	assert.Len(t, result, 2)
}

// TestGetAllOperationRoutesNotFound tests retrieval when no operation routes are found
func TestGetAllOperationRoutesNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	pagination := libPostgres.Pagination{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.Error(t, err)
	assert.Nil(t, result)

	// Check if it's the proper business error
	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0102", entityNotFoundError.Code)
}

// TestGetAllOperationRoutesError tests retrieval with database error
func TestGetAllOperationRoutesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	pagination := libPostgres.Pagination{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}
	databaseError := errors.New("database connection error")

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(nil, databaseError).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
	assert.Nil(t, result)
}

// TestGetAllOperationRoutesEmptyResult tests successful retrieval with empty result
func TestGetAllOperationRoutesEmptyResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	pagination := libPostgres.Pagination{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	emptyRoutes := []*mmodel.OperationRoute{}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, pagination).
		Return(emptyRoutes, nil).
		Times(1)

	result, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, pagination)

	assert.NoError(t, err)
	assert.Equal(t, emptyRoutes, result)
	assert.Len(t, result, 0)
}
