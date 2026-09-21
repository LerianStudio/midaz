// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// accountClosingBalanceState pairs one persisted balance row of the account with
// the live cached state of the same balance.
//
// Live is nil when the transaction cache holds nothing for that balance. That is
// an absence of cached state, never a proof that the balance is settled: the
// persistence check answers it from the operation trail instead.
type accountClosingBalanceState struct {
	Persisted *mmodel.Balance
	Live      *mmodel.Balance
}

// effective returns the state that decides the monetary components. Live cached
// money always wins over the row, which is written behind it by the sync worker.
func (state accountClosingBalanceState) effective() *mmodel.Balance {
	if state.Live != nil {
		return state.Live
	}

	return state.Persisted
}

// loadAccountClosingBalanceStates reads every balance of the account from the
// PRIMARY and pairs each row with the live cached state of the same balance.
//
// The whole list is read: default, additional and internal balances alike, with
// no pagination that could hide one of them. The cache is only READ — nothing is
// warmed, rewritten or evicted, and no administrative ownership is taken, because
// the closing already holds its own over this account.
//
// A cache read that fails is not an absence: it leaves the state of that balance
// unknown, so the closing refuses instead of treating the balance as uncached.
func (uc *UseCase) loadAccountClosingBalanceStates(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]accountClosingBalanceState, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load account closing balance states: %w", err)
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.load_account_closing_balance_states")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	balances, err := uc.BalanceRepo.ListByAccountID(readrouting.WithPrimaryRead(ctx), organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list the account balances for closing", err)
		logger.Log(ctx, libLog.LevelError, "Failed to list the account balances for closing", libLog.Err(err))

		// A store that cannot answer leaves the balance list unknown, which is the
		// same refusal as a cache that cannot answer it: the closing state could not
		// be established. The driver's own error stays on the span and in the log and
		// never reaches the caller, whose envelope carries the sentinel instead.
		return nil, pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	states := make([]accountClosingBalanceState, 0, len(balances))

	for _, balance := range balances {
		live, err := uc.readLiveClosingBalance(ctx, organizationID, ledgerID, balance)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to read the live balance state for closing", err)
			logger.Log(ctx, libLog.LevelError, "Failed to read the live balance state for closing",
				libLog.String("balance_id", balance.ID), libLog.Err(err))

			return nil, pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
		}

		states = append(states, accountClosingBalanceState{Persisted: balance, Live: live})
	}

	span.SetAttributes(attribute.Int("app.account_closing.balances_read", len(states)))

	return states, nil
}

// readLiveClosingBalance reads one cached balance. A cache miss returns (nil, nil);
// every other failure is returned, and a cached blob describing another balance is
// a failure of its own — evidence that cannot be attributed proves nothing.
func (uc *UseCase) readLiveClosingBalance(ctx context.Context, organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) (*mmodel.Balance, error) {
	live, err := uc.TransactionRedisRepo.ListBalanceByKey(ctx, organizationID, ledgerID, balance.Alias+"#"+balance.Key)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}

		return nil, err
	}

	if live == nil {
		return nil, nil
	}

	if live.ID != balance.ID || live.AccountID != balance.AccountID {
		return nil, fmt.Errorf("cached balance %q does not describe the persisted balance %q", live.ID, balance.ID)
	}

	return live, nil
}

// verifyAccountClosingBalancesZeroed refuses the closing while any component of
// any balance still holds value.
//
// Available, OnHold and OverdraftUsed are checked one by one, on every balance:
// nothing is compensated across components or across balances, and no residual is
// rounded away. An overdraft LIMIT is not inspected at all — an unused limit is a
// permission to owe, not a debt, so a balance that never used it closes.
func verifyAccountClosingBalancesZeroed(states []accountClosingBalanceState) error {
	for _, state := range states {
		balance := state.effective()

		if !balance.Available.IsZero() || !balance.OnHold.IsZero() || !balance.OverdraftUsed.IsZero() {
			return pkg.ValidateBusinessError(constant.ErrAccountBalanceNotZero, constant.EntityAccount)
		}
	}

	return nil
}

// verifyAccountClosingBalancePersistence proves that what the live state holds has
// reached PostgreSQL before the closing reads the row as final.
//
// A cached balance ahead of its row is work the sync worker still owes, so the
// closing is refused temporarily: the workers converge on their own, and the
// closing never syncs, overwrites or evicts anything to help them along. A cached
// balance BEHIND its row, or one that disagrees with it at the same version, is
// evidence that cannot be reconciled here, and that is refused as indeterminate
// rather than passed over.
//
// Without a cached state the operation trail answers instead: the high-water mark
// records the state the last persisted movement left behind, so a row behind it is
// a row the sync worker has not caught up with. A mark that carries no state to
// compare against is inconclusive, never a silent pass.
//
// The whole verification is a read under the closing's own ownership: it acquires
// no further admission, warms no balance into the cache and deletes nothing.
func (uc *UseCase) verifyAccountClosingBalancePersistence(ctx context.Context, organizationID, ledgerID uuid.UUID, states []accountClosingBalanceState) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify account closing balance persistence: %w", err)
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.verify_account_closing_balance_persistence")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.account_closing.balance_states_count", len(states)),
	)

	uncached := make([]accountClosingBalanceState, 0, len(states))

	for _, state := range states {
		if state.Live == nil {
			uncached = append(uncached, state)

			continue
		}

		if err := verifyLiveBalanceIsPersisted(state); err != nil {
			recordAccountClosingPersistenceRefusal(ctx, span, logger, err)

			return err
		}
	}

	return uc.verifyUncachedBalancesArePersisted(ctx, span, organizationID, ledgerID, uncached)
}

