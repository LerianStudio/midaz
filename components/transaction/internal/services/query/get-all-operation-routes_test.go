package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
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

	filter := http.QueryHeader{
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

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 2)
}

// TestGetAllOperationRoutesError tests getting all operation routes with database error
func TestGetAllOperationRoutesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	expectedError := errors.New("database connection error")

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(nil, libHTTP.CursorPagination{}, expectedError).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cur)
}

// TestGetAllOperationRoutesEmpty tests getting all operation routes when no results found
func TestGetAllOperationRoutesEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{}
	expectedCursor := libHTTP.CursorPagination{}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 0)
}

// TestGetAllOperationRoutesWithDifferentPagination tests getting all operation routes with different pagination settings
func TestGetAllOperationRoutesWithDifferentPagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{
		Limit:     5,
		SortOrder: "desc",
		Cursor:    "test_cursor",
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

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Equal(t, expectedCursor, cur)
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

	filter := http.QueryHeader{
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

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 1)
}
