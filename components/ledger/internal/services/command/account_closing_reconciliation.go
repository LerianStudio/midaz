// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
)

const (
	// accountClosingReconcileScanCount is the COUNT hint of one protection page. It
	// is a hint, so a page may come back larger; the walk never drops what it gets.
	accountClosingReconcileScanCount = 200

	// maxAccountClosingReconcilePages bounds one reconciliation pass. Whatever the
	// pass does not reach stays protected and is picked up by the next one: the
	// bound limits the cost of a cycle, never what the reconciliation may conclude.
	maxAccountClosingReconcilePages = 100
)

// AccountClosingReconciliationStats reports one reconciliation pass by outcome.
// The reasons are a fixed set and the counters carry no account, so a metric built
// from them cannot grow a label per account.
type AccountClosingReconciliationStats struct {
	// Scanned is how many closing markers the pass looked at.
	Scanned int
	// Completed is how many confirmed closings it finished.
	Completed int
	// Released is how many aborted attempts it gave back.
	Released int
	// Retained is how many markers it deliberately left in place, because their
	// outcome could not be established.
	Retained int
	// Unreadable is how many keys carried no resolvable scope or attempt.
	Unreadable int
	// Ownerships is how many administrative ownerships were still installed when the
	// pass finished. It is the backlog measure of the coordination, and the pass
	// never releases one it did not resolve through its own closing.
	Ownerships int
}

// ReconcileAccountClosings resolves the closing protection that earlier attempts
// left behind, including across a restart.
//
// Discovery is a paginated scan of the protection namespace, not a callback held
// in memory: a marker only becomes interesting once the attempt that installed it
// is no longer there to finish its own work. For each one the authoritative row
// decides, and only three things can follow. A recorded closing instant is
// finished — the same eviction, negative cache and marker removal the attempt
// itself would have run, with the instant it already has. An attempt that never
// issued its closing write cannot have closed anything, so its protection is given
// back conditionally on that exact phase: if the owner moved on in the meantime
// the conditional removal simply does not apply, and once its marker is gone that
// owner can no longer issue its write at all. Anything else is left alone.
//
// Nothing here reopens an account, rewrites an instant, reapplies a movement or
// releases a protection because time passed.
func (uc *UseCase) ReconcileAccountClosings(ctx context.Context) AccountClosingReconciliationStats {
	var stats AccountClosingReconciliationStats

	if uc.AccountRepo == nil || uc.TransactionRedisRepo == nil {
		return stats
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.reconcile_account_closings")
	defer span.End()

	for cursor, page := uint64(0), 0; page < maxAccountClosingReconcilePages; page++ {
		if ctx.Err() != nil {
			break
		}

		scan, err := uc.TransactionRedisRepo.ScanAccountClosingMarkers(ctx, cursor, accountClosingReconcileScanCount)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan the account closing markers", err)
			logger.Log(ctx, libLog.LevelError, "Failed to scan the account closing markers", libLog.Err(err))

			break
		}

		stats.Unreadable += scan.Unreadable

		for _, scope := range scan.Scopes {
			stats.Scanned++

			uc.reconcileAccountClosing(ctx, logger, scope, &stats)
		}

		cursor = scan.Cursor
		if cursor == 0 {
			break
		}
	}

	uc.countAbandonedAccountOwnerships(ctx, logger, &stats)

	span.SetAttributes(
		attribute.Int("app.account_closing.reconciled_markers", stats.Scanned),
		attribute.Int("app.account_closing.reconciled_completed", stats.Completed),
		attribute.Int("app.account_closing.reconciled_released", stats.Released),
		attribute.Int("app.account_closing.reconciled_retained", stats.Retained),
		attribute.Int("app.account_closing.reconciled_ownership_backlog", stats.Ownerships),
	)

	return stats
}

// reconcileAccountClosing resolves one discovered closing marker.
func (uc *UseCase) reconcileAccountClosing(
	ctx context.Context,
	logger libLog.Logger,
	scope txRedis.AccountProtectionScope,
	stats *AccountClosingReconciliationStats,
) {
	attempt, found, err := uc.TransactionRedisRepo.ReadAccountClosingAttempt(ctx, scope.OrganizationID, scope.LedgerID, scope.AccountID)
	if err != nil {
		stats.Unreadable++

		logger.Log(ctx, libLog.LevelWarn, "Failed to read a closing marker while reconciling", libLog.Err(err))

		return
	}

	if !found {
		// The marker went away between the scan and this read, which is what a
		// finishing attempt does.
		return
	}

	closedAt, err := uc.readReconciledClosingInstant(ctx, scope)
	if err != nil {
		stats.Retained++

		logger.Log(ctx, libLog.LevelWarn, "Failed to read the authoritative closing state while reconciling", libLog.Err(err))

		return
	}

	if closedAt != nil {
		uc.completeReconciledAccountClosing(ctx, logger, scope, attempt, *closedAt, stats)

		return
	}

	if attempt.WriteIssued {
		// The write may have committed with its answer lost, so a NULL column proves
		// nothing. The protection stays until that write is resolved; it is never
		// retried and never released because time passed.
		stats.Retained++

		return
	}

	uc.releaseAbortedAccountClosing(ctx, logger, scope, attempt, stats)
}

