// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllOperationsByAccount(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	t.Run("with_metadata", func(t *testing.T) {
		op1ID := libCommons.GenerateUUIDv7().String()
		op2ID := libCommons.GenerateUUIDv7().String()
		operations := []*operation.Operation{
			{ID: op1ID},
			{ID: op2ID},
		}

		metadata := []*mongodb.Metadata{
			{
				EntityID: op1ID,
				Data:     mongodb.JSON{"key1": "value1"},
			},
			{
				EntityID: op2ID,
				Data:     mongodb.JSON{"key2": "value2"},
			},
		}

		mockOperationRepo.
			EXPECT().
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
			Return(operations, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, collection string, entityIDs []string) ([]*mongodb.Metadata, error) {
				assert.ElementsMatch(t, []string{op1ID, op2ID}, entityIDs)
				return metadata, nil
			}).
			Times(1)

		result, cur, err := uc.GetAllOperationsByAccount(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, mockCur, cur)

		assert.Equal(t, "value1", result[0].Metadata["key1"])
		assert.Equal(t, "value2", result[1].Metadata["key2"])
	})

	t.Run("empty_operations", func(t *testing.T) {
		mockOperationRepo.
			EXPECT().
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
			Return([]*operation.Operation{}, mockCur, nil).
			Times(1)

		result, cur, err := uc.GetAllOperationsByAccount(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Empty(t, result)
		assert.Equal(t, mockCur, cur)
	})

	t.Run("repo_error_not_found", func(t *testing.T) {
		mockOperationRepo.
			EXPECT().
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
			Times(1)

		result, cur, err := uc.GetAllOperationsByAccount(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "No operations were found")
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	t.Run("repo_error_generic", func(t *testing.T) {
		mockOperationRepo.
			EXPECT().
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, errors.New("database connection error")).
			Times(1)

		result, cur, err := uc.GetAllOperationsByAccount(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	t.Run("metadata_error", func(t *testing.T) {
		operations := []*operation.Operation{{ID: libCommons.GenerateUUIDv7().String()}}

		mockOperationRepo.
			EXPECT().
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
			Return(operations, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), gomock.Any()).
			Return(nil, errors.New("metadata error")).
			Times(1)

		result, cur, err := uc.GetAllOperationsByAccount(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	t.Run("partial_metadata", func(t *testing.T) {
		op1ID := libCommons.GenerateUUIDv7().String()
		op2ID := libCommons.GenerateUUIDv7().String()
		operations := []*operation.Operation{
			{ID: op1ID},
			{ID: op2ID},
		}

		// Only op1 has metadata
		metadata := []*mongodb.Metadata{
			{
				EntityID: op1ID,
				Data:     mongodb.JSON{"key1": "value1"},
			},
		}

		mockOperationRepo.
			EXPECT().
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
			Return(operations, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), gomock.Any()).
			Return(metadata, nil).
			Times(1)

		result, cur, err := uc.GetAllOperationsByAccount(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, mockCur, cur)

		// op1 has metadata
		assert.Equal(t, "value1", result[0].Metadata["key1"])
		// op2 has no metadata
		assert.Nil(t, result[1].Metadata)
	})
}
