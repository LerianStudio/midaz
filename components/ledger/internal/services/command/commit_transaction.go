// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// PendingTransitionInput names the PENDING transaction a commit or a cancel acts
// on. The target status is not carried here: it is fixed by the use case the
// transport binds, so a shell cannot commit through the cancel entry.
type PendingTransitionInput struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	TransactionID  uuid.UUID

	// AccountBlockExceptionID is the single-use account-block exception the
	// commit presented, or nil when it presented none. Only the /v2 commit
	// carries a body that can name one; cancel accepts no grant on any contract,
	// because a cancel is never barred by an account block to begin with.
	AccountBlockExceptionID *uuid.UUID
}

// CommitTransactionV1 approves a PENDING transaction under the /v1 contract, frozen
// at what /v1 shipped with: no tracer reservation lifecycle. A client integrated
// against it must not acquire a reservation rejection, or a confirm it never asked
// for, from a version upgrade — so this pipeline references no reservation seam at
// all.
func (uc *UseCase) CommitTransactionV1(ctx context.Context, in PendingTransitionInput) (*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.commit_transaction_v1")
	defer span.End()

	tran, err := uc.loadPendingTransaction(ctx, span, in)
	if err != nil {
		return nil, err
	}

	return uc.transitionPendingV1(ctx, &pendingTransitionRun{
		organizationID: in.OrganizationID,
		ledgerID:       in.LedgerID,
		tran:           tran,
		status:         constant.APPROVED,
	})
}

// CancelTransactionV1 cancels a PENDING transaction under the /v1 contract. Same
// frozen surface as CommitTransactionV1: no reservation is released, because the
// /v1 create never held one.
func (uc *UseCase) CancelTransactionV1(ctx context.Context, in PendingTransitionInput) (*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.cancel_transaction_v1")
	defer span.End()

	tran, err := uc.loadPendingTransaction(ctx, span, in)
	if err != nil {
		return nil, err
	}

	return uc.transitionPendingV1(ctx, &pendingTransitionRun{
		organizationID: in.OrganizationID,
		ledgerID:       in.LedgerID,
		tran:           tran,
		status:         constant.CANCELED,
	})
}

// CommitTransactionV2 approves a PENDING transaction under the /v2 contract, which
// includes the tracer reservation lifecycle: the create-pending reserve is confirmed
// by transaction id once the balances have moved.
func (uc *UseCase) CommitTransactionV2(ctx context.Context, in PendingTransitionInput) (*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.commit_transaction_v2")
	defer span.End()

	tran, err := uc.loadPendingTransaction(ctx, span, in)
	if err != nil {
		return nil, err
	}

	return uc.transitionPendingV2(ctx, &pendingTransitionRun{
		organizationID: in.OrganizationID,
		ledgerID:       in.LedgerID,
		tran:           tran,
		status:         constant.APPROVED,

		accountBlockExceptionID: in.AccountBlockExceptionID,
	})
}

// CancelTransactionV2 cancels a PENDING transaction under the /v2 contract, releasing
// the reservations the create-pending held.
func (uc *UseCase) CancelTransactionV2(ctx context.Context, in PendingTransitionInput) (*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.cancel_transaction_v2")
	defer span.End()

	tran, err := uc.loadPendingTransaction(ctx, span, in)
	if err != nil {
		return nil, err
	}

	return uc.transitionPendingV2(ctx, &pendingTransitionRun{
		organizationID: in.OrganizationID,
		ledgerID:       in.LedgerID,
		tran:           tran,
		status:         constant.CANCELED,
	})
}

// transitionPendingV1 is the /v1 state-transition pipeline: lock, prepare, commit the
// balances, finalize. It names no reservation seam.
func (uc *UseCase) transitionPendingV1(ctx context.Context, run *pendingTransitionRun) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.transition_pending_transaction")
	defer span.End()

	unlock, err := uc.lockPendingTransaction(ctx, span, logger, run)
	if err != nil {
		return nil, err
	}

	if uc.Engine != nil {
		tran, err := uc.transitionPendingWithEngine(ctx, span, logger, run, unlock, false)
		if err != nil {
			recordCommandError(ctx, span, logger, "Failed to transition transaction with engine", err)
		}

		return tran, err
	}

	if err := uc.preparePendingTransition(ctx, span, logger, run, unlock); err != nil {
		return nil, err
	}

	if err := uc.commitPendingBalances(ctx, span, logger, run, unlock); err != nil {
		return nil, err
	}

	return uc.finalizePendingTransition(ctx, span, logger, run, unlock)
}

// transitionPendingV2 is the /v2 state-transition pipeline: the /v1 sequence plus
// reservation phase two (F3-T15, PENDING lifecycle). The PENDING create path reserved
// capacity but deferred the confirm/release to this state transition; /commit and
// /cancel carry only the transaction id, so the tracer is addressed by transaction id
// and flips every RESERVED reservation the transaction holds. Non-blocking: a transport
// failure never fails the request; it is retried off the request path instead. The
// long-lived TTL hint set at create-pending keeps these reservations alive until this
// transition.
func (uc *UseCase) transitionPendingV2(ctx context.Context, run *pendingTransitionRun) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.transition_pending_transaction")
	defer span.End()

	unlock, err := uc.lockPendingTransaction(ctx, span, logger, run)
	if err != nil {
		return nil, err
	}

	if uc.Engine != nil {
		tran, err := uc.transitionPendingWithEngine(ctx, span, logger, run, unlock, true)
		if err != nil {
			recordCommandError(ctx, span, logger, "Failed to transition transaction with engine", err)
		}

		return tran, err
	}

	if err := uc.preparePendingTransition(ctx, span, logger, run, unlock); err != nil {
		return nil, err
	}

	// Account-block exception: a pending created before its source account was
	// blocked is only released by a grant presented on the commit, and only the
	// /v2 commit carries one. transitionPendingV1 names this resolver nowhere.
	// An identifier with no live key rejects here, before any balance moves —
	// the lock is released, and the pending is left untouched for a retry.
	run.accountBlockExceptionGrant, err = uc.resolveAccountBlockExceptionGrant(ctx, span, logger,
		run.organizationID, run.ledgerID, run.accountBlockExceptionID)
	if err != nil {
		unlock()

		return nil, err
	}

	if err := uc.commitPendingBalances(ctx, span, logger, run, unlock); err != nil {
		return nil, err
	}

	identity := run.reservationIdentity()

	switch run.status {
	case constant.APPROVED:
		uc.confirmReservationsByTransaction(ctx, span, logger, run.ledgerSettings.Tracer, identity, run.honoredTracerSkip)
	case constant.CANCELED:
		uc.releaseReservationsByTransaction(ctx, span, logger, run.ledgerSettings.Tracer, identity, run.honoredTracerSkip)
	}

	return uc.finalizePendingTransition(ctx, span, logger, run, unlock)
}
