// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// annotationBalanceReader serves the balances of an annotation's legs, so a create
// runs past the balance read into the operation build.
type annotationBalanceReader struct {
	*versionReader
	balances []*mmodel.Balance
}

func (r *annotationBalanceReader) GetBalances(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
	r.getBalancesCalls++

	return r.balances, nil
}

// annotationLegBalances returns the balances behind skippingTransaction's legs.
func annotationLegBalances() []*mmodel.Balance {
	balances := make([]*mmodel.Balance, 0, 2)

	for _, alias := range []string{"@payer", "@payee"} {
		balances = append(balances, &mmodel.Balance{
			ID:             uuid.NewString(),
			AccountID:      uuid.NewString(),
			Alias:          alias,
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		})
	}

	return balances
}

// createAnnotation runs an annotation create through the use case of the given
// route version.
func createAnnotation(uc *UseCase, version string, organizationID, ledgerID uuid.UUID) error {
	input := skippingTransaction()
	input.Skip = nil

	if version == "v1" {
		_, _, err := uc.CreateTransactionV1(context.Background(), CreateTransactionV1Input{
			OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input,
			TransactionStatus: constant.NOTED, IdempotencyTTL: time.Minute,
		})

		return err
	}

	_, _, err := uc.CreateTransactionV2(context.Background(), CreateTransactionV2Input{
		OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input,
		TransactionStatus: constant.NOTED, IdempotencyTTL: time.Minute,
	})

	return err
}

// TestCreateAnnotationWritesItsBackupOnceWithOperations proves the annotation's
// backup-queue entry is written once, after the balance read, already carrying the
// operations of the transaction the client reads, and that it stays for the legacy
// replay when the database write fails: the claim is kept and the answer is the
// sanitized persistence error. The backup is never read back to be rewritten.
func TestCreateAnnotationWritesItsBackupOnceWithOperations(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			transactionRepo := transaction.NewMockRepository(ctrl)
			reader := &annotationBalanceReader{versionReader: &versionReader{}, balances: annotationLegBalances()}
			uc := &UseCase{
				TransactionRedisRepo:        redisRepo,
				TransactionReader:           reader,
				TransactionRepo:             transactionRepo,
				AppliedTransactionCompleter: &createAppliedTransactionCompleter{},
			}
			organizationID := uuid.New()
			ledgerID := uuid.New()

			var (
				backupKey   string
				backup      []byte
				writeBehind []byte
			)

			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
			redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, key string, raw []byte) error {
					assert.Equal(t, 1, reader.getBalancesCalls, "the backup must be written after the balance read")

					backupKey, backup = key, raw

					return nil
				}).
				Times(1)
			redisRepo.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, data []byte, _ time.Duration) error {
					writeBehind = data

					return nil
				}).
				Times(1)
			transactionRepo.EXPECT().BeginTx(gomock.Any()).Return(nil, errors.New("transaction database unavailable")).Times(1)

			err := createAnnotation(uc, version, organizationID, ledgerID)

			var persistenceErr pkg.InternalServerError

			require.ErrorAs(t, err, &persistenceErr)
			assert.Equal(t, constant.ErrMessageBrokerUnavailable.Error(), persistenceErr.Code)

			var served transaction.Transaction

			require.NoError(t, msgpack.Unmarshal(writeBehind, &served))
			require.Len(t, served.Operations, 2, "the annotation carries one operation per leg")

			var entry mmodel.TransactionRedisQueue

			require.NoError(t, json.Unmarshal(backup, &entry))
			assert.Equal(t, utils.TransactionInternalKey(organizationID, ledgerID, served.ID), backupKey)
			assert.Equal(t, constant.NOTED, entry.TransactionStatus)
			assert.Empty(t, entry.Balances, "the annotation backup carries no balance snapshots")
			require.Len(t, entry.Operations, len(served.Operations))

			for i, op := range served.Operations {
				assert.Equal(t, op.ID, entry.Operations[i].ID, "the backup must carry the operations the client reads")
			}
		})
	}
}

