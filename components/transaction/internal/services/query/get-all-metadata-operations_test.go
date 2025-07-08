package query

import (
	"context"
	"errors"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
	"reflect"
	"testing"
)

// TestGetAllMetadataOperations is responsible to test GetAllMetadataOperations with success and error
func TestGetAllMetadataOperations(t *testing.T) {
	collection := reflect.TypeOf(operation.Operation{}).Name()
	filter := http.QueryHeader{
		Metadata: &bson.M{"metadata": 1},
		Limit:    10,
		Page:     1,
	}

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(gomock.NewController(t))
	uc := UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	t.Run("Success", func(t *testing.T) {
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), collection, filter).
			Return([]*mongodb.Metadata{{ID: primitive.NewObjectID()}}, nil).
			Times(1)
		res, err := uc.MetadataRepo.FindList(context.TODO(), collection, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMSG := "errDatabaseItemNotFound"
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), collection, filter).
			Return(nil, errors.New(errMSG)).
			Times(1)
		res, err := uc.MetadataRepo.FindList(context.TODO(), collection, filter)

		assert.EqualError(t, err, errMSG)
		assert.Nil(t, res)
	})
}

// TestGetAllMetadataOperationsWithOperations tests that operations are populated for operations
// retrieved by metadata filtering in the GetAllMetadataOperations method
func TestGetAllMetadataOperationsWithOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)

	orgIDStr := "00000000-0000-0000-0000-000000000001"
	ledgerIDStr := "00000000-0000-0000-0000-000000000002"
	accountIDStr := "00000000-0000-0000-0000-000000000003"
	opID1Str := "00000000-0000-0000-0000-000000000004"
	opID2Str := "00000000-0000-0000-0000-000000000005"

	orgID, _ := uuid.Parse(orgIDStr)
	ledgerID, _ := uuid.Parse(ledgerIDStr)
	accountID, _ := uuid.Parse(accountIDStr)

	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	metadataList := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: opID1Str,
			Data:     map[string]interface{}{"key": "value"},
		},
		{
			ID:       primitive.NewObjectID(),
			EntityID: opID2Str,
			Data:     map[string]interface{}{"key": "value"},
		},
	}

	operations := []*operation.Operation{
		{
			ID:           opID1Str,
			Type:         constant.DEBIT,
			AccountAlias: "source1",
		},
		{
			ID:           opID2Str,
			Type:         constant.CREDIT,
			AccountAlias: "destination2",
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), filter).
		Return(metadataList, nil)

	mockOperationRepo.EXPECT().
		FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
		Return(operations, libHTTP.CursorPagination{}, nil)

	uc := &UseCase{
		MetadataRepo:  mockMetadataRepo,
		OperationRepo: mockOperationRepo,
	}

	result, _, err := uc.GetAllMetadataOperations(context.Background(), orgID, ledgerID, accountID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)

	for _, op := range result {
		assert.NotNil(t, op.Metadata, "Operation metadata should be populated")
		assert.Equal(t, "value", op.Metadata["key"])

		if op.ID == opID1Str {
			assert.Equal(t, constant.DEBIT, op.Type)
			assert.Equal(t, "source1", op.AccountAlias)
		} else if op.ID == opID2Str {
			assert.Equal(t, constant.CREDIT, op.Type)
			assert.Equal(t, "destination2", op.AccountAlias)
		}
	}
}

// TestGetAllMetadataOperationsMetadataNotFound tests error handling when metadata is not found
func TestGetAllMetadataOperationsMetadataNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), filter).
		Return(nil, errors.New("metadata not found"))

	uc := &UseCase{
		MetadataRepo:  mockMetadataRepo,
		OperationRepo: mockOperationRepo,
	}

	result, _, err := uc.GetAllMetadataOperations(context.Background(), orgID, ledgerID, accountID, filter)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "No operations were found in the search")
}

// TestGetAllMetadataOperationsOperationNotFound tests error handling when operations are not found
func TestGetAllMetadataOperationsOperationNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	metadataList := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: "op1",
			Data:     map[string]interface{}{"key": "value"},
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), filter).
		Return(metadataList, nil)

	mockOperationRepo.EXPECT().
		FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
		Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound)

	uc := &UseCase{
		MetadataRepo:  mockMetadataRepo,
		OperationRepo: mockOperationRepo,
	}

	result, _, err := uc.GetAllMetadataOperations(context.Background(), orgID, ledgerID, accountID, filter)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "No operations were found in the search")
}

// TestGetAllMetadataOperationsOperationRepoError tests error handling when operation repository returns error
func TestGetAllMetadataOperationsOperationRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	metadataList := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: "op1",
			Data:     map[string]interface{}{"key": "value"},
		},
	}

	repoError := errors.New("database connection error")

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), filter).
		Return(metadataList, nil)

	mockOperationRepo.EXPECT().
		FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination()).
		Return(nil, libHTTP.CursorPagination{}, repoError)

	uc := &UseCase{
		MetadataRepo:  mockMetadataRepo,
		OperationRepo: mockOperationRepo,
	}

	result, _, err := uc.GetAllMetadataOperations(context.Background(), orgID, ledgerID, accountID, filter)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)
}