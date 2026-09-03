// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// BlockAccount marks an account as blocked and drives that state into every
// read model the transactional hot path consults: the account row (source of
// truth), the account_blocked projection on its balances, and the balance cache.
//
// holderPolicy is threaded through to the account read because block/unblock is
// registered on both /v1 and /v2 account routes and the handler serializes the
// returned account: only /v2 may observe the holder columns.
func (uc *UseCase) BlockAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, holderPolicy mmodel.HolderPolicy) (_ *mmodel.Account, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.block_account")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "block_account", start, err)
	}()

	blockedAccount, err := uc.setAccountBlockState(ctx, organizationID, ledgerID, accountID, true, holderPolicy)
	if err != nil {
		return nil, err
	}

	// Emitted last, after every read model converged: no event may claim a state
	// the balances and the cache did not reach.
	uc.emitAccountUpdatedEvent(ctx, span, logger, blockedAccount)

	return blockedAccount, nil
}

// setAccountBlockState is the single state-transition path shared by
// BlockAccount and UnblockAccount. The direction is the only difference between
// them, so the convergence guarantees below are proven once for both.
//
// Sequence, and why the order is load-bearing:
//
//  1. Read the account. A missing account is a 404, decided before anything is
//     written.
//  2. On a BLOCK, add the account to the blocked-accounts index (SADD) BEFORE
//     anything else. That index is what the transactional hot path enforces
//     against, so this is the instant the block becomes effective — every step
//     after it can fail without ever leaving the account free to transact.
//  3. Short-circuit the source-of-truth write when the account already holds the
//     target state — but DO NOT return. The index write, propagation and cache
//     eviction still run. That is what makes a retry after a partial failure
//     converge: a previous attempt may have committed step 4 and died later.
//  4. Write account.blocked, the durable source of truth.
//  5. On an UNBLOCK, remove the account from the index (SREM) AFTER the durable
//     write. The mirrored position is the whole point: enforcement is only
//     lifted once the database backs it, so a failure in between leaves a
//     residual block — restrictive, never permissive.
//  6. Propagate to every balance in one atomic UPDATE (legacy projection, kept
//     in parallel during the strangling).
//  7. Drive the new block state into the balance cache with one atomic,
//     AccountBlocked-only mutation. The cache is write-back — it may hold
//     transactional values not yet synced to PostgreSQL — so the flag is flipped
//     in place rather than evicted, which would drop those in-flight mutations.
//
// Any failure from step 2 onward is returned to the caller, and NOTHING is
// compensated. Rolling the SADD back after a failed step 4 would reopen exactly
// the window step 2 closes, so the index deliberately keeps an entry the durable
// state does not (yet) back. Both directions are idempotent — SADD and SREM are
// natural no-ops on repetition — so the operator's retry is what completes the
// transition. Nothing is confirmed to the operator on a partial write.
//
// Emission is the caller's job, deliberately. BlockAccount and UnblockAccount
// emit account.updated as their last act, so the event still comes after every
// read model converged — including on the short-circuit path, which is the only
// way the operator action reaches the audit stream when a first attempt died
// mid-propagation. UpdateAccount instead folds this transition into the single
// account.updated it already emits for the whole PATCH, so a request that
// changes blocked alongside other fields does not publish the same resource
// twice.
func (uc *UseCase) setAccountBlockState(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, blocked bool, holderPolicy mmodel.HolderPolicy) (*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_account_block_state")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
		attribute.Bool("app.request.blocked", blocked),
	)

	accFound, err := uc.findAccountToBlock(ctx, organizationID, ledgerID, accountID, holderPolicy)
	if err != nil {
		return nil, err
	}

	// External accounts are the traceability anchor for value crossing the ledger
	// boundary; blocking one would strand every transaction that settles through
	// it. Same guard, same code (0074) and same position as UpdateAccount and
	// DeleteAccountByID use — before anything is written, propagated or evicted.
	if accFound.ID == accountID.String() && accFound.Type == "external" {
		err = pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount)

		logger.Log(ctx, libLog.LevelWarn, "Rejected block state change on external account",
			libLog.String("account_id", accountID.String()),
			libLog.Bool("blocked", blocked))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rejected block state change on external account", err)

		return nil, err
	}

	// Enforcement first on the way IN. From here on the account cannot transact,
	// whatever happens to the durable write below.
	if blocked {
		if err := uc.setBlockedAccountsIndexEntry(ctx, organizationID, ledgerID, accountID, true); err != nil {
			return nil, err
		}
	}

	// A nil Blocked is NOT equal to false: the column was never written on that
	// row, so the source of truth still has to be made explicit.
	alreadyInTargetState := accFound.Blocked != nil && *accFound.Blocked == blocked

	updatedAt := accFound.UpdatedAt

	if alreadyInTargetState {
		span.SetAttributes(attribute.Bool("app.block.no_op", true))
		logger.Log(ctx, libLog.LevelInfo, "Account already in target block state, re-propagating for convergence",
			libLog.String("account_id", accountID.String()),
			libLog.Bool("blocked", blocked))
	} else {
		updatedAt, err = uc.persistAccountBlockState(ctx, organizationID, ledgerID, accountID, blocked)
		if err != nil {
			return nil, err
		}
	}

	// Enforcement last on the way OUT: the block is only lifted once the durable
	// state says so. A failure here leaves the account blocked in the index —
	// the operator sees the error and retries, and nothing transacted meanwhile.
	if !blocked {
		if err := uc.setBlockedAccountsIndexEntry(ctx, organizationID, ledgerID, accountID, false); err != nil {
			return nil, err
		}
	}

	if err := uc.propagateBlockStateToBalances(ctx, organizationID, ledgerID, accountID, blocked); err != nil {
		return nil, err
	}

	if err := uc.invalidateAccountBalanceCache(ctx, organizationID, ledgerID, accountID, blocked); err != nil {
		return nil, err
	}

	// AccountRepo.Update returns an input-derived record with bogus identity
	// fields, so the post-state is rebuilt in-memory from the pre-state exactly
	// as UpdateAccount does. On the no-op path there was no Update at all and
	// the pre-state timestamp is the persisted one.
	return mergePatchAccount(accFound, &mmodel.Account{Blocked: &blocked}, updatedAt), nil
}

