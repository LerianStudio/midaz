package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
)

// TestGetAllTransactionRoutesSuccess tests successful retrieval of all transaction routes
func TestGetAllTransactionRoutesSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionRouteID1 := uuid.New()
	transactionRouteID2 := uuid.New()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		Metadata:  &bson.M{},
	}

	expectedTransactionRoutes := []*mmodel.TransactionRoute{
		{
			ID:             transactionRouteID1,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "route1",
			Description:    "Description 1",
		},
		{
			ID:             transactionRouteID2,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "route2",
			Description:    "Description 2",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: transactionRouteID1.String(),
			Data:     mongodb.JSON{"key1": "value1"},
		},
		{
			ID:       primitive.NewObjectID(),
			EntityID: transactionRouteID2.String(),
			Data:     mongodb.JSON{"key2": "value2"},
		},
	}

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedTransactionRoutes, expectedCursor, nil)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedMetadata, nil)

	result, cursor, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedCursor, cursor)
	assert.Len(t, result, 2)
	assert.Equal(t, expectedTransactionRoutes[0].ID, result[0].ID)
	assert.Equal(t, expectedTransactionRoutes[1].ID, result[1].ID)
	assert.Equal(t, map[string]any{"key1": "value1"}, result[0].Metadata)
	assert.Equal(t, map[string]any{"key2": "value2"}, result[1].Metadata)
}

// TestGetAllTransactionRoutesSuccessWithoutMetadata tests successful retrieval without metadata filter
func TestGetAllTransactionRoutesSuccessWithoutMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionRouteID1 := uuid.New()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	expectedTransactionRoutes := []*mmodel.TransactionRoute{
		{
			ID:             transactionRouteID1,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "route1",
			Description:    "Description 1",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: transactionRouteID1.String(),
			Data:     mongodb.JSON{"key1": "value1"},
		},
	}

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedTransactionRoutes, expectedCursor, nil)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedMetadata, nil)

	result, cursor, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedCursor, cursor)
	assert.Len(t, result, 1)
	assert.Equal(t, expectedTransactionRoutes[0].ID, result[0].ID)
	assert.Equal(t, map[string]any{"key1": "value1"}, result[0].Metadata)
}

// TestGetAllTransactionRoutesNotFound tests when no transaction routes are found
func TestGetAllTransactionRoutesNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound)

	result, cursor, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0106", entityNotFoundError.Code)
}

// TestGetAllTransactionRoutesRepositoryError tests repository error handling
func TestGetAllTransactionRoutesRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	expectedError := errors.New("database connection error")

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, expectedError)

	result, cursor, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)
	assert.Equal(t, expectedError, err)
}

// TestGetAllTransactionRoutesMetadataError tests metadata repository error handling
func TestGetAllTransactionRoutesMetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionRouteID1 := uuid.New()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	expectedTransactionRoutes := []*mmodel.TransactionRoute{
		{
			ID:             transactionRouteID1,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "route1",
			Description:    "Description 1",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	expectedMetadataError := errors.New("metadata repository error")

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedTransactionRoutes, expectedCursor, nil)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(nil, expectedMetadataError)

	result, cursor, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0007", entityNotFoundError.Code)
}

// TestGetAllTransactionRoutesNilTransactionRoutes tests when transaction routes are nil
func TestGetAllTransactionRoutesNilTransactionRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, expectedCursor, nil)

	result, cursor, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedCursor, cursor)
}
