package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
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
		MetadataTransactionRepo: mockMetadataRepo,
	}

	t.Run("Success", func(t *testing.T) {
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), collection, filter).
			Return([]*mongodb.Metadata{{ID: primitive.NewObjectID()}}, nil).
			Times(1)
		res, err := uc.MetadataTransactionRepo.FindList(context.TODO(), collection, filter)

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
		res, err := uc.MetadataTransactionRepo.FindList(context.TODO(), collection, filter)

		assert.EqualError(t, err, errMSG)
		assert.Nil(t, res)
	})
}

// TestGetAllMetadataTransactionsWithOperations tests that operations are populated for transactions
// retrieved by metadata filtering in the GetAllMetadataTransactions method
func TestGetAllMetadataTransactionsWithOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	orgIDStr := "00000000-0000-0000-0000-000000000001"
	ledgerIDStr := "00000000-0000-0000-0000-000000000002"
	txID1Str := "00000000-0000-0000-0000-000000000003"
	txID2Str := "00000000-0000-0000-0000-000000000004"

	orgID, _ := uuid.Parse(orgIDStr)
	ledgerID, _ := uuid.Parse(ledgerIDStr)
	txID1, _ := uuid.Parse(txID1Str)
	txID2, _ := uuid.Parse(txID2Str)

	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

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

	opMeta := []*mongodb.Metadata{
		{
			EntityID:   "op1-" + txID1Str,
			EntityName: "Operation",
			Data:       map[string]any{"op_key1": "op_value1"},
		},
		{
			EntityID:   "op2-" + txID2Str,
			EntityName: "Operation",
			Data:       map[string]any{"op_key2": "op_value2"},
		},
	}

	ops1 := []*operation.Operation{{
		ID:           "op1-" + txID1Str,
		Type:         constant.DEBIT,
		AccountAlias: "source1",
	}}
	ops2 := []*operation.Operation{{
		ID:           "op2-" + txID2Str,
		Type:         constant.CREDIT,
		AccountAlias: "destination2",
	}}

	transactions := []*transaction.Transaction{
		{
			ID:          txID1Str,
			Operations:  ops1,
			Source:      []string{"source1"},
			Destination: []string{},
		},
		{
			ID:          txID2Str,
			Operations:  ops2,
			Source:      []string{},
			Destination: []string{"destination2"},
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(transaction.Transaction{}).Name(), filter).
		Return(metadataList, nil)

	mockTransactionRepo.EXPECT().
		FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{txID1, txID2}, filter.ToCursorPagination()).
		Return(transactions, libHTTP.CursorPagination{}, nil)

	// Expect operation metadata lookup with both operation IDs
	mockMetadataRepo.EXPECT().
		FindByEntityIDs(
			gomock.Any(),
			reflect.TypeOf(operation.Operation{}).Name(),
			[]string{"op1-" + txID1Str, "op2-" + txID2Str},
		).
		Return(opMeta, nil)

	uc := &UseCase{
		MetadataTransactionRepo: mockMetadataRepo,
		TransactionRepo:         mockTransactionRepo,
	}

	result, _, err := uc.GetAllMetadataTransactions(context.Background(), orgID, ledgerID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)

	for _, tx := range result {
		assert.NotEmpty(t, tx.Operations, "Transaction operations should be populated")
		assert.NotNil(t, tx.Metadata, "Transaction metadata should be populated")
		assert.Equal(t, "value", tx.Metadata["key"])
		assert.NotNil(t, tx.Operations[0].Metadata, "Operation metadata should be populated")

		if tx.ID == txID1Str {
			assert.Equal(t, "op1-"+txID1Str, tx.Operations[0].ID)
			assert.Contains(t, tx.Source, "source1")
			assert.Equal(t, "op_value1", tx.Operations[0].Metadata["op_key1"])
		} else if tx.ID == txID2Str {
			assert.Equal(t, "op2-"+txID2Str, tx.Operations[0].ID)
			assert.Contains(t, tx.Destination, "destination2")
			assert.Equal(t, "op_value2", tx.Operations[0].Metadata["op_key2"])
		}
	}
}

// TestGetAllMetadataTransactions_NoMetadata ensures that when metadata lookup
// returns an empty (non-nil) slice, the use case returns no transactions and no error,
// and does not call the transaction repository.
func TestGetAllMetadataTransactions_NoMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	collection := reflect.TypeOf(transaction.Transaction{}).Name()
	filter := http.QueryHeader{
		Metadata: &bson.M{"k": "v"},
		Limit:    10,
		Page:     1,
	}

	// Return an empty, non-nil slice to hit the early-return branch.
	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), collection, filter).
		Return([]*mongodb.Metadata{}, nil)

	uc := &UseCase{
		MetadataTransactionRepo: mockMetadataRepo,
		TransactionRepo:         mockTransactionRepo, // must not be called
	}

	result, cur, err := uc.GetAllMetadataTransactions(context.Background(), uuid.UUID{}, uuid.UUID{}, filter)

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cur)
}
