package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllTransactions is responsible to test GetAllTransactions with success and error
func TestGetAllTransactions(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
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

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)

	uc := UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
		OperationRepo:   mockOperationRepo,
	}

	t.Run("Success", func(t *testing.T) {
		// Create test data
		transactionID := uuid.New()
		trans := []*transaction.Transaction{
			{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     []*operation.Operation{},
			},
		}

		// Mock metadata
		metadata := []*mongodb.Metadata{
			{
				EntityID:   transactionID.String(),
				EntityName: "Transaction",
				Data:       map[string]any{"key": "value"},
			},
		}

		// Mock operations
		operations := []*operation.Operation{
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           "debit",
				AccountAlias:   "source",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           "credit",
				AccountAlias:   "destination",
			},
		}

		// Mock transaction repo FindAll
		mockTransactionRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		// Mock metadata repo FindList for transactions
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "Transaction", filter).
			Return(metadata, nil).
			Times(1)

		// Mock metadata repo FindList for operations
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "Operation", gomock.Any()).
			Return([]*mongodb.Metadata{
				{
					EntityID:   operations[0].ID,
					EntityName: "Operation",
					Data:       map[string]any{"op_key1": "op_value1"},
				},
				{
					EntityID:   operations[1].ID,
					EntityName: "Operation",
					Data:       map[string]any{"op_key2": "op_value2"},
				},
			}, nil).
			Times(1)

		// Mock operation repo FindAll for GetAllOperations
		mockOperationRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, transactionID, gomock.Any()).
			Return(operations, libHTTP.CursorPagination{}, nil).
			Times(1)

		// Call the actual UseCase method
		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		// Assertions
		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, mockCur, cur)
		assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
		assert.Len(t, result[0].Operations, 2)
	})

	t.Run("Error_FindAll", func(t *testing.T) {
		// Mock transaction repo FindAll to return an error
		mockTransactionRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, errors.New("database error")).
			Times(1)

		// Call the actual UseCase method
		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("Error_ItemNotFound", func(t *testing.T) {
		// Mock transaction repo FindAll to return ItemNotFound error
		mockTransactionRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
			Times(1)

		// Call the actual UseCase method
		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "No transactions were found")
	})

	t.Run("Error_Metadata", func(t *testing.T) {
		// Create test data
		trans := []*transaction.Transaction{
			{
				ID:             uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     []*operation.Operation{},
			},
		}

		// Mock transaction repo FindAll
		mockTransactionRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		// Mock metadata repo FindList for transactions to return an error
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "Transaction", filter).
			Return(nil, errors.New("metadata error")).
			Times(1)

		// Call the actual UseCase method
		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "No transactions were found")
	})

	t.Run("Error_GetOperations", func(t *testing.T) {
		// Create test data
		transactionID := uuid.New()
		trans := []*transaction.Transaction{
			{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     []*operation.Operation{},
			},
		}

		// Mock metadata
		metadata := []*mongodb.Metadata{
			{
				EntityID:   transactionID.String(),
				EntityName: "Transaction",
				Data:       map[string]any{"key": "value"},
			},
		}

		// Mock transaction repo FindAll
		mockTransactionRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		// Mock metadata repo FindList for transactions
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "Transaction", filter).
			Return(metadata, nil).
			Times(1)

		// Mock operation repo FindAll for GetAllOperations to return an error
		mockOperationRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, transactionID, gomock.Any()).
			Return(nil, libHTTP.CursorPagination{}, errors.New("operations error")).
			Times(1)

		// Mock metadata repo FindList for operations - this is needed even though it won't be called
		// because the test expects this call to be set up
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "Operation", gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Call the actual UseCase method
		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "No operations were found")
	})
}