// findAccountToBlock reads the account that is about to change state and turns
// both "no row" shapes the repository can return into the 404 business error.
func (uc *UseCase) findAccountToBlock(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, holderPolicy mmodel.HolderPolicy) (*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_account_block_state.find_account")
	defer span.End()

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, accountID, holderPolicy)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find account by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to find account by id", libLog.Err(err))

		return nil, err
	}

	if accFound == nil {
		err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

		logger.Log(ctx, libLog.LevelWarn, "Account ID not found on block state change",
			libLog.String("account_id", accountID.String()))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account not found for block state change", err)

		return nil, err
	}

	return accFound, nil
}

// setBlockedAccountsIndexEntry drives the blocked-accounts SET, the index the
// transactional hot path enforces against.
//
// It is called unconditionally in both directions — including on the
// short-circuit path where the account already holds the target state — because
// SADD and SREM are natural no-ops on repetition, and re-asserting the entry is
// what sweeps a residue left by an earlier attempt that died mid-way.
//
// A failure is returned, never swallowed and never compensated. On a block the
// entry may already be in place with no durable write behind it; that is the
// deliberate fail-closed direction, and the operator's retry resolves it.
func (uc *UseCase) setBlockedAccountsIndexEntry(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, blocked bool) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_account_block_state.update_blocked_index")
	defer span.End()

	span.SetAttributes(attribute.Bool("app.request.blocked", blocked))

	var err error
	if blocked {
		err = uc.TransactionRedisRepo.AddBlockedAccount(ctx, organizationID, ledgerID, accountID)
	} else {
		err = uc.TransactionRedisRepo.RemoveBlockedAccount(ctx, organizationID, ledgerID, accountID)
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update blocked accounts index", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update blocked accounts index",
			libLog.String("account_id", accountID.String()),
			libLog.Bool("blocked", blocked),
			libLog.Err(err))

		return err
	}

	return nil
}

