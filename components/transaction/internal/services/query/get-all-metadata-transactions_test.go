package query

import (
	"context"
	"errors"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
	"reflect"
	"testing"
)

// TestGetAllMetadataTransactions is responsible to test GetAllMetadataTransactions with success and error
func TestGetAllMetadataTransactions(t *testing.T) {
	collection := reflect.TypeOf(transaction.Transaction{}).Name()
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

// TestGetAllMetadataTransactionsWithOperations tests that operations are populated for transactions
// retrieved by metadata filtering in the GetAllMetadataTransactions method
func TestGetAllMetadataTransactionsWithOperations(t *testing.T) {
	// Setup test controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock repositories
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)

	// Create test data
	orgIDStr := "00000000-0000-0000-0000-000000000001"
	ledgerIDStr := "00000000-0000-0000-0000-000000000002"
	txID1Str := "00000000-0000-0000-0000-000000000003"
	txID2Str := "00000000-0000-0000-0000-000000000004"

	orgID, _ := uuid.Parse(orgIDStr)
	ledgerID, _ := uuid.Parse(ledgerIDStr)
	txID1, _ := uuid.Parse(txID1Str)
	txID2, _ := uuid.Parse(txID2Str)

	// Create filter
	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	// Create metadata list
	metadataList := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: txID1Str,
			Data:     map[string]interface{}{"key": "value"},
		},
		{
			ID:       primitive.NewObjectID(),
			EntityID: txID2Str,
			Data:     map[string]interface{}{"key": "value"},
		},
	}

	// Create transactions
	transactions := []*transaction.Transaction{
		{ID: txID1Str},
		{ID: txID2Str},
	}

	// Create operations for transactions
	ops1 := []*operation.Operation{{
		ID: "op1-" + txID1Str,
	}}  
	ops2 := []*operation.Operation{{
		ID: "op2-" + txID2Str,
	}}

	// Setup mock expectations
	// 1. FindList should return our metadata list
	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(transaction.Transaction{}).Name(), filter).
		Return(metadataList, nil)

	// 2. ListByIDs should return our transactions
	mockTransactionRepo.EXPECT().
		ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, ids []uuid.UUID) ([]*transaction.Transaction, error) {
			// Verify that the correct transaction IDs are being requested
			assert.Equal(t, 2, len(ids))
			assert.Contains(t, []uuid.UUID{txID1, txID2}, ids[0])
			assert.Contains(t, []uuid.UUID{txID1, txID2}, ids[1])
			return transactions, nil
		})

	// 3. For each transaction, GetAllOperations should be called to populate operations
	// This is the key part we're testing - that operations are populated
	mockOperationRepo.EXPECT().
		FindAll(gomock.Any(), orgID, ledgerID, txID1, gomock.Any()).
		Return(ops1, libHTTP.CursorPagination{}, nil)

	mockOperationRepo.EXPECT().
		FindAll(gomock.Any(), orgID, ledgerID, txID2, gomock.Any()).
		Return(ops2, libHTTP.CursorPagination{}, nil)

	// 4. Metadata for operations might be fetched
	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), gomock.Any()).
		Return([]*mongodb.Metadata{}, nil).AnyTimes()

	// Setup the UseCase with our mocks
	uc := &UseCase{
		MetadataRepo:    mockMetadataRepo,
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
	}

	// Call the method under test
	result, err := uc.GetAllMetadataTransactions(context.Background(), orgID, ledgerID, filter)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)

	// Verify operations were populated
	for _, tx := range result {
		assert.NotEmpty(t, tx.Operations, "Transaction operations should be populated")
		assert.NotNil(t, tx.Metadata, "Transaction metadata should be populated")
		assert.Equal(t, "value", tx.Metadata["key"])
		
		// Verify the correct operations were assigned to each transaction
		if tx.ID == txID1Str {
			assert.Equal(t, "op1-"+txID1Str, tx.Operations[0].ID)
		} else if tx.ID == txID2Str {
			assert.Equal(t, "op2-"+txID2Str, tx.Operations[0].ID)
		}
	}
}
