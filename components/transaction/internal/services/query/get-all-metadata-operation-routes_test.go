// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v3/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
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

func TestGetAllMetadataOperationRoutes(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()
	operationRouteID2 := uuid.New()

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		Metadata:  &bson.M{"key": "value"},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	t.Run("success_with_metadata_filtering", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		uc := &UseCase{
			OperationRouteRepo: mockOperationRouteRepo,
			MetadataRepo:       mockMetadataRepo,
		}

		expectedMetadata := []*mongodb.Metadata{
			{
				ID:       primitive.NewObjectID(),
				EntityID: operationRouteID1.String(),
				Data:     mongodb.JSON{"key": "value"},
			},
			{
				ID:       primitive.NewObjectID(),
				EntityID: operationRouteID2.String(),
				Data:     mongodb.JSON{"key": "value"},
			},
		}

		// Third route should be filtered out (no matching metadata)
		expectedOperationRoutes := []*mmodel.OperationRoute{
			{
				ID:             operationRouteID1,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Description:    "Description 1",
			},
			{
				ID:             operationRouteID2,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Description:    "Description 2",
			},
			{
				ID:             uuid.New(),
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Description:    "Description 3 - should be filtered out",
			},
		}

		mockMetadataRepo.EXPECT().
			FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), gomock.Any()).
			Return(expectedMetadata, nil)

		mockOperationRouteRepo.EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(expectedOperationRoutes, expectedCursor, nil)

		result, cursor, err := uc.GetAllMetadataOperationRoutes(context.Background(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Equal(t, expectedCursor, cursor)
		assert.Len(t, result, 2)
		assert.Equal(t, operationRouteID1, result[0].ID)
		assert.Equal(t, operationRouteID2, result[1].ID)
		assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
		assert.Equal(t, map[string]any{"key": "value"}, result[1].Metadata)
	})

	t.Run("metadata_repo_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		uc := &UseCase{
			OperationRouteRepo: mockOperationRouteRepo,
			MetadataRepo:       mockMetadataRepo,
		}

		mockMetadataRepo.EXPECT().
			FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), gomock.Any()).
			Return(nil, errors.New("metadata repository error"))

		result, cursor, err := uc.GetAllMetadataOperationRoutes(context.Background(), organizationID, ledgerID, filter)

		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cursor)

		var entityNotFoundError pkg.EntityNotFoundError
		assert.True(t, errors.As(err, &entityNotFoundError))
		assert.Equal(t, "0102", entityNotFoundError.Code)
	})

	t.Run("metadata_returns_nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		uc := &UseCase{
			OperationRouteRepo: mockOperationRouteRepo,
			MetadataRepo:       mockMetadataRepo,
		}

		mockMetadataRepo.EXPECT().
			FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), gomock.Any()).
			Return(nil, nil)

		result, cursor, err := uc.GetAllMetadataOperationRoutes(context.Background(), organizationID, ledgerID, filter)

		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cursor)

		var entityNotFoundError pkg.EntityNotFoundError
		assert.True(t, errors.As(err, &entityNotFoundError))
		assert.Equal(t, "0102", entityNotFoundError.Code)
	})

	t.Run("operation_route_repo_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		uc := &UseCase{
			OperationRouteRepo: mockOperationRouteRepo,
			MetadataRepo:       mockMetadataRepo,
		}

		expectedMetadata := []*mongodb.Metadata{
			{
				ID:       primitive.NewObjectID(),
				EntityID: operationRouteID1.String(),
				Data:     mongodb.JSON{"key": "value"},
			},
		}

		mockMetadataRepo.EXPECT().
			FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), gomock.Any()).
			Return(expectedMetadata, nil)

		mockOperationRouteRepo.EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(nil, libHTTP.CursorPagination{}, errors.New("database error"))

		result, cursor, err := uc.GetAllMetadataOperationRoutes(context.Background(), organizationID, ledgerID, filter)

		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cursor)
		assert.Error(t, err)
		assert.Equal(t, "database error", err.Error())
	})

	t.Run("operation_route_not_found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		uc := &UseCase{
			OperationRouteRepo: mockOperationRouteRepo,
			MetadataRepo:       mockMetadataRepo,
		}

		expectedMetadata := []*mongodb.Metadata{
			{
				ID:       primitive.NewObjectID(),
				EntityID: operationRouteID1.String(),
				Data:     mongodb.JSON{"key": "value"},
			},
		}

		mockMetadataRepo.EXPECT().
			FindList(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), gomock.Any()).
			Return(expectedMetadata, nil)

		mockOperationRouteRepo.EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound)

		result, cursor, err := uc.GetAllMetadataOperationRoutes(context.Background(), organizationID, ledgerID, filter)

		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cursor)

		var entityNotFoundError pkg.EntityNotFoundError
		assert.True(t, errors.As(err, &entityNotFoundError))
		assert.Equal(t, "0102", entityNotFoundError.Code)
	})
}
