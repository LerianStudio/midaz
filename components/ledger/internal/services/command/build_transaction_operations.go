// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

//nolint:gocognit,gocyclo // Will be refactored into smaller functions.
func (uc *UseCase) BuildOperations(
	ctx context.Context,
	balances []*mmodel.Balance,
	balancesAfter []*mmodel.Balance,
	fromTo []mtransaction.FromTo,
	transactionInput mtransaction.Transaction,
	tran transaction.Transaction,
	validate *mtransaction.Responses,
	transactionDate time.Time,
	isAnnotation bool,
	routeValidationEnabled bool,
	transactionRouteCache *mmodel.TransactionRouteCache,
	action string,
) ([]*operation.Operation, []*mmodel.Balance, error) {
	var operations []*operation.Operation

	var preBalances []*mmodel.Balance

	logger, tracer, _, metricFactory := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "command.build_transaction_operations")
	defer span.End()

	if routeValidationEnabled {
		span.SetAttributes(attribute.Bool("app.route_validation_enabled", true))
	}

	// Index Lua's authoritative post-mutation balances by `alias#key` so we
	// can resolve the "balance after" side of each Operation record directly
	// instead of recomputing it via OperateBalances. This is what keeps
	// operation records in sync with the balance table when the Lua
	// overdraft branch floors a credit balance at zero — without this,
	// `available_balance_after` carries the naive `before - amount` arithmetic
	// (e.g. `50 - 100 = -50`) while the DB balance itself shows 0. The map
	// is nil-tolerant: legacy callers (replay path in bootstrap/redis.consumer.go)
	// that cannot supply `balancesAfter` fall back to OperateBalances
	// gracefully.
	afterByAliasKey := make(map[string]*mmodel.Balance, len(balancesAfter))

	for _, b := range balancesAfter {
		if b == nil {
			continue
		}

		afterByAliasKey[b.Alias+"#"+b.Key] = b
	}

	// Track aliases that already had double-entry operations built, so we skip
	// the second Lua before-balance for the same source account.
	processedDoubleEntry := make(map[string]bool)

	for _, blc := range balances {
		for i := range fromTo {
			if blc.Alias == fromTo[i].AccountAlias {
				preBalances = append(preBalances, blc)

				txBal, tErr := blc.ToTransactionBalance()
				if tErr != nil {
					libOpentelemetry.HandleSpanError(span, "Corrupted balance for validation", tErr)

					logger.Log(ctx, libLog.LevelWarn, "Corrupted balance OverdraftLimit", libLog.Err(tErr))

					return nil, nil, tErr
				}

				amt, bat, err := mtransaction.ValidateFromToOperation(fromTo[i], *validate, txBal)
				if err != nil {
					libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)

					logger.Log(ctx, libLog.LevelWarn, "Failed to validate balance", libLog.Err(err))

					return nil, nil, err
				}

				// Prefer the Lua-authoritative after-state when available —
				// it correctly reflects the overdraft floor-at-zero and any
				// OverdraftUsed repayment decrement that the naive
				// OperateBalances path above cannot model.
				//
				// Hoist `after` into the outer scope so the snapshot builder
				// can use it below (after is nil when the legacy replay path
				// cannot supply `balancesAfter`).
				var after *mmodel.Balance

				if a, ok := afterByAliasKey[blc.Alias+"#"+blc.Key]; ok && a != nil {
					after = a
					bat.Available = after.Available
					bat.OnHold = after.OnHold
					bat.Version = after.Version

					// Clip the Operation record's amount to the ACTUAL
					// movement on this balance. Without overdraft the
					// movement equals the requested amount; with
					// overdraft, Lua redirects part of it onto the
					// companion balance (a separate Operation record with
					// its own amount), so the primary op's "amount"
					// field should reflect only what really moved on
					// this balance. This preserves double-entry across
					// the enriched record set:
					//   sum(primary.amount, companion.amount) == requested amount
					// Pre-fix, the primary recorded the full requested
					// amount AND the companion recorded the redirected
					// portion, so sum(debits) > sum(credits) and the
					// audit trail broke. See regression: operation row
					// for overdraft debit split showing amount=100 while
					// the default balance only moved 50.
					amt.Value = effectiveOperationAmount(amt, blc.Available, after.Available, blc.OnHold, after.OnHold)
				}

				if ops, handled, err := uc.tryBuildDoubleEntryOps(
					ctx, blc, fromTo[i], amt, bat, tran, transactionInput,
					transactionDate, isAnnotation, routeValidationEnabled, processedDoubleEntry, i,
				); err != nil {
					return nil, nil, err
				} else if handled {
					if len(ops) > 0 {
						// Always-populated snapshot contract: every op carries
						// snapshot + typed Balance.OverdraftUsed / BalanceAfter
						// .OverdraftUsed regardless of overdraft participation.
						// Double-entry rows share the Lua-authoritative final
						// overdraft lifecycle so pending/cancel overdraft rows do
						// not show stale 0->0 snapshots.
						for _, pendingOp := range ops {
							if isAnnotation {
								pendingOp.Snapshot = buildOperationSnapshot(nil, nil)
								pendingOp.Balance.OverdraftUsed = decimal.Zero
								pendingOp.BalanceAfter.OverdraftUsed = decimal.Zero
							} else {
								pendingOp.Snapshot = buildOperationSnapshot(blc, after)
								if blc != nil {
									pendingOp.Balance.OverdraftUsed = blc.OverdraftUsed
									pendingOp.BalanceAfter.OverdraftUsed = blc.OverdraftUsed
								} else {
									pendingOp.Balance.OverdraftUsed = decimal.Zero
									pendingOp.BalanceAfter.OverdraftUsed = decimal.Zero
								}

								if after != nil {
									pendingOp.BalanceAfter.OverdraftUsed = after.OverdraftUsed
								}
							}
						}

						operations = append(operations, ops...)

						if err := metricFactory.RecordTransactionProcessed(
							ctx,
							attribute.String("organization_id", tran.OrganizationID),
							attribute.String("ledger_id", tran.LedgerID),
						); err != nil {
							libOpentelemetry.HandleSpanError(span, "Failed to record transaction processed metric", err)
						}
					}

					continue
				}

				op, err := uc.buildStandardOp(
					blc, fromTo[i], amt, bat, tran, transactionInput, transactionDate, isAnnotation,
				)
				if err != nil {
					return nil, nil, err
				}

				// Always-populated snapshot contract: every op carries snapshot
				// + typed Balance.OverdraftUsed / BalanceAfter.OverdraftUsed
				// regardless of overdraft participation. Annotation ops carry
				// "0"/"0" + decimal.Zero (no balance movement, but the wire
				// shape stays uniform).
				if isAnnotation {
					op.Snapshot = buildOperationSnapshot(nil, nil)
					op.Balance.OverdraftUsed = decimal.Zero
					op.BalanceAfter.OverdraftUsed = decimal.Zero
				} else {
					op.Snapshot = buildOperationSnapshot(blc, after)

					if blc != nil {
						op.Balance.OverdraftUsed = blc.OverdraftUsed
					} else {
						op.Balance.OverdraftUsed = decimal.Zero
					}

					if after != nil {
						op.BalanceAfter.OverdraftUsed = after.OverdraftUsed
					} else {
						op.BalanceAfter.OverdraftUsed = decimal.Zero
					}
				}

				operations = append(operations, op)

				if err := metricFactory.RecordTransactionProcessed(
					ctx,
					attribute.String("organization_id", tran.OrganizationID),
					attribute.String("ledger_id", tran.LedgerID),
				); err != nil {
					libOpentelemetry.HandleSpanError(span, "Failed to record transaction processed metric", err)
				}
			}
		}
	}

	// Companion snapshot propagation: overdraft companions are built from their
	// own blc/after values (which have OverdraftUsed=0 since the companion balance
	// doesn't track overdraft usage). The primary (default key) operation carries
	// the DEFAULT balance's overdraft lifecycle; propagate it to companions so
	// both rows in the pair tell the same story. Under the always-populated
	// contract every primary already carries a snapshot value; companions get a
	// value-copy of the primary's data.
	snapshotAdapters := make([]*operationForSnapshot, len(operations))
	for idx, op := range operations {
		snapshotAdapters[idx] = &operationForSnapshot{
			accountAlias:              op.AccountAlias,
			balanceKey:                op.BalanceKey,
			snapshot:                  op.Snapshot,
			balanceOverdraftUsed:      op.Balance.OverdraftUsed,
			balanceAfterOverdraftUsed: op.BalanceAfter.OverdraftUsed,
		}
	}

	propagateSnapshotToCompanions(snapshotAdapters)

	for idx, a := range snapshotAdapters {
		operations[idx].Snapshot = a.snapshot
		operations[idx].Balance.OverdraftUsed = a.balanceOverdraftUsed
		operations[idx].BalanceAfter.OverdraftUsed = a.balanceAfterOverdraftUsed
	}

	resolveRouteCodesFromCache(operations, transactionRouteCache, action, transactionInput.OperationTypeOverride)

	return operations, preBalances, nil
}

