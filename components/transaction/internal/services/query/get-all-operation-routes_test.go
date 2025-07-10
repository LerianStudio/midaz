package query

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	// Mock the OperationRouteRepo.FindAll call
	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: expectedOperationRoutes[0].ID.String(),
			Data:     map[string]any{"key1": "value1"},
		},
		{
			ID:       primitive.NewObjectID(),
			EntityID: expectedOperationRoutes[1].ID.String(),
			Data:     map[string]any{"key2": "value2"},
		},
	}

	metadataFilter := filter
	if metadataFilter.Metadata == nil {
		metadataFilter.Metadata = &bson.M{}
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), metadataFilter).
		Return(expectedMetadata, nil).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 2)

	assert.Equal(t, map[string]any{"key1": "value1"}, result[0].Metadata)
	assert.Equal(t, map[string]any{"key2": "value2"}, result[1].Metadata)
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
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
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

// TestGetAllOperationRoutesNotFound tests getting all operation routes when no results found
func TestGetAllOperationRoutesNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cur)
	assert.Contains(t, err.Error(), "No operation routes were found in the search")
}

// TestGetAllOperationRoutesEmpty tests getting all operation routes when empty results returned
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
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	metadataFilter := filter
	if metadataFilter.Metadata == nil {
		metadataFilter.Metadata = &bson.M{}
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), metadataFilter).
		Return([]*mongodb.Metadata{}, nil).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 0)
}

// TestGetAllOperationRoutesMetadataError tests getting all operation routes with metadata error
func TestGetAllOperationRoutesMetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	metadataError := errors.New("metadata repository error")

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Test Route",
			Description:    "Test Description",
			Type:           "debit",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	metadataFilter := filter
	if metadataFilter.Metadata == nil {
		metadataFilter.Metadata = &bson.M{}
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), metadataFilter).
		Return(nil, metadataError).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cur)
	assert.Contains(t, err.Error(), "No entity was found for the given ID")
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
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	metadataFilter := filter
	if metadataFilter.Metadata == nil {
		metadataFilter.Metadata = &bson.M{}
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), metadataFilter).
		Return([]*mongodb.Metadata{}, nil).
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
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	metadataFilter := filter
	if metadataFilter.Metadata == nil {
		metadataFilter.Metadata = &bson.M{}
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), metadataFilter).
		Return([]*mongodb.Metadata{}, nil).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoutes, result)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 1)
}

// TestGetAllOperationRoutesWithMetadataFilter tests getting all operation routes with metadata filtering
func TestGetAllOperationRoutesWithMetadataFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
		Metadata:  &bson.M{"category": "payment"},
	}

	operationRouteID := uuid.New()
	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:             operationRouteID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Payment Route",
			Description:    "Payment Description",
			Type:           "debit",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedOperationRoutes, expectedCursor, nil).
		Times(1)

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: operationRouteID.String(),
			Data:     map[string]any{"category": "payment", "priority": "high"},
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), filter).
		Return(expectedMetadata, nil).
		Times(1)

	result, cur, err := uc.GetAllOperationRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 1)

	assert.Equal(t, map[string]any{"category": "payment", "priority": "high"}, result[0].Metadata)
}