// readReconciledClosingInstant reads the authoritative closing state of one
// account. The primary answers, because a closing that committed a moment ago has
// to be visible to the decision that depends on it.
func (uc *UseCase) readReconciledClosingInstant(ctx context.Context, scope txRedis.AccountProtectionScope) (*time.Time, error) {
	states, err := uc.AccountRepo.ListClosedAtByIDs(readrouting.WithPrimaryRead(ctx), scope.OrganizationID, scope.LedgerID, []uuid.UUID{scope.AccountID})
	if err != nil {
		return nil, err
	}

	return states[scope.AccountID], nil
}

// completeReconciledAccountClosing finishes a closing whose instant is recorded.
//
// It runs the same steps the attempt would have, in the same order and with the
// instant the database already holds: the cached balances go first, the negative
// cache next, and the marker last. A step that fails leaves the protection in
// place for the next pass, which is why none of them is best-effort here.
func (uc *UseCase) completeReconciledAccountClosing(
	ctx context.Context,
	logger libLog.Logger,
	scope txRedis.AccountProtectionScope,
	attempt txRedis.AccountClosingAttempt,
	closedAt time.Time,
	stats *AccountClosingReconciliationStats,
) {
	balances, err := uc.BalanceRepo.ListByAccountID(readrouting.WithPrimaryRead(ctx), scope.OrganizationID, scope.LedgerID, scope.AccountID)
	if err != nil {
		stats.Retained++

		logger.Log(ctx, libLog.LevelWarn, "Failed to list the balances of a closed account while reconciling", libLog.Err(err))

		return
	}

	for _, balance := range balances {
		if err := uc.TransactionRedisRepo.Del(ctx, balanceCacheKeyFor(scope.OrganizationID, scope.LedgerID, balance)); err != nil {
			stats.Retained++

			logger.Log(ctx, libLog.LevelWarn, "Failed to evict a balance of a closed account while reconciling", libLog.Err(err))

			return
		}
	}

	if err := uc.TransactionRedisRepo.SetAccountClosedMarker(ctx, scope.OrganizationID, scope.LedgerID, scope.AccountID, closedAt); err != nil {
		stats.Retained++

		logger.Log(ctx, libLog.LevelWarn, "Failed to install the closed marker while reconciling", libLog.Err(err))

		return
	}

	if _, err := uc.TransactionRedisRepo.ReleaseAccountClosingAttempt(ctx, scope.OrganizationID, scope.LedgerID, scope.AccountID, attempt.Token); err != nil {
		stats.Retained++

		logger.Log(ctx, libLog.LevelWarn, "Failed to remove the closing marker of a finished closing while reconciling", libLog.Err(err))

		return
	}

	uc.releaseReconciledOwnership(ctx, logger, scope, attempt.Token)

	stats.Completed++
}

// releaseAbortedAccountClosing gives back the protection of an attempt that never
// issued its closing write.
//
// The removal is conditional on that exact phase, which is what makes it safe
// without knowing whether the owner is still running: an owner that advanced in
// the meantime no longer matches, and one whose marker is removed here can no
// longer record a write intent, so it cannot write either.
func (uc *UseCase) releaseAbortedAccountClosing(
	ctx context.Context,
	logger libLog.Logger,
	scope txRedis.AccountProtectionScope,
	attempt txRedis.AccountClosingAttempt,
	stats *AccountClosingReconciliationStats,
) {
	released, err := uc.TransactionRedisRepo.ReleaseAccountClosingMarker(ctx, scope.OrganizationID, scope.LedgerID, scope.AccountID, attempt.Token)
	if err != nil {
		stats.Retained++

		logger.Log(ctx, libLog.LevelWarn, "Failed to release an aborted closing while reconciling", libLog.Err(err))

		return
	}

	if !released {
		stats.Retained++

		return
	}

	uc.releaseReconciledOwnership(ctx, logger, scope, attempt.Token)

	stats.Released++
}

// releaseReconciledOwnership drops the administrative ownership that belongs to
// the same attempt, conditionally on its token so an ownership another operation
// took afterwards is never touched.
func (uc *UseCase) releaseReconciledOwnership(ctx context.Context, logger libLog.Logger, scope txRedis.AccountProtectionScope, token string) {
	if _, err := uc.TransactionRedisRepo.ReleaseAccountAdminOwnership(ctx, scope.OrganizationID, scope.LedgerID, scope.AccountID, token); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to release the ownership of a reconciled closing", libLog.Err(err))
	}
}

// countAbandonedAccountOwnerships measures the ownerships still installed once the
// closings of this pass were resolved.
//
// What remains belongs to a balance creation, a deletion or a cache-miss admission
// whose result this pass cannot establish — the engine's classification of that
// result decides it, and it is not readable from a key. They are counted as
// backlog and left exactly where they are: releasing one would let a closing
// validate a balance list that work can still change.
func (uc *UseCase) countAbandonedAccountOwnerships(ctx context.Context, logger libLog.Logger, stats *AccountClosingReconciliationStats) {
	for cursor, page := uint64(0), 0; page < maxAccountClosingReconcilePages; page++ {
		if ctx.Err() != nil {
			return
		}

		scan, err := uc.TransactionRedisRepo.ScanAccountAdminOwnerships(ctx, cursor, accountClosingReconcileScanCount)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to scan the account administrative ownerships", libLog.Err(err))

			return
		}

		stats.Unreadable += scan.Unreadable
		stats.Ownerships += len(scan.Scopes)

		cursor = scan.Cursor
		if cursor == 0 {
			return
		}
	}
}