// statusToAction maps a transaction status code to the corresponding accounting
// action used for looking up the correct AccountingEntries rubric.
// resolveRouteCodesFromCache populates the RouteCode and RouteDescription fields
// on each operation by looking up the operation's RouteID in the transaction route
// cache for the given accounting action (direct, hold, commit, cancel, revert).
//
// Both RouteCode and RouteDescription are resolved from the AccountingRubric
// that matches the operation's action and direction (debit → Debit rubric,
// credit → Credit rubric).
func resolveRouteCodesFromCache(operations []*operation.Operation, cache *mmodel.TransactionRouteCache, action, operationTypeOverride string) {
	if cache == nil {
		return
	}

	for _, op := range operations {
		if op.RouteID == nil || *op.RouteID == "" {
			continue
		}

		// Companion operations (BalanceKey = "overdraft") are emitted by
		// the enrichment engine and represent the overdraft leg of the
		// transaction. They share the primary's RouteID but must resolve
		// their rubric through the `overdraft` AccountingEntry — not the
		// top-level transaction action. This mirrors the hold/commit/cancel
		// pattern where a single routeID drives multiple action-specific
		// rubric lookups.
		//
		// Both DEBIT (overdraft usage — deficit grows) and CREDIT
		// (repayment — deficit shrinks) on the overdraft balance resolve
		// to ActionOverdraft. The direction-based rubric selection
		// (Debit vs Credit) happens inside resolveAccountingRubric via
		// op.Direction. The enrichment engine sets Direction="credit" on
		// repayment companions so the resolver picks Overdraft.Credit
		// without special-casing here.
		resolvedAction := action

		// Block/unblock operations carry the base action (direct) but a
		// semantic OperationTypeOverride (BLOCK/UNBLOCK). When the route
		// configures a dedicated Block/Unblock AccountingEntry, route the
		// rubric lookup to it; otherwise leave resolvedAction unchanged so
		// block/unblock keep resolving via the Direct rubric.
		switch operationTypeOverride {
		case constant.BLOCK:
			if blockEntryConfigured(cache, action, *op.RouteID, func(ae *mmodel.AccountingEntries) bool {
				return ae.Block != nil
			}) {
				resolvedAction = constant.ActionBlock
			}
		case constant.UNBLOCK:
			if blockEntryConfigured(cache, action, *op.RouteID, func(ae *mmodel.AccountingEntries) bool {
				return ae.Unblock != nil
			}) {
				resolvedAction = constant.ActionUnblock
			}
		}

		// The Overdraft BalanceKey override takes precedence: companion
		// operations on the overdraft balance always resolve their rubric
		// through the Overdraft AccountingEntry, regardless of block/unblock.
		if op.BalanceKey == constant.OverdraftBalanceKey {
			resolvedAction = constant.ActionOverdraft
		}

		actionCache, ok := cache.Actions[resolvedAction]
		if !ok {
			// Defensive only. When route validation is enabled, the
			// ValidateAccountingRules gate rejects any overdraft companion whose
			// route lacks a direction-specific overdraft rubric with
			// ErrOverdraftRouteNotConfigured, so a companion reaching here is
			// guaranteed to have its overdraft action present. For block/unblock
			// this branch is also unreachable, since resolvedAction only switches
			// to Block/Unblock after blockEntryConfigured confirms the action
			// exists. Leaving RouteCode nil keeps the primary op resolving
			// correctly for any residual unreachable state.
			continue
		}

		routeID := *op.RouteID

		if rc, ok := actionCache.FindRoute(routeID); ok {
			if rubric := resolveAccountingRubric(rc.AccountingEntries, resolvedAction, op.Direction); rubric != nil && rubric.Code != "" {
				code := rubric.Code
				op.RouteCode = &code

				if rubric.Description != "" {
					desc := rubric.Description
					op.RouteDescription = &desc
				}
			}
		}
	}
}