// verifyLiveBalanceIsPersisted compares one cached balance against its row.
func verifyLiveBalanceIsPersisted(state accountClosingBalanceState) error {
	live, persisted := state.Live, state.Persisted

	if live.Version > persisted.Version {
		return pkg.ValidateBusinessError(constant.ErrAccountClosingPersistencePending, constant.EntityAccount)
	}

	if live.Version < persisted.Version {
		return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	if !live.Available.Equal(persisted.Available) || !live.OnHold.Equal(persisted.OnHold) || !live.OverdraftUsed.Equal(persisted.OverdraftUsed) {
		return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	return nil
}

// verifyUncachedBalancesArePersisted answers the balances the cache does not hold
// from the operation trail, through the same high-water-mark read the cache-miss
// load already uses.
func (uc *UseCase) verifyUncachedBalancesArePersisted(
	ctx context.Context,
	span trace.Span,
	organizationID, ledgerID uuid.UUID,
	states []accountClosingBalanceState,
) error {
	if len(states) == 0 {
		return nil
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	refs := make([]operation.BalanceHWMRef, 0, len(states))

	for _, state := range states {
		accountID, err := uuid.Parse(state.Persisted.AccountID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid account ID on balance", err)
			logger.Log(ctx, libLog.LevelError, "Invalid account ID on balance", libLog.String("balance_id", state.Persisted.ID), libLog.Err(err))

			return err
		}

		balanceID, err := uuid.Parse(state.Persisted.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid balance ID on balance", err)
			logger.Log(ctx, libLog.LevelError, "Invalid balance ID on balance", libLog.String("balance_id", state.Persisted.ID), libLog.Err(err))

			return err
		}

		refs = append(refs, operation.BalanceHWMRef{AccountID: accountID, BalanceID: balanceID})
	}

	highWaterMarks, err := uc.OperationRepo.ListLatestByBalances(ctx, organizationID, ledgerID, refs)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to load balance high-water marks for closing", err)
		logger.Log(ctx, libLog.LevelError, "Failed to load balance high-water marks for closing", libLog.Err(err))

		return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	for _, state := range states {
		if err := verifyBalanceMatchesHighWaterMark(state.Persisted, highWaterMarks[state.Persisted.ID]); err != nil {
			recordAccountClosingPersistenceRefusal(ctx, span, logger, err)

			return err
		}
	}

	return nil
}

// verifyBalanceMatchesHighWaterMark compares one row with the operation that holds
// its high-water mark. No mark at all is the shape of a balance that never moved,
// and it passes; a mark ahead of the row is persistence still owed; a mark that
// cannot be compared is inconclusive.
func verifyBalanceMatchesHighWaterMark(balance *mmodel.Balance, hwm *operation.Operation) error {
	if hwm == nil {
		return nil
	}

	if hwm.BalanceAfter.Version == nil || hwm.BalanceAfter.Available == nil || hwm.BalanceAfter.OnHold == nil {
		return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	if *hwm.BalanceAfter.Version > balance.Version {
		return pkg.ValidateBusinessError(constant.ErrAccountClosingPersistencePending, constant.EntityAccount)
	}

	if *hwm.BalanceAfter.Version < balance.Version {
		// A completion or a recovery that landed after the last operation leaves the
		// row ahead of the trail. That is persistence having finished, not a fork.
		return nil
	}

	if !hwm.BalanceAfter.Available.Equal(balance.Available) || !hwm.BalanceAfter.OnHold.Equal(balance.OnHold) {
		return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	return verifyBalanceMatchesOverdraftSnapshot(balance, hwm)
}

// verifyBalanceMatchesOverdraftSnapshot compares the row's debt with the snapshot
// the high-water-mark operation carries. The overdraft companion is excluded: the
// operations of a companion mirror the DEFAULT balance's overdraft snapshot, so
// that value describes another balance and comparing it would fail every time.
func verifyBalanceMatchesOverdraftSnapshot(balance *mmodel.Balance, hwm *operation.Operation) error {
	if balance.Key == constant.OverdraftBalanceKey {
		return nil
	}

	overdraftUsed, err := decimal.NewFromString(hwm.Snapshot.OverdraftUsedAfter)
	if err != nil || !overdraftUsed.Equal(balance.OverdraftUsed) {
		return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	return nil
}

// recordAccountClosingPersistenceRefusal records a refusal on the span by its
// class: a temporary conflict is a business outcome of the verification and keeps
// the span green, while an inconclusive state is a technical refusal served as
// 503 and flips it red.
func recordAccountClosingPersistenceRefusal(ctx context.Context, span trace.Span, logger libLog.Logger, err error) {
	const message = "Refused to close the account before its persistence concluded"

	if isAccountClosingIndeterminate(err) {
		libOpentelemetry.HandleSpanError(span, message, err)
		logger.Log(ctx, libLog.LevelError, message, libLog.Err(err))

		return
	}

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, message, err)
	logger.Log(ctx, libLog.LevelWarn, message, libLog.Err(err))
}

// isAccountClosingIndeterminate reports whether a refusal is the indeterminate
// one. pkg.ValidateBusinessError maps a sentinel onto a typed error that does not
// wrap it, so errors.Is against the sentinel never matches: the class is read from
// the mapped type and the code it carries.
func isAccountClosingIndeterminate(err error) bool {
	var unavailable pkg.ServiceUnavailableError

	return errors.As(err, &unavailable) && unavailable.Code == constant.ErrAccountClosingProtectionIndeterminate.Error()
}
