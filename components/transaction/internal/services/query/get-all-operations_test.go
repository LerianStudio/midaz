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
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/mock/gomock"
)

func TestGetAllOperations(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
		Metadata:     &bson.M{},
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

	t.Run("Success with metadata", func(t *testing.T) {
		op1ID := libCommons.GenerateUUIDv7().String()
		op2ID := libCommons.GenerateUUIDv7().String()
		operations := []*operation.Operation{
			{ID: op1ID},
			{ID: op2ID},
		}

		// Setup test metadata
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
			FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
			Return(operations, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, collection string, queryFilter http.QueryHeader) ([]*mongodb.Metadata, error) {
				// Verify the filter has the correct metadata type
				assert.IsType(t, &bson.M{}, queryFilter.Metadata)
				return metadata, nil
			}).
			Times(1)

		result, cur, err := uc.GetAllOperations(context.TODO(), organizationID, ledgerID, transactionID, filter)

		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, mockCur, cur)

		assert.Equal(t, "value1", result[0].Metadata["key1"])
		assert.Equal(t, "value2", result[1].Metadata["key2"])
	})

	t.Run("Success with no operations", func(t *testing.T) {
		mockOperationRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
			Return(nil, mockCur, nil).
			Times(1)

		result, cur, err := uc.GetAllOperations(context.TODO(), organizationID, ledgerID, transactionID, filter)

		assert.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, mockCur, cur)
	})

	t.Run("Error in FindAll", func(t *testing.T) {
		mockOperationRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
			Times(1)

		result, cur, err := uc.GetAllOperations(context.TODO(), organizationID, ledgerID, transactionID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	t.Run("Error in FindList metadata", func(t *testing.T) {
		operations := []*operation.Operation{{ID: libCommons.GenerateUUIDv7().String()}}

		mockOperationRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
			Return(operations, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), gomock.Any()).
			Return(nil, errors.New("metadata error")).
			Times(1)

		result, cur, err := uc.GetAllOperations(context.TODO(), organizationID, ledgerID, transactionID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})
}