// blockEntryConfigured reports whether the route identified by routeID — looked up
// under the given base action in the cache — has the block/unblock AccountingEntry
// satisfied by the supplied predicate. Used to decide whether a BLOCK/UNBLOCK
// operation should resolve via its dedicated rubric or fall back to Direct.
func blockEntryConfigured(cache *mmodel.TransactionRouteCache, action, routeID string, has func(*mmodel.AccountingEntries) bool) bool {
	actionCache, ok := cache.Actions[action]
	if !ok {
		return false
	}

	rc, ok := actionCache.FindRoute(routeID)
	if !ok || rc.AccountingEntries == nil {
		return false
	}

	return has(rc.AccountingEntries)
}

// resolveAccountingRubric selects the appropriate AccountingRubric from the given
// AccountingEntries based on the action name and operation direction.
// Returns nil when no matching entry or rubric exists.
func resolveAccountingRubric(entries *mmodel.AccountingEntries, action, direction string) *mmodel.AccountingRubric {
	if entries == nil {
		return nil
	}

	var entry *mmodel.AccountingEntry

	switch strings.ToLower(action) {
	case constant.ActionDirect:
		entry = entries.Direct
	case constant.ActionHold:
		entry = entries.Hold
	case constant.ActionCommit:
		entry = entries.Commit
	case constant.ActionCancel:
		entry = entries.Cancel
	case constant.ActionRevert:
		entry = entries.Revert
	case constant.ActionOverdraft:
		entry = entries.Overdraft
	case constant.ActionBlock:
		entry = entries.Block
	case constant.ActionUnblock:
		entry = entries.Unblock
	}

	if entry == nil {
		return nil
	}

	switch strings.ToLower(direction) {
	case constant.DirectionDebit:
		return entry.Debit
	case constant.DirectionCredit:
		return entry.Credit
	}

	return nil
}