// TestCreateAnnotationWritesNoBackupWhenOperationsCannotBeBuilt proves that an
// annotation failing to build its operations, after its balances loaded, releases its
// idempotency claim and leaves no backup-queue entry for the replay to persist. A
// balance whose overdraft limit is not a decimal passes the balance stage and fails the
// build.
func TestCreateAnnotationWritesNoBackupWhenOperationsCannotBeBuilt(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			corruptLimit := "not-a-decimal"
			balances := annotationLegBalances()
			balances[0].Settings = &mmodel.BalanceSettings{OverdraftLimit: &corruptLimit}
			reader := &annotationBalanceReader{versionReader: &versionReader{}, balances: balances}
			uc := &UseCase{
				TransactionRedisRepo:        redisRepo,
				TransactionReader:           reader,
				TransactionRepo:             transaction.NewMockRepository(ctrl),
				AppliedTransactionCompleter: &createAppliedTransactionCompleter{},
			}

			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
			redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

			err := createAnnotation(uc, version, uuid.New(), uuid.New())

			require.ErrorContains(t, err, `invalid OverdraftLimit "not-a-decimal"`)
			assert.Equal(t, 1, reader.getBalancesCalls, "the failure must come after the balance stage")
		})
	}
}

// TestCreateAnnotationReleasesItsClaimAndEntryWhenTheBackupWriteFails proves that a
// failed backup-queue write answers the backup-cache error, releases the idempotency
// claim, and removes the entry the ambiguous write may still have stored, so the
// replay cannot persist an annotation whose client was told it failed. Nothing is
// written to the write-behind cache or the database: neither mock registers a call.
func TestCreateAnnotationReleasesItsClaimAndEntryWhenTheBackupWriteFails(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			reader := &annotationBalanceReader{versionReader: &versionReader{}, balances: annotationLegBalances()}
			uc := &UseCase{
				TransactionRedisRepo:        redisRepo,
				TransactionReader:           reader,
				TransactionRepo:             transaction.NewMockRepository(ctrl),
				AppliedTransactionCompleter: &createAppliedTransactionCompleter{},
			}

			var backupKey string

			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
			redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, key string, _ []byte) error {
					backupKey = key

					return errors.New("backup queue write timed out")
				}).
				Times(1)
			redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			redisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, key string) error {
					assert.Equal(t, backupKey, key, "the removal must target the entry the failed write addressed")

					return nil
				}).
				Times(1)

			err := createAnnotation(uc, version, uuid.New(), uuid.New())

			var backupErr pkg.InternalServerError

			require.ErrorAs(t, err, &backupErr)
			assert.Equal(t, constant.ErrTransactionBackupCacheFailed.Error(), backupErr.Code)
		})
	}
}

// TestCreateAnnotationRejectsAnInternalScopeBalanceBeforeAnyBackupWrite proves the
// use-case path of the scope guard: an annotation over an internal-scope balance is
// refused with the scope error, releases its claim and writes no backup-queue entry.
func TestCreateAnnotationRejectsAnInternalScopeBalanceBeforeAnyBackupWrite(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			balances := annotationLegBalances()
			balances[0].Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}
			reader := &annotationBalanceReader{versionReader: &versionReader{}, balances: balances}
			uc := &UseCase{
				TransactionRedisRepo:        redisRepo,
				TransactionReader:           reader,
				TransactionRepo:             transaction.NewMockRepository(ctrl),
				AppliedTransactionCompleter: &createAppliedTransactionCompleter{},
			}

			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
			redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

			err := createAnnotation(uc, version, uuid.New(), uuid.New())

			var scopeErr pkg.UnprocessableOperationError

			require.ErrorAs(t, err, &scopeErr)
			assert.Equal(t, constant.ErrDirectOperationOnInternalBalance.Error(), scopeErr.Code)
		})
	}
}
