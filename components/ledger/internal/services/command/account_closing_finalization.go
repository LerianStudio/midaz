// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// accountClosingEvictionBatch bounds how many balance blobs one eviction pass
// drops before the attempt checks whether it may continue. The eviction runs under
// the closing marker, so pausing between batches costs nothing: no movement can be
// admitted while it walks.
const accountClosingEvictionBatch = 50

// finalizeAccountClosing records the closing instant and finishes the transition
// under the protection that has been held since before the verification.
//
// The instant is the database's: it comes back from the statement that applied it,
// so two attempts cannot disagree about which one landed and no clock of this
// process ever reaches the column. Nothing here writes a transaction, an operation
// or a balance row — the closing is a state of the account, not a movement, and
// the balances it leaves behind stay readable exactly as they were.
//
// The PostgreSQL write above is authoritative once it succeeds, so the completion
// that follows runs on a bounded context detached from the caller's cancellation:
// a caller that gives up must not leave eviction and the closed marker undone.
func (uc *UseCase) finalizeAccountClosing(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	attempt *accountClosingAttempt,
	states []accountClosingBalanceState,
) (time.Time, error) {
	if err := uc.recordAccountClosingWriteIntent(ctx, span, logger, attempt); err != nil {
		return time.Time{}, err
	}

	closedAt, err := uc.AccountRepo.CloseAccount(ctx, attempt.organizationID, attempt.ledgerID, attempt.accountID)
	if err != nil {
		return time.Time{}, uc.resolveFailedAccountClosingWrite(ctx, span, logger, attempt, err)
	}

	completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), accountClosingCleanupTimeout)
	defer cancel()

	if err := uc.completeAccountClosing(completionCtx, span, logger, attempt, states, closedAt); err != nil {
		return time.Time{}, err
	}

	return closedAt, nil
}

// recordAccountClosingWriteIntent records on the marker that this attempt is about
// to issue its closing write.
//
// It is what lets a later reconciliation tell the two abandoned attempts apart. An
// attempt that never reached this point cannot have closed anything, so its
// protection may be given back; past this point a write may be in flight, and an
// authoritative row reading NULL does not prove it will stay that way. The record
// is written BEFORE the write for exactly that reason.
//
// A failure here leaves the protection releasable: whatever the marker ended up
// carrying, the write below was never issued.
func (uc *UseCase) recordAccountClosingWriteIntent(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	attempt *accountClosingAttempt,
) error {
	recorded, err := uc.TransactionRedisRepo.MarkAccountClosingWriteIssued(ctx, attempt.organizationID, attempt.ledgerID, attempt.accountID, attempt.token)
	if err == nil && recorded {
		return nil
	}

	indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

	if err == nil {
		err = errors.New("the closing marker no longer belongs to this attempt")
	}

	libOpentelemetry.HandleSpanError(span, "Failed to record the account closing write intent", err)
	logger.Log(ctx, libLog.LevelError, "Failed to record the account closing write intent", libLog.Err(err))

	return indeterminate
}

// resolveFailedAccountClosingWrite decides what a failed conditional write means
// for the account and for the protection.
//
// A statement that matched no row is a known outcome and the authoritative row
// says which one it was. An error the server answered with is equally known: it
// did not land. Everything else — a cancelled context, an expired deadline, a lost
// connection — proves nothing, because the commit may have happened with the
// answer lost on the way back. Such an attempt keeps its protection: it is
// resolved by reading the authoritative row later, never by retrying the write.
func (uc *UseCase) resolveFailedAccountClosingWrite(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	attempt *accountClosingAttempt,
	err error,
) error {
	if errors.Is(err, account.ErrAccountCloseNotApplied) {
		return uc.resolveUnappliedAccountClosing(ctx, span, logger, attempt)
	}

	if sqlWriteOutcomeIsKnown(err) {
		libOpentelemetry.HandleSpanError(span, "Failed to record the account closing", err)
		logger.Log(ctx, libLog.LevelError, "Failed to record the account closing", libLog.Err(err))

		return err
	}

	attempt.retain()

	indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

	libOpentelemetry.HandleSpanError(span, "The account closing write left an unknown outcome", err)
	logger.Log(ctx, libLog.LevelError, "The account closing write left an unknown outcome", libLog.Err(err))

	return indeterminate
}