// effectiveOperationAmount returns the amount that should be recorded on a
// standard Operation record when the Lua-authoritative after-state differs
// from the naive "before − requested amount" arithmetic. The only case
// where this happens today is the overdraft engine redirecting part of a
// debit/credit onto the companion balance:
//
//   - Debit split: source.default goes from 50 → 0 for a requested debit of
//     100 (the remaining 50 is debited onto the companion's Available via a
//     separate, enrichment-produced Operation record). The primary
//     Operation record's `amount` should be 50, not 100, so that
//     `sum(debits) == sum(credits)` across the enriched record set.
//   - Credit repayment: destination.default goes from 0 → 30 for a
//     requested credit of 80 because the first 50 repaid outstanding
//     overdraft (recorded as a CREDIT on the companion). The primary
//     Operation record's `amount` should be 30.
//
// For every non-overdraft operation the |Available delta| + |OnHold delta|
// equals the requested amount, so the clip is a no-op — the function is
// safe to call on every operation. Direction-aware arithmetic does not
// need special handling here: what matters is the absolute movement on the
// balance, which is always non-negative.
//
// Returns the smaller of the requested amount and the observed movement,
// so a balance that somehow moves MORE than the requested amount (a bug
// that should not exist) never retroactively inflates the Operation
// amount — we keep the request as the upper bound and surface the anomaly
// via the balance vs. operation divergence in downstream reconciliation.
func effectiveOperationAmount(
	amt mtransaction.Amount,
	beforeAvailable, afterAvailable, beforeOnHold, afterOnHold decimal.Decimal,
) decimal.Decimal {
	availableDelta := beforeAvailable.Sub(afterAvailable).Abs()
	onHoldDelta := beforeOnHold.Sub(afterOnHold).Abs()

	// Available and OnHold deltas are additive for the PENDING/COMMIT/CANCEL
	// transaction-type flow (a PENDING entry moves amount from Available
	// into OnHold; both deltas individually equal the amount, so adding
	// them would double-count). Use the max instead — it matches the
	// requested amount on every non-overdraft path and only shrinks in
	// the overdraft redirect case (where OnHold is untouched and only
	// Available shrinks by the non-redirected portion).
	movement := availableDelta
	if onHoldDelta.GreaterThan(movement) {
		movement = onHoldDelta
	}

	// Upper-bound at the requested amount. A movement greater than the
	// requested amount would indicate a ledger bug (e.g. Lua applied more
	// than it was asked to); we do not want the Operation record to
	// silently inflate in that case.
	if movement.GreaterThan(amt.Value) {
		return amt.Value
	}

	return movement
}

