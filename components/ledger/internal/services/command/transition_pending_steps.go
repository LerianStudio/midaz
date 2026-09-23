// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// loadPendingTransaction resolves the transaction the transition acts on, reading
// the write-behind cache first and falling back to the database.
func (uc *UseCase) loadPendingTransaction(ctx context.Context, span trace.Span, in PendingTransitionInput) (*transaction.Transaction, error) {
	tran, err := uc.TransactionReader.GetWriteBehindTransaction(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
	if err != nil {
		// Load the operations with the transaction: cancel needs them to unwind an
		// overdraft hold. The write-behind cache is cleared once the create persists,
		// so this fallback carries the transaction into the engine transition.
		tran, err = uc.TransactionReader.GetTransactionWithOperationsByID(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
		if err != nil {
			spanattr.HandleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

			return nil, err
		}

		// FindWithOperations joins on operations, so a transaction with no rows comes
		// back as an empty value with no error. Fall back to the row-only read.
		if tran == nil || tran.ID == "" {
			tran, err = uc.TransactionReader.GetTransactionByID(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
			if err != nil {
				spanattr.HandleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

				return nil, err
			}
		}
	}

	return tran, nil
}

// lockPendingTransaction claims the per-transaction Redis lock that serialises
// concurrent commit/cancel attempts. It returns the unlock closure used by engine
// preparation and confirmed pre-commit failures; a lock already held is a 422.
func (uc *UseCase) lockPendingTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, run *pendingTransitionRun) (func(), error) {
	lockPendingTransactionKey := utils.PendingTransactionLockKey(run.organizationID, run.ledgerID, run.tran.ID)

	ttl := time.Duration(300)

	success, err := uc.TransactionRedisRepo.SetNX(ctx, lockPendingTransactionKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to set pending transaction lock on redis", libLog.Err(err))

		return nil, err
	}

	if !success {
		err := pkg.ValidateBusinessError(constant.ErrPendingTransactionLocked, "ValidateTransactionNotPending")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is locked", err)
		logger.Log(ctx, libLog.LevelWarn, "Transaction is locked", libLog.String("transaction_id", run.tran.ID), libLog.Err(err))

		return nil, err
	}

	deleteLockOnError := func() {
		if delErr := uc.TransactionRedisRepo.Del(ctx, lockPendingTransactionKey); delErr != nil {
			recordCommandError(ctx, span, logger, "Failed to delete pending transaction lock", delErr)
		}
	}

	return deleteLockOnError, nil
}
