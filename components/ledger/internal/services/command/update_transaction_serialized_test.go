// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type ambiguousCommitDBTransaction struct {
	commitErr      error
	commitCalled   bool
	rollbackCalled bool
}

func (tx *ambiguousCommitDBTransaction) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}

func (tx *ambiguousCommitDBTransaction) Commit() error {
	tx.commitCalled = true

	return tx.commitErr
}

func (tx *ambiguousCommitDBTransaction) Rollback() error {
	tx.rollbackCalled = true

	return nil
}

func TestUpdateTransactionSerialized_AmbiguousCommitUsesExactPrimaryProof(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("postgres commit response lost")
	readErr := errors.New("postgres primary unavailable")

	tests := []struct {
		name             string
		persisted        func(*transaction.Transaction, time.Time) *transaction.Transaction
		readErr          error
		readCalls        int
		existingMetadata map[string]any
		inputMetadata    map[string]any
		wantRelease      int
		wantSuccess      bool
		wantCommitErr    bool
	}{
		{
			name: "exact applied version releases rollout lease and succeeds",
			persisted: func(before *transaction.Transaction, version time.Time) *transaction.Transaction {
				applied := *before
				applied.Description = "patched description"
				applied.UpdatedAt = version

				return &applied
			},
			readCalls:   1,
			wantRelease: 1,
			wantSuccess: true,
		},
		{
			name: "exact rollback version releases rollout lease for retry",
			persisted: func(before *transaction.Transaction, _ time.Time) *transaction.Transaction {
				rolledBack := *before

				return &rolledBack
			},
			readCalls:     1,
			wantRelease:   1,
			wantCommitErr: true,
		},
		{
			name: "postgres rollback after metadata changed retains rollout lease",
			persisted: func(before *transaction.Transaction, _ time.Time) *transaction.Transaction {
				rolledBack := *before

				return &rolledBack
			},
			existingMetadata: map[string]any{"existing": "value"},
			inputMetadata:    map[string]any{"patched": "value"},
			readCalls:        1,
		},
		{
			name: "divergent primary state retains rollout lease for reconciliation",
			persisted: func(before *transaction.Transaction, version time.Time) *transaction.Transaction {
				divergent := *before
				divergent.Description = "another payload"
				divergent.UpdatedAt = version.Add(time.Microsecond)

				return &divergent
			},
			readCalls: 1,
		},
		{
			name:      "unreadable primary retains rollout lease after bounded retries",
			readErr:   readErr,
			readCalls: transactionUpdateCommitReadAttempts,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			transactionRepo := transaction.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)
			dbTx := &ambiguousCommitDBTransaction{commitErr: commitErr}
			organizationID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()
			before := &transaction.Transaction{
				ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
				Description: "original description", Status: transaction.Status{Code: constant.APPROVED},
				UpdatedAt: time.Date(2026, time.August, 18, 12, 0, 0, 123000000, time.UTC),
			}
			var updateVersion time.Time

			transactionRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil)
			transactionRepo.EXPECT().FindForUpdate(gomock.Any(), dbTx, organizationID, ledgerID, transactionID).
				Return(before, nil)
			transactionRepo.EXPECT().UpdateTx(gomock.Any(), dbTx, organizationID, ledgerID, transactionID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ any, _, _, _ uuid.UUID, update *transaction.Transaction) (*transaction.Transaction, error) {
					updateVersion = update.UpdatedAt
					assert.True(t, updateVersion.After(before.UpdatedAt))

					return &transaction.Transaction{UpdatedAt: updateVersion}, nil
				})
			if tc.inputMetadata == nil {
				metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction, transactionID.String()).
					Return(nil, nil)
				metadataRepo.EXPECT().Update(gomock.Any(), constant.EntityTransaction, transactionID.String(), map[string]any{}).
					Return(nil)
			} else {
				existing := &mongodb.Metadata{Data: tc.existingMetadata}
				metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction, transactionID.String()).
					Return(existing, nil)
				metadataRepo.EXPECT().Update(gomock.Any(), constant.EntityTransaction, transactionID.String(),
					map[string]any{"existing": "value", "patched": "value"}).Return(nil)
			}
			if tc.readCalls > 0 {
				transactionRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, transactionID).
					DoAndReturn(func(ctx context.Context, _, _, _ uuid.UUID) (*transaction.Transaction, error) {
						require.True(t, readrouting.IsPrimaryRead(ctx), "ambiguous commit must be reconciled on primary")
						if tc.readErr != nil {
							return nil, tc.readErr
						}

						return tc.persisted(before, updateVersion), nil
					}).Times(tc.readCalls)
			}

			releases := 0
			uc := &UseCase{TransactionRepo: transactionRepo, TransactionMetadataRepo: metadataRepo}
			result, err := uc.UpdateTransactionSerialized(context.Background(), organizationID, ledgerID, transactionID,
				&transaction.UpdateTransactionInput{Description: "patched description", Metadata: tc.inputMetadata},
				func(context.Context, string) (func() error, error) {
					return func() error {
						releases++

						return nil
					}, nil
				})

			assert.True(t, dbTx.commitCalled)
			assert.Equal(t, tc.wantRelease, releases)
			if tc.wantSuccess {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "patched description", result.Description)
				assert.False(t, dbTx.rollbackCalled)

				return
			}

			require.Error(t, err)
			assert.Nil(t, result)
			assert.True(t, dbTx.rollbackCalled)
			if tc.wantCommitErr {
				assert.ErrorIs(t, err, commitErr)

				return
			}

			var unavailable pkg.ServiceUnavailableError
			require.ErrorAs(t, err, &unavailable)
			assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
		})
	}
}