// buildOperationSnapshot returns a snapshot for the operation. Always
// populated — non-overdraft operations carry "0" / "0", matching the
// always-populated wire-shape contract.
//
// `before` and `after` are the pre- and post-mutation balance values for
// the balance the operation acts on. Either may be nil (defensive against
// builder paths that don't have one or the other in scope) — nil decays
// to "0" for the corresponding field.
//
// For PENDING / CANCELED paths, call with before == after (the hold/cancel
// does not mutate OverdraftUsed, but capturing the pre-state keeps shape
// consistent with the standard path).
func buildOperationSnapshot(before, after *mmodel.Balance) mmodel.OperationSnapshot {
	snap := mmodel.OperationSnapshot{
		OverdraftUsedBefore: "0",
		OverdraftUsedAfter:  "0",
	}

	if before != nil {
		snap.OverdraftUsedBefore = before.OverdraftUsed.String()
	}

	if after != nil {
		snap.OverdraftUsedAfter = after.OverdraftUsed.String()
	}

	return snap
}

// operationForSnapshot is a lightweight adapter used by propagateSnapshotToCompanions
// to decouple the propagation logic from the concrete operation.Operation type,
// making the function independently testable.
type operationForSnapshot struct {
	accountAlias              string
	balanceKey                string
	snapshot                  mmodel.OperationSnapshot
	balanceOverdraftUsed      decimal.Decimal
	balanceAfterOverdraftUsed decimal.Decimal
}