// persistAccountBlockState writes the new state to the source of truth and
// returns the persisted UpdatedAt so the emitted event carries the timestamp the
// row now holds.
func (uc *UseCase) persistAccountBlockState(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, blocked bool) (time.Time, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_account_block_state.update_account")
	defer span.End()

	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, nil, accountID, &mmodel.Account{Blocked: &blocked})
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

			logger.Log(ctx, libLog.LevelWarn, "Account ID not found on block state update",
				libLog.String("account_id", accountID.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account block state", err)

			return time.Time{}, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to update account block state", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update account block state", libLog.Err(err))

		return time.Time{}, err
	}

	return accountUpdated.UpdatedAt, nil
}

// propagateBlockStateToBalances drives the account_blocked projection on every
// balance of the account. A failure here is returned, never swallowed: the
// operator must see that account and balances disagree so the call is retried.
func (uc *UseCase) propagateBlockStateToBalances(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, blocked bool) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_account_block_state.propagate_balances")
	defer span.End()

	if err := uc.BalanceRepo.UpdateAccountBlockedByAccountID(ctx, organizationID, ledgerID, accountID, blocked); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to propagate account block state to balances", err)
		logger.Log(ctx, libLog.LevelError, "Failed to propagate account block state to balances",
			libLog.String("account_id", accountID.String()),
			libLog.Bool("blocked", blocked),
			libLog.Err(err))

		return err
	}

	return nil
}

// invalidateAccountBalanceCache drives the new block state into the cached
// balances of the account with a single atomic, AccountBlocked-only mutation, so
// no reader can observe a mixture of stale and fresh keys.
//
// The balance cache is a write-back store, not a plain read cache: the atomic
// Lua path mutates the authoritative Available / OnHold / Version /
// OverdraftUsed inside each cached BalanceRedis blob and may be AHEAD of
// PostgreSQL until the sync worker flushes it. Deleting the keys here would drop
// those in-flight, not-yet-synced mutations (and their scheduled sync), and the
// next read would repopulate STALE balances from PostgreSQL. So instead of
// evicting, this flips ONLY the AccountBlocked projection in place via
// SetAccountBlockedMany — mirroring how UpdateBalanceCacheSettings mutates the
// settings fields — leaving every transactional field, the backup and the sync
// schedule intact.
//
// Unlike evictBalanceCaches — which runs after an already-committed delete and
// therefore only warns — a failure here is returned. The propagated rows are
// durable but the hot path reads the cache, so a surviving stale flag means a
// blocked account still transacts. The operator has to know, and the retry is
// safe because both the UPDATE and this in-place mutation are idempotent.
func (uc *UseCase) invalidateAccountBalanceCache(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, blocked bool) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_account_block_state.invalidate_cache")
	defer span.End()

	balances, err := uc.BalanceRepo.ListByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list balances for cache invalidation", err)
		logger.Log(ctx, libLog.LevelError, "Failed to list balances for cache invalidation",
			libLog.String("account_id", accountID.String()),
			libLog.Err(err))

		return err
	}

	if len(balances) == 0 {
		return nil
	}

	cacheKeys := make([]string, 0, len(balances))
	for _, b := range balances {
		cacheKeys = append(cacheKeys, balanceCacheKeyFor(organizationID, ledgerID, b))
	}

	if err := uc.TransactionRedisRepo.SetAccountBlockedMany(ctx, cacheKeys, blocked); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to invalidate balance cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to invalidate balance cache after block state change",
			libLog.String("account_id", accountID.String()),
			libLog.Int("keys", len(cacheKeys)),
			libLog.Bool("blocked", blocked),
			libLog.Err(err))

		return err
	}

	return nil
}
