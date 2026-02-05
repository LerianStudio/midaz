// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
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

		operationMetadata := []*mongodb.Metadata{
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
		}

		operationIDs := []string{operations[0].ID, operations[1].ID}

		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Transaction", []string{transactionID.String()}).
			Return(metadata, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Operation", operationIDs).
			Return(operationMetadata, nil).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, mockCur, cur)
		assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
		assert.Len(t, result[0].Operations, 2)
		assert.Contains(t, result[0].Source, "source")
		assert.Contains(t, result[0].Destination, "destination")

		assert.Equal(t, map[string]any{"op_key1": "op_value1"}, result[0].Operations[0].Metadata)
		assert.Equal(t, map[string]any{"op_key2": "op_value2"}, result[0].Operations[1].Metadata)
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
			FindByEntityIDs(gomock.Any(), "Transaction", []string{trans[0].ID}).
			Return(nil, errors.New("metadata error")).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "No transactions were found")
	})
}

func TestGetOperationsByTransaction(t *testing.T) {
	t.Parallel()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := uuid.New()
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	tests := []struct {
		name              string
		setupMocks        func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository)
		expectedErr       error
		expectedSourceLen int
		expectedDestLen   int
		expectedOpLen     int
	}{
		{
			name: "success with debit and credit operations",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				ops := []*operation.Operation{
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.DEBIT,
						AccountAlias:   "source1",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.CREDIT,
						AccountAlias:   "dest1",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.DEBIT,
						AccountAlias:   "source2",
					},
				}

				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return(ops, libHTTP.CursorPagination{}, nil).
					Times(1)

				mockMetaRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Operation", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedErr:       nil,
			expectedSourceLen: 2,
			expectedDestLen:   1,
			expectedOpLen:     3,
		},
		{
			name: "success with no operations",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				// When there are no operations, GetAllOperations returns early without calling metadata
				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return([]*operation.Operation{}, libHTTP.CursorPagination{}, nil).
					Times(1)
			},
			expectedErr:       nil,
			expectedSourceLen: 0,
			expectedDestLen:   0,
			expectedOpLen:     0,
		},
		{
			name: "error from GetAllOperations",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return(nil, libHTTP.CursorPagination{}, errors.New("database error")).
					Times(1)
			},
			expectedErr: errors.New("database error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOpRepo := operation.NewMockRepository(ctrl)
			mockMetaRepo := mongodb.NewMockRepository(ctrl)

			tt.setupMocks(mockOpRepo, mockMetaRepo)

			uc := UseCase{
				OperationRepo: mockOpRepo,
				MetadataRepo:  mockMetaRepo,
			}

			tran := &transaction.Transaction{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
			}

			result, err := uc.GetOperationsByTransaction(context.Background(), organizationID, ledgerID, tran, filter)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
				assert.Nil(t, result)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Len(t, result.Source, tt.expectedSourceLen)
			assert.Len(t, result.Destination, tt.expectedDestLen)
			assert.Len(t, result.Operations, tt.expectedOpLen)
		})
	}
}
