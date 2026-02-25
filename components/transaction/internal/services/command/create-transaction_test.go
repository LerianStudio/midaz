// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUseCase_CreateTransaction(t *testing.T) {
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	parentTxID := libCommons.GenerateUUIDv7()

	tests := []struct {
		name          string
		transactionID uuid.UUID
		dsl           *pkgTransaction.Transaction
		setupMocks    func(ctrl *gomock.Controller, uc *UseCase)
		validate      func(t *testing.T, result *transaction.Transaction, err error)
	}{
		{
			name:          "creates_transaction_with_approved_status",
			transactionID: uuid.Nil,
			dsl: &pkgTransaction.Transaction{
				Description: "Test transaction",
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockTxRepo := transaction.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockTxRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, tx *transaction.Transaction) (*transaction.Transaction, error) {
						assert.Equal(t, constant.APPROVED, tx.Status.Code)
						assert.Equal(t, constant.APPROVED, *tx.Status.Description)
						assert.Equal(t, orgID.String(), tx.OrganizationID)
						assert.Equal(t, ledgerID.String(), tx.LedgerID)
						assert.Equal(t, "Test transaction", tx.Description)
						assert.Equal(t, "USD", tx.AssetCode)
						assert.Nil(t, tx.ParentTransactionID)
						return tx, nil
					}).
					Times(1)

				uc.TransactionRepo = mockTxRepo
				uc.MetadataRepo = mockMetaRepo
			},
			validate: func(t *testing.T, result *transaction.Transaction, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, constant.APPROVED, result.Status.Code)
			},
		},
		{
			name:          "sets_parent_transaction_id_when_provided",
			transactionID: parentTxID,
			dsl: &pkgTransaction.Transaction{
				Description: "Child transaction",
				Send: pkgTransaction.Send{
					Asset: "BRL",
					Value: decimal.NewFromInt(500),
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockTxRepo := transaction.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockTxRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, tx *transaction.Transaction) (*transaction.Transaction, error) {
						require.NotNil(t, tx.ParentTransactionID)
						assert.Equal(t, parentTxID.String(), *tx.ParentTransactionID)
						return tx, nil
					}).
					Times(1)

				uc.TransactionRepo = mockTxRepo
				uc.MetadataRepo = mockMetaRepo
			},
			validate: func(t *testing.T, result *transaction.Transaction, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, result.ParentTransactionID)
				assert.Equal(t, parentTxID.String(), *result.ParentTransactionID)
			},
		},
		{
			name:          "persists_metadata_when_provided",
			transactionID: uuid.Nil,
			dsl: &pkgTransaction.Transaction{
				Description: "Transaction with metadata",
				Metadata:    map[string]any{"key": "value", "number": 42},
				Send: pkgTransaction.Send{
					Asset: "EUR",
					Value: decimal.NewFromInt(200),
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockTxRepo := transaction.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockTxRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, tx *transaction.Transaction) (*transaction.Transaction, error) {
						return tx, nil
					}).
					Times(1)

				mockMetaRepo.EXPECT().
					Create(gomock.Any(), "Transaction", gomock.Any()).
					DoAndReturn(func(ctx context.Context, entityName string, meta *mongodb.Metadata) error {
						assert.Equal(t, "Transaction", meta.EntityName)
						assert.Equal(t, "value", meta.Data["key"])
						assert.Equal(t, 42, meta.Data["number"])
						return nil
					}).
					Times(1)

				uc.TransactionRepo = mockTxRepo
				uc.MetadataRepo = mockMetaRepo
			},
			validate: func(t *testing.T, result *transaction.Transaction, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "value", result.Metadata["key"])
				assert.Equal(t, 42, result.Metadata["number"])
			},
		},
		{
			name:          "skips_metadata_creation_when_nil",
			transactionID: uuid.Nil,
			dsl: &pkgTransaction.Transaction{
				Description: "No metadata",
				Metadata:    nil,
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(50),
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockTxRepo := transaction.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockTxRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, tx *transaction.Transaction) (*transaction.Transaction, error) {
						return tx, nil
					}).
					Times(1)

				// MetadataRepo.Create should NOT be called
				uc.TransactionRepo = mockTxRepo
				uc.MetadataRepo = mockMetaRepo
			},
			validate: func(t *testing.T, result *transaction.Transaction, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Nil(t, result.Metadata)
			},
		},
		{
			name:          "returns_error_when_transaction_repo_fails",
			transactionID: uuid.Nil,
			dsl: &pkgTransaction.Transaction{
				Description: "Will fail",
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockTxRepo := transaction.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockTxRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database connection error")).
					Times(1)

				uc.TransactionRepo = mockTxRepo
				uc.MetadataRepo = mockMetaRepo
			},
			validate: func(t *testing.T, result *transaction.Transaction, err error) {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), "database connection error")
			},
		},
		{
			name:          "returns_error_when_metadata_repo_fails",
			transactionID: uuid.Nil,
			dsl: &pkgTransaction.Transaction{
				Description: "Metadata will fail",
				Metadata:    map[string]any{"key": "value"},
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockTxRepo := transaction.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockTxRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, tx *transaction.Transaction) (*transaction.Transaction, error) {
						return tx, nil
					}).
					Times(1)

				mockMetaRepo.EXPECT().
					Create(gomock.Any(), "Transaction", gomock.Any()).
					Return(errors.New("mongodb connection error")).
					Times(1)

				uc.TransactionRepo = mockTxRepo
				uc.MetadataRepo = mockMetaRepo
			},
			validate: func(t *testing.T, result *transaction.Transaction, err error) {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), "mongodb connection error")
			},
		},
		{
			name:          "sets_chart_of_accounts_group_name",
			transactionID: uuid.Nil,
			dsl: &pkgTransaction.Transaction{
				Description:              "With chart of accounts",
				ChartOfAccountsGroupName: "revenue-1000",
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockTxRepo := transaction.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockTxRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, tx *transaction.Transaction) (*transaction.Transaction, error) {
						assert.Equal(t, "revenue-1000", tx.ChartOfAccountsGroupName)
						return tx, nil
					}).
					Times(1)

				uc.TransactionRepo = mockTxRepo
				uc.MetadataRepo = mockMetaRepo
			},
			validate: func(t *testing.T, result *transaction.Transaction, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "revenue-1000", result.ChartOfAccountsGroupName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			uc := &UseCase{}
			tt.setupMocks(ctrl, uc)

			ctx := context.Background()
			result, err := uc.CreateTransaction(ctx, orgID, ledgerID, tt.transactionID, tt.dsl)

			tt.validate(t, result, err)
		})
	}
}