// propagateSnapshotToCompanions copies the primary (default-key) operation's
// snapshot and typed OverdraftUsed fields to each companion (overdraft-key)
// operation that shares the same account alias. This ensures both rows in the
// operation pair tell the same lifecycle story.
//
// Under the always-populated contract every primary already carries a snapshot
// value; companions get a value-copy of the primary's data.
func propagateSnapshotToCompanions(ops []*operationForSnapshot) {
	// Index primaries by bare account alias.
	primaryByAlias := make(map[string]*operationForSnapshot, len(ops))

	for _, op := range ops {
		if op.balanceKey != constant.OverdraftBalanceKey {
			primaryByAlias[op.accountAlias] = op
		}
	}

	for _, op := range ops {
		if op.balanceKey != constant.OverdraftBalanceKey {
			continue
		}

		primary, ok := primaryByAlias[op.accountAlias]
		if !ok {
			continue
		}

		op.snapshot = primary.snapshot
		op.balanceOverdraftUsed = primary.balanceOverdraftUsed
		op.balanceAfterOverdraftUsed = primary.balanceAfterOverdraftUsed
	}
}

// zeroAnnotationBalances zeroes out the Available, OnHold, and Version fields of the
// given balance and balanceAfter structs. It is used for annotation operations where
// we record the operation shape but do not reflect real balance values.
// Each call allocates fresh values so that callers never share pointers.
func zeroAnnotationBalances(balance, balanceAfter *operation.Balance) {
	aBefore := decimal.NewFromInt(0)
	balance.Available = &aBefore
	aAfter := decimal.NewFromInt(0)
	balanceAfter.Available = &aAfter

	oBefore := decimal.NewFromInt(0)
	balance.OnHold = &oBefore
	oAfter := decimal.NewFromInt(0)
	balanceAfter.OnHold = &oAfter

	vBefore := int64(0)
	balance.Version = &vBefore
	vAfter := int64(0)
	balanceAfter.Version = &vAfter
}

// propagateRouteValidation sets RouteValidationEnabled on Amount entries in the
// validate response maps when the transaction is pending or canceled. This flag controls how
// OperateBalances splits balance effects between Available and OnHold fields.
// buildDoubleEntryPendingOps generates two operations for a PENDING source entry
// when route validation is enabled:
// Op1: DEBIT (debit direction) - decreases Available only
// Op2: ONHOLD (credit direction) - increases OnHold only
// This ensures proper double-entry where each operation affects a single balance field.
func (uc *UseCase) buildDoubleEntryPendingOps(
	ctx context.Context,
	blc *mmodel.Balance,
	ft mtransaction.FromTo,
	amt mtransaction.Amount,
	bat mtransaction.Balance,
	tran transaction.Transaction,
	transactionInput mtransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*operation.Operation, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "command.build_double_entry_pending_ops")
	defer span.End()

	description := ft.Description
	if libCommons.IsNilOrEmpty(&ft.Description) {
		description = transactionInput.Description
	}

	// Op1: DEBIT (debit) - Available-- only. Prefer Lua's authoritative
	// final Available when present; the overdraft branch floors at zero, while
	// naive local arithmetic would record negative audit balances.
	debitAvailable := blc.Available.Sub(amt.Value)
	debitAmount := amt.Value
	debitVersion := blc.Version + 1
	onholdOnHold := blc.OnHold.Add(amt.Value)
	onholdVersion := debitVersion + 1

	if bat.Version > 0 {
		debitAvailable = bat.Available
		onholdOnHold = bat.OnHold
		onholdVersion = bat.Version

		debitAmount = blc.Available.Sub(debitAvailable).Abs()
		if debitAmount.GreaterThan(amt.Value) {
			debitAmount = amt.Value
		}
	}

	debitBalance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &blc.OnHold,
		Version:   &blc.Version,
	}

	debitBalanceAfter := operation.Balance{
		Available: &debitAvailable,
		OnHold:    &blc.OnHold,
		Version:   &debitVersion,
	}

	if isAnnotation {
		zeroAnnotationBalances(&debitBalance, &debitBalanceAfter)
	}

	op1ID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, err
	}

	op1 := &operation.Operation{
		ID:              op1ID.String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            constant.DEBIT,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          operation.Amount{Value: &debitAmount},
		Balance:         debitBalance,
		BalanceAfter:    debitBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    mtransaction.SplitAlias(blc.Alias),
		AccountType:     blc.AccountType,
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
		Direction:       constant.DirectionDebit,
	}

	// Op2: ONHOLD (credit) - OnHold++ only
	// Balance before: op1's balance after (chaining)
	// Balance after: OnHold increased, Available unchanged from op1

	onholdBalance := operation.Balance{
		Available: &debitAvailable,
		OnHold:    &blc.OnHold,
		Version:   &debitVersion,
	}

	onholdBalanceAfter := operation.Balance{
		Available: &debitAvailable,
		OnHold:    &onholdOnHold,
		Version:   &onholdVersion,
	}

	if isAnnotation {
		zeroAnnotationBalances(&onholdBalance, &onholdBalanceAfter)
	}

	op2ID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, err
	}

	op2 := &operation.Operation{
		ID:              op2ID.String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            constant.ONHOLD,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          operation.Amount{Value: &amt.Value},
		Balance:         onholdBalance,
		BalanceAfter:    onholdBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    mtransaction.SplitAlias(blc.Alias),
		AccountType:     blc.AccountType,
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
		Direction:       constant.DirectionCredit,
	}

	return []*operation.Operation{op1, op2}, nil
}

