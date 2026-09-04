// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libConstants "github.com/LerianStudio/lib-commons/v7/commons/constants"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// TestCancelTransaction_WriteBehindMiss_FallbackLoadsOperations pins the money-path
// fix for the two-phase overdraft cancel. The write-behind cache is cleared once the
// create persists, so a later /cancel misses it and falls through to the database.
// That fallback MUST read the transaction WITH its operations
// (GetTransactionWithOperationsByID / FindWithOperations): commitOrCancelTransaction's
// annotateCanceledOverdraftAmounts step reads tran.Operations to size the overdraft
// deficit, and a row-only read (GetTransactionByID / Find) leaves Operations empty so
// the cancel restores the whole hold to available instead of only the non-overdraft
// portion.
//
// The transaction is APPROVED here so the commit/cancel state machine short-circuits
// at its not-pending guard right after the fetch — that keeps the assertion on the
// fetch selection alone. Find is deliberately NOT stubbed, so the gomock controller
// fails the test if the fallback still calls the row-only read. The downstream
// consumption (operations -> OverdraftAmount -> Lua -> Available) is covered by
// TestAnnotateCanceledOverdraftAmounts_UsesPendingCompanionAmount and the redis Lua
// integration tests (TestIntegration_Overdraft_PendingLegacyCancelRestoresCompanion).
func TestCancelTransaction_WriteBehindMiss_FallbackLoadsOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	amount := decimal.NewFromInt(100)
	txBody := mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "BRL",
			Value: amount,
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{AccountAlias: "@source"}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{AccountAlias: "@dest"}},
			},
		},
	}

	// A transaction that overflowed a 50-available balance into a 50 overdraft hold:
	// the ONHOLD companion leg on the overdraft balance is the deficit annotate reads.
	deficit := decimal.NewFromInt(50)
	tranWithOps := &transaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
		Amount:         &amount,
		// APPROVED (not PENDING) so the state machine returns at the not-pending guard.
		Status: transaction.Status{Code: cn.APPROVED},
		Body:   txBody,
		Operations: []*operation.Operation{
			{
				Type:         libConstants.DEBIT,
				BalanceKey:   cn.OverdraftBalanceKey,
				AccountAlias: "@source",
				Amount:       operation.Amount{Value: &deficit},
			},
		},
	}

	// Write-behind cache miss forces the database fallback.
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("cache miss")).
		AnyTimes()

	// The fallback MUST use the with-operations read.
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tranWithOps, nil).
		Times(1)

	// Find (row-only read) MUST NOT be reached: any call fails the test.

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	// Lock acquired, then released on the not-pending guard error path.
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).
		Times(1)
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}
	commandUC := &command.UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: commandUC}

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost,
		humaTransactionURL(orgID, ledgerID, "/"+transactionID.String()+"/cancel"),
		nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 409, resp.StatusCode, "expected HTTP 409 for non-PENDING status after the with-operations fallback")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(body, &errResp), "error response should be valid JSON")
	assert.Equal(t, cn.ErrCommitTransactionNotPending.Error(), errResp["code"],
		"expected error code 0099 (ErrCommitTransactionNotPending)")
}

// TestCancelTransaction_WriteBehindMiss_NonexistentTransaction_Returns404 guards the
// with-operations fallback against its INNER JOIN semantics: FindWithOperations returns
// an empty transaction (no error) when it matches no operation rows — a nonexistent
// transaction, or one with no operations. Without the empty-value guard,
// commitOrCancelTransaction would parse an empty organization id and panic. The guard
// falls back to the row-only read, which reports not-found, so the response stays a
// clean 404.
func TestCancelTransaction_WriteBehindMiss_NonexistentTransaction_Returns404(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Write-behind cache miss forces the database fallback.
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("cache miss")).
		AnyTimes()

	// INNER JOIN matched nothing: empty transaction, no error.
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(&transaction.Transaction{}, nil).
		Times(1)

	// Metadata lookup runs against the empty (non-nil) transaction.
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	// The guard falls back to the row-only read, which surfaces the not-found error.
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, pkg.EntityNotFoundError{
			EntityType: "Transaction",
			Code:       cn.ErrEntityNotFound.Error(),
			Title:      "Entity Not Found",
			Message:    "Transaction not found",
		}).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost,
		humaTransactionURL(orgID, ledgerID, "/"+transactionID.String()+"/cancel"),
		nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode, "an empty with-operations result must degrade to a not-found, never a panic")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(body, &errResp), "error response should be valid JSON")
	assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"], "expected entity-not-found code")
}

// TestCancelTransaction_WriteBehindMiss_RowOnlyFallbackReturnsRealTransaction pins the
// OTHER branch of the empty-value guard: FindWithOperations returns an empty transaction
// (its INNER JOIN matched no operation rows), but the transaction itself exists and simply
// has no operations. The guard must fall back to the row-only read, which returns the REAL
// row — not a not-found. Here that row is APPROVED, so the state machine short-circuits at
// its not-pending guard and answers 409, proving the fallback carried a live transaction
// (not a nil/empty value) into commitOrCancelTransaction.
func TestCancelTransaction_WriteBehindMiss_RowOnlyFallbackReturnsRealTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	amount := decimal.NewFromInt(100)

	// A REAL, operation-less transaction: valid ids and APPROVED status so
	// commitOrCancelTransaction parses the ids and returns at the not-pending guard.
	tranRowOnly := &transaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
		Amount:         &amount,
		Status:         transaction.Status{Code: cn.APPROVED},
	}

	// Write-behind cache miss forces the database fallback.
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("cache miss")).
		AnyTimes()

	// INNER JOIN matched no operation rows: empty transaction, no error — triggers the guard.
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(&transaction.Transaction{}, nil).
		Times(1)

	// The guard falls back to the row-only read, which returns the REAL operation-less row.
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tranRowOnly, nil).
		Times(1)

	// Metadata lookup runs on both reads (with-operations empty result + row-only result).
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(2)

	// Lock acquired, then released on the not-pending guard error path.
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).
		Times(1)
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}
	commandUC := &command.UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: commandUC}

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost,
		humaTransactionURL(orgID, ledgerID, "/"+transactionID.String()+"/cancel"),
		nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 409, resp.StatusCode,
		"a real operation-less transaction from the row-only fallback must reach the not-pending guard")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(body, &errResp), "error response should be valid JSON")
	assert.Equal(t, cn.ErrCommitTransactionNotPending.Error(), errResp["code"],
		"expected error code 0099 (ErrCommitTransactionNotPending)")
}
