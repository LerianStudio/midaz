// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// loadPendingTransaction resolves the transaction the transition acts on. An
// asynchronous create answers before its projection reaches PostgreSQL, so the
// load must reach the sources that can name a transaction still in that window.
func (uc *UseCase) loadPendingTransaction(ctx context.Context, span trace.Span, in PendingTransitionInput) (*transaction.Transaction, error) {
	tran, err := uc.loadLifecycleTransaction(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		return nil, err
	}

	return tran, nil
}

// pendingTransactionUnlockTimeout bounds the lock release, which runs detached
// from the request so a cancelled or timed-out request still frees the lock.
const pendingTransactionUnlockTimeout = 5 * time.Second

// lockPendingTransaction claims the per-transaction Redis lock that serialises
// concurrent commit/cancel attempts; a lock already held is a 409. It only
// serialises requests: the engine's lifecycle guard is what keeps a transition
// from applying twice, so releasing the lock never re-opens a transition that
// already applied. The returned closure releases it. The lock holds a token of
// its own, so a release that outlives the TTL never removes a lock another
// request acquired after it expired.
func (uc *UseCase) lockPendingTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, run *pendingTransitionRun) (func(), error) {
	lockPendingTransactionKey := utils.PendingTransactionLockKey(run.organizationID, run.ledgerID, run.tran.ID)

	ttl := time.Duration(300)

	ownerToken := uuid.NewString()

	success, err := uc.TransactionRedisRepo.SetNX(ctx, lockPendingTransactionKey, ownerToken, ttl)
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

	unlock := func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pendingTransactionUnlockTimeout)
		defer cancel()

		// false means the lock already expired, possibly re-acquired by another
		// request; that lock is not this request's to release.
		if _, delErr := uc.TransactionRedisRepo.DeleteIfValue(releaseCtx, lockPendingTransactionKey, ownerToken); delErr != nil {
			recordCommandError(ctx, span, logger, "Failed to delete pending transaction lock", delErr)
		}
	}

	return unlock, nil
}