// buildDoubleEntryCanceledOps generates two operations for a CANCELED source entry
// when route validation is enabled:
// Op1: RELEASE (debit direction) - decreases OnHold only
// Op2: CREDIT (credit direction) - increases Available only
// This ensures proper double-entry where each operation affects a single balance field.
func (uc *UseCase) buildDoubleEntryCanceledOps(
	ctx context.Context,
	blc *mmodel.Balance,
	ft mtransaction.FromTo,
	amt mtransaction.Amount,
	_ mtransaction.Balance,
	tran transaction.Transaction,
	transactionInput mtransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*operation.Operation, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "command.build_double_entry_canceled_ops")
	defer span.End()

	description := ft.Description
	if libCommons.IsNilOrEmpty(&ft.Description) {
		description = transactionInput.Description
	}

	// Op1: RELEASE (debit) - OnHold-- only
	// Balance before: original balance
	// Balance after: OnHold decreased, Available unchanged
	releaseOnHold := blc.OnHold.Sub(amt.Value)
	releaseVersion := blc.Version + 1

	releaseBalance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &blc.OnHold,
		Version:   &blc.Version,
	}

	releaseBalanceAfter := operation.Balance{
		Available: &blc.Available,
		OnHold:    &releaseOnHold,
		Version:   &releaseVersion,
	}

	if isAnnotation {
		zeroAnnotationBalances(&releaseBalance, &releaseBalanceAfter)
	}

	op1ID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, err
	}

	op1 := &operation.Operation{
		ID:              op1ID.String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            constant.RELEASE,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          operation.Amount{Value: &amt.Value},
		Balance:         releaseBalance,
		BalanceAfter:    releaseBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    mtransaction.SplitAlias(blc.Alias),
		AccountType:     blc.AccountType,
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
		Direction:       constant.DirectionDebit,
	}

	// Op2: CREDIT (credit) - Available++ only
	// Balance before: op1's balance after (chaining)
	// Balance after: Available increased, OnHold unchanged from op1
	creditAvailable := blc.Available.Add(amt.Value)
	creditVersion := releaseVersion + 1

	creditBalance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &releaseOnHold,
		Version:   &releaseVersion,
	}

	creditBalanceAfter := operation.Balance{
		Available: &creditAvailable,
		OnHold:    &releaseOnHold,
		Version:   &creditVersion,
	}

	if isAnnotation {
		zeroAnnotationBalances(&creditBalance, &creditBalanceAfter)
	}

	op2ID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, err
	}

	op2 := &operation.Operation{
		ID:              op2ID.String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            constant.CREDIT,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          operation.Amount{Value: &amt.Value},
		Balance:         creditBalance,
		BalanceAfter:    creditBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    mtransaction.SplitAlias(blc.Alias),
		AccountType:     blc.AccountType,
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
		Direction:       constant.DirectionCredit,
	}

	return []*operation.Operation{op1, op2}, nil
}

