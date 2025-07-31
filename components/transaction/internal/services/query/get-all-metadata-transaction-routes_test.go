package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
)

// TestGetAllMetadataTransactionRoutesSuccess tests successful retrieval with metadata filter
func TestGetAllMetadataTransactionRoutesSuccess(t *testing.T) {
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
		Metadata:  &bson.M{"key": "value"},
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: transactionRouteID1.String(),
			Data:     mongodb.JSON{"key": "value"},
		},
		{
			ID:       primitive.NewObjectID(),
			EntityID: transactionRouteID2.String(),
			Data:     mongodb.JSON{"key": "value"},
		},
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
		{
			ID:             uuid.New(), // This one should be filtered out
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "route3",
			Description:    "Description 3",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedMetadata, nil)

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedTransactionRoutes, expectedCursor, nil)

	result, cursor, err := uc.GetAllMetadataTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedCursor, cursor)
	assert.Len(t, result, 2)
	assert.Equal(t, expectedTransactionRoutes[0].ID, result[0].ID)
	assert.Equal(t, expectedTransactionRoutes[1].ID, result[1].ID)
	assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
	assert.Equal(t, map[string]any{"key": "value"}, result[1].Metadata)
}

// TestGetAllMetadataTransactionRoutesMetadataError tests metadata repository error handling
func TestGetAllMetadataTransactionRoutesMetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

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
		Metadata:  &bson.M{"key": "value"},
	}

	expectedMetadataError := errors.New("metadata repository error")

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(nil, expectedMetadataError)

	result, cursor, err := uc.GetAllMetadataTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0106", entityNotFoundError.Code)
}

// TestGetAllMetadataTransactionRoutesNoMetadata tests when no metadata is found
func TestGetAllMetadataTransactionRoutesNoMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

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
		Metadata:  &bson.M{"key": "nonexistent"},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(nil, errors.New("metadata not found"))

	result, cursor, err := uc.GetAllMetadataTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0106", entityNotFoundError.Code)
}

// TestGetAllMetadataTransactionRoutesTransactionRouteRepoError tests transaction route repository error
func TestGetAllMetadataTransactionRoutesTransactionRouteRepoError(t *testing.T) {
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
		Metadata:  &bson.M{"key": "value"},
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: transactionRouteID1.String(),
			Data:     mongodb.JSON{"key": "value"},
		},
	}

	expectedRepoError := errors.New("database error")

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedMetadata, nil)

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, expectedRepoError)

	result, cursor, err := uc.GetAllMetadataTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)
	assert.Equal(t, expectedRepoError, err)
}

// TestGetAllMetadataTransactionRoutesTransactionRouteNotFound tests when transaction routes are not found
func TestGetAllMetadataTransactionRoutesTransactionRouteNotFound(t *testing.T) {
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
		Metadata:  &bson.M{"key": "value"},
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: transactionRouteID1.String(),
			Data:     mongodb.JSON{"key": "value"},
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedMetadata, nil)

	mockTransactionRouteRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound)

	result, cursor, err := uc.GetAllMetadataTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0106", entityNotFoundError.Code)
}