// resolveUnappliedAccountClosing decides what a zero-row conditional write meant.
//
// Absent, soft-deleted and already closed are indistinguishable from the statement
// alone, so the authoritative row is read: a closing instant there is the conflict
// of a repeat, and no row at all is the account being gone from the scope. A read
// that fails leaves the outcome unknown, which is refused rather than guessed.
func (uc *UseCase) resolveUnappliedAccountClosing(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	attempt *accountClosingAttempt,
) error {
	states, err := uc.AccountRepo.ListClosedAtByIDs(ctx, attempt.organizationID, attempt.ledgerID, []uuid.UUID{attempt.accountID})
	if err != nil {
		indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

		libOpentelemetry.HandleSpanError(span, "Failed to resolve the unapplied account closing", err)
		logger.Log(ctx, libLog.LevelError, "Failed to resolve the unapplied account closing", libLog.Err(err))

		return indeterminate
	}

	if closedAt, ok := states[attempt.accountID]; ok && closedAt != nil {
		alreadyClosed := pkg.ValidateBusinessError(constant.ErrAccountAlreadyClosed, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account was already closed", alreadyClosed)

		return alreadyClosed
	}

	notFound := pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account to close is no longer in the scope", notFound)

	return notFound
}

// completeAccountClosing finishes a confirmed closing, in the only order that is
// safe to interrupt.
//
// The cached balances go first, so no load can serve them once the protection is
// gone; the negative cache goes next, so the account reads as closed without a
// query; the ownership follows; and the closing marker leaves last, because it is
// what holds every other path off while the steps above run and the anchor
// through which reconciliation finds an ownership left behind. Each step is
// CONFIRMED before the next: a failure at any of them keeps the marker, and
// whatever of the ownership is still held, in place, which is what lets
// reconciliation resume the same finalization instead of a movement slipping
// through a half-finished one.
//
// The account is closed from the write onwards, so a failure here is reported
// technically without ever undoing the transition — a repeat answers that it is
// already closed, and the instant never changes.
func (uc *UseCase) completeAccountClosing(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	attempt *accountClosingAttempt,
	states []accountClosingBalanceState,
	closedAt time.Time,
) error {
	if err := uc.evictAccountClosingBalances(ctx, attempt, states); err != nil {
		return uc.retainUnfinishedAccountClosing(ctx, span, logger, attempt, "Failed to evict the balances of the closed account", err)
	}

	if err := uc.TransactionRedisRepo.SetAccountClosedMarker(ctx, attempt.organizationID, attempt.ledgerID, attempt.accountID, closedAt); err != nil {
		return uc.retainUnfinishedAccountClosing(ctx, span, logger, attempt, "Failed to install the closed account marker", err)
	}

	if err := attempt.admission.ReleaseConfirmed(ctx); err != nil {
		return uc.retainUnfinishedAccountClosing(ctx, span, logger, attempt, "Failed to release the ownership of the finished closing", err)
	}

	released, err := uc.TransactionRedisRepo.ReleaseAccountClosingAttempt(ctx, attempt.organizationID, attempt.ledgerID, attempt.accountID, attempt.token)
	if err != nil {
		return uc.retainUnfinishedAccountClosing(ctx, span, logger, attempt, "Failed to remove the closing marker of the finished closing", err)
	}

	// A marker that no longer carries this token is a marker this attempt no longer
	// has to remove; the protection it stands for is already gone.
	attempt.markerReleased = true

	if !released {
		logger.Log(ctx, libLog.LevelDebug, "The account closing marker was not owned when the closing finished")
	}

	return nil
}

// evictAccountClosingBalances drops every cached balance of the closed account.
//
// The list is the one the verification proved settled and persisted, so nothing is
// dropped whose state had not reached PostgreSQL. It walks in batches under the
// closing marker, which is what makes pausing safe: no admission can refill what
// it has already dropped. An eviction that fails is returned rather than logged
// away — a blob left behind is a balance a later load could still serve.
func (uc *UseCase) evictAccountClosingBalances(ctx context.Context, attempt *accountClosingAttempt, states []accountClosingBalanceState) error {
	for index, state := range states {
		if index%accountClosingEvictionBatch == 0 {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("evict the balances of the closed account: %w", err)
			}
		}

		key := balanceCacheKeyFor(attempt.organizationID, attempt.ledgerID, state.Persisted)

		if err := uc.TransactionRedisRepo.Del(ctx, key); err != nil {
			return fmt.Errorf("evict the cached balance of the closed account: %w", err)
		}
	}

	return nil
}

// retainUnfinishedAccountClosing keeps the protection of a closing whose
// finalization did not conclude and reports it technically.
//
// The transition itself is not in doubt — the instant is recorded — so nothing is
// undone. What is in doubt is whether the cache still holds state the account may
// no longer serve, and that is exactly the situation the marker exists for.
func (uc *UseCase) retainUnfinishedAccountClosing(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	attempt *accountClosingAttempt,
	message string,
	err error,
) error {
	attempt.retain()

	span.SetAttributes(attribute.Bool("app.account_closing.finalization_retained", true))

	libOpentelemetry.HandleSpanError(span, message, err)
	logger.Log(ctx, libLog.LevelError, message, libLog.Err(err))

	return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
}
