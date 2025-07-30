package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

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

	uc := UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	t.Run("Success", func(t *testing.T) {
		transactionID := uuid.New()

		operations := []*operation.Operation{
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.DEBIT,
				AccountAlias:   "source",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.CREDIT,
				AccountAlias:   "destination",
			},
		}

		trans := []*transaction.Transaction{
			{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     operations,
				Source:         []string{"source"},
				Destination:    []string{"destination"},
			},
		}

		metadata := []*mongodb.Metadata{
			{
				EntityID:   transactionID.String(),
				EntityName: "Transaction",
				Data:       map[string]any{"key": "value"},
			},
		}

		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "Transaction", filter).
			Return(metadata, nil).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, mockCur, cur)
		assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
		assert.Len(t, result[0].Operations, 2)
		assert.Contains(t, result[0].Source, "source")
		assert.Contains(t, result[0].Destination, "destination")
	})

	t.Run("Error_FindAll", func(t *testing.T) {
		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, errors.New("database error")).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("Error_ItemNotFound", func(t *testing.T) {
		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "No transactions were found")
	})

	t.Run("Error_Metadata", func(t *testing.T) {
		trans := []*transaction.Transaction{
			{
				ID:             uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     []*operation.Operation{},
			},
		}

		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "Transaction", filter).
			Return(nil, errors.New("metadata error")).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "No transactions were found")
	})
}