// tryBuildDoubleEntryOps checks whether the current balance entry qualifies for double-entry
// splitting (PENDING or CANCELED with route validation) and, if so, returns the pair of operations.
// Returns handled=true when operations were built, false when standard path should be used.
func (uc *UseCase) tryBuildDoubleEntryOps(
	ctx context.Context,
	blc *mmodel.Balance,
	ft mtransaction.FromTo,
	amt mtransaction.Amount,
	bat mtransaction.Balance,
	tran transaction.Transaction,
	transactionInput mtransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
	routeValidationEnabled bool,
	processedDoubleEntry map[string]bool,
	fromToIndex int,
) ([]*operation.Operation, bool, error) {
	if !routeValidationEnabled || !ft.IsFrom {
		return nil, false, nil
	}

	if !mtransaction.IsDoubleEntrySource(amt) {
		return nil, false, nil
	}

	isPendingDoubleEntry := amt.TransactionType == constant.PENDING

	// Dedup key combines alias with fromTo index so that multiple entries
	// for the same account (e.g. transfer + fee) each produce their own
	// double-entry pair, while the balances×fromTo nested loop is still
	// protected against generating duplicates for the same entry.
	dedupKey := blc.Alias + "#" + strconv.Itoa(fromToIndex)

	if processedDoubleEntry[dedupKey] {
		return nil, true, nil
	}

	processedDoubleEntry[dedupKey] = true

	if isPendingDoubleEntry {
		ops, err := uc.buildDoubleEntryPendingOps(
			ctx, blc, ft, amt, bat, tran, transactionInput, transactionDate, isAnnotation,
		)

		return ops, true, err
	}

	ops, err := uc.buildDoubleEntryCanceledOps(
		ctx, blc, ft, amt, bat, tran, transactionInput, transactionDate, isAnnotation,
	)

	return ops, true, err
}

// buildStandardOp creates a single operation for a standard (non-double-entry) balance entry.
func (uc *UseCase) buildStandardOp(
	blc *mmodel.Balance,
	ft mtransaction.FromTo,
	amt mtransaction.Amount,
	bat mtransaction.Balance,
	tran transaction.Transaction,
	transactionInput mtransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
) (*operation.Operation, error) {
	amount := operation.Amount{
		Value: &amt.Value,
	}

	balance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &blc.OnHold,
		Version:   &blc.Version,
	}

	balanceAfter := operation.Balance{
		Available: &bat.Available,
		OnHold:    &bat.OnHold,
		Version:   &bat.Version,
	}

	if isAnnotation {
		zeroAnnotationBalances(&balance, &balanceAfter)
	}

	description := ft.Description
	if libCommons.IsNilOrEmpty(&ft.Description) {
		description = transactionInput.Description
	}

	operationID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, err
	}

	opType := amt.Operation
	if blc.Key == constant.OverdraftBalanceKey {
		opType = constant.OVERDRAFT
	} else if transactionInput.OperationTypeOverride != "" {
		// Block/unblock paths set a semantic Type label (BLOCK/UNBLOCK). It
		// overrides only the Type, never Direction or amount. Overdraft
		// companion rows keep their OVERDRAFT label and are excluded above.
		opType = transactionInput.OperationTypeOverride
	}

	return &operation.Operation{
		ID:              operationID.String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            opType,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          amount,
		Balance:         balance,
		BalanceAfter:    balanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    mtransaction.SplitAlias(blc.Alias),
		AccountType:     blc.AccountType,
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
		Direction:       amt.Direction,
	}, nil
}
