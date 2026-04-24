// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

//nolint:gocognit // Will be refactored into smaller functions.
func (handler *TransactionHandler) BuildOperations(
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

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction_operations")
	defer span.End()

	if routeValidationEnabled {
		logger.Log(ctx, libLog.LevelInfo, "Route validation enabled for ledger", libLog.String("ledger_id", tran.LedgerID))

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
				logger.Log(ctx, libLog.LevelInfo, "Creating operation for account", libLog.String("account_id", blc.ID), libLog.String("account_alias", blc.Alias))

				preBalances = append(preBalances, blc)

				amt, bat, err := mtransaction.ValidateFromToOperation(fromTo[i], *validate, blc.ToTransactionBalance())
				if err != nil {
					libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)

					logger.Log(ctx, libLog.LevelWarn, "Failed to validate balance", libLog.Err(err))

					return nil, nil, err
				}

				// Prefer the Lua-authoritative after-state when available —
				// it correctly reflects the overdraft floor-at-zero and any
				// OverdraftUsed repayment decrement that the naive
				// OperateBalances path above cannot model.
				if after, ok := afterByAliasKey[blc.Alias+"#"+blc.Key]; ok && after != nil {
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

				if ops, handled, err := handler.tryBuildDoubleEntryOps(
					ctx, blc, fromTo[i], amt, bat, tran, transactionInput,
					transactionDate, isAnnotation, routeValidationEnabled, processedDoubleEntry, i,
				); err != nil {
					return nil, nil, err
				} else if handled {
					if len(ops) > 0 {
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

				op, err := handler.buildStandardOp(
					blc, fromTo[i], amt, bat, tran, transactionInput, transactionDate, isAnnotation,
				)
				if err != nil {
					return nil, nil, err
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

	resolveRouteCodesFromCache(operations, transactionRouteCache, action)

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
func resolveRouteCodesFromCache(operations []*operation.Operation, cache *mmodel.TransactionRouteCache, action string) {
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

		if op.BalanceKey == constant.OverdraftBalanceKey {
			resolvedAction = constant.ActionOverdraft
		}

		actionCache, ok := cache.Actions[resolvedAction]
		if !ok {
			// No entries for this action — e.g. route defines only Direct
			// and the companion wants Overdraft. Leave RouteCode nil; the
			// primary op still resolves correctly, and the companion
			// simply carries no accounting rubric.
			continue
		}

		routeID := *op.RouteID

		if rc, ok := findRouteInActionCache(actionCache, routeID); ok {
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

// findRouteInActionCache searches for a routeID across Source, Destination, and
// Bidirectional maps of an ActionRouteCache.
func findRouteInActionCache(actionCache mmodel.ActionRouteCache, routeID string) (mmodel.OperationRouteCache, bool) {
	if rc, ok := actionCache.Source[routeID]; ok {
		return rc, true
	}

	if rc, ok := actionCache.Destination[routeID]; ok {
		return rc, true
	}

	if rc, ok := actionCache.Bidirectional[routeID]; ok {
		return rc, true
	}

	return mmodel.OperationRouteCache{}, false
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
func (handler *TransactionHandler) buildDoubleEntryPendingOps(
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.build_double_entry_pending_ops")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Building double-entry pending ops", libLog.String("balance_id", blc.ID))

	description := ft.Description
	if libCommons.IsNilOrEmpty(&ft.Description) {
		description = transactionInput.Description
	}

	// Op1: DEBIT (debit) - Available-- only
	// Balance before: original balance
	// Balance after: Available decreased, OnHold unchanged
	debitAvailable := blc.Available.Sub(amt.Value)
	debitVersion := blc.Version + 1

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
		Amount:          operation.Amount{Value: &amt.Value},
		Balance:         debitBalance,
		BalanceAfter:    debitBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    mtransaction.SplitAlias(blc.Alias),
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
		Direction:       constant.DirectionDebit,
	}

	// Op2: ONHOLD (credit) - OnHold++ only
	// Balance before: op1's balance after (chaining)
	// Balance after: OnHold increased, Available unchanged from op1
	onholdOnHold := blc.OnHold.Add(amt.Value)
	onholdVersion := debitVersion + 1

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
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
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
func (handler *TransactionHandler) buildDoubleEntryCanceledOps(
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.build_double_entry_canceled_ops")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Building double-entry canceled ops", libLog.String("balance_id", blc.ID))

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
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
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
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
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
func (handler *TransactionHandler) tryBuildDoubleEntryOps(
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

	// Dedup key combines alias with fromTo index so that multiple DSL entries
	// for the same account (e.g. transfer + fee) each produce their own
	// double-entry pair, while the balances×fromTo nested loop is still
	// protected against generating duplicates for the same entry.
	dedupKey := blc.Alias + "#" + strconv.Itoa(fromToIndex)

	if processedDoubleEntry[dedupKey] {
		return nil, true, nil
	}

	processedDoubleEntry[dedupKey] = true

	if isPendingDoubleEntry {
		ops, err := handler.buildDoubleEntryPendingOps(
			ctx, blc, ft, amt, bat, tran, transactionInput, transactionDate, isAnnotation,
		)

		return ops, true, err
	}

	ops, err := handler.buildDoubleEntryCanceledOps(
		ctx, blc, ft, amt, bat, tran, transactionInput, transactionDate, isAnnotation,
	)

	return ops, true, err
}

// buildStandardOp creates a single operation for a standard (non-double-entry) balance entry.
func (handler *TransactionHandler) buildStandardOp(
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

	return &operation.Operation{
		ID:              operationID.String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            amt.Operation,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          amount,
		Balance:         balance,
		BalanceAfter:    balanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    mtransaction.SplitAlias(blc.Alias),
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
		Direction:       amt.Direction,
	}, nil
}

// createTransaction creates a new transaction with the given status.
func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, transactionInput mtransaction.Transaction, transactionStatus string) error {
	return handler.executeCreateTransaction(c, transactionInput, transactionStatus, false)
}

// createRevertTransaction creates a reversal transaction. The action is forced
// to "revert" so that accounting route lookups use the revert rubrics instead
// of the status-derived action.
func (handler *TransactionHandler) createRevertTransaction(c *fiber.Ctx, transactionInput mtransaction.Transaction, transactionStatus string) error {
	return handler.executeCreateTransaction(c, transactionInput, transactionStatus, true)
}

func (handler *TransactionHandler) executeCreateTransaction(c *fiber.Ctx, transactionInput mtransaction.Transaction, transactionStatus string, isRevert bool) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction.orchestrate")
	defer span.End()

	params, err := readPathParams(c)
	if err != nil {
		return http.WithError(c, err)
	}

	transactionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate transaction id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to generate transaction id", libLog.Err(err))

		return http.WithError(c, err)
	}

	transactionDate, err := mtransaction.CheckTransactionDate(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction date validation failed", err)

		return http.WithError(c, err)
	}

	recordSafePayloadAttributes(span, transactionInput)

	if transactionInput.Send.Value.LessThanOrEqual(decimal.Zero) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction value must be greater than zero", err)
		logger.Log(ctx, libLog.LevelWarn, "Transaction value must be greater than zero", libLog.String("value", transactionInput.Send.Value.String()))

		return http.WithError(c, err)
	}

	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []mtransaction.FromTo

	fromTo = append(fromTo, mtransaction.MutateConcatAliases(transactionInput.Send.Source.From)...)
	to := mtransaction.MutateConcatAliases(transactionInput.Send.Distribute.To)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	// Idempotency: extract key/TTL from HTTP headers, hash the request body,
	// then check or claim the idempotency slot in Redis.
	idempotencyKey, idempotencyTTL := http.GetIdempotencyKeyAndTTL(c)

	ts, err := libCommons.StructToJSONString(transactionInput)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to serialize transaction for idempotency hash", err)
		logger.Log(ctx, libLog.LevelError, "Failed to serialize transaction for idempotency hash", libLog.Err(err))

		return http.WithError(c, err)
	}

	idempotencyHash := libCommons.HashSHA256(ts)

	c.Set(libConstants.IdempotencyReplayed, "false")

	idempotencyResult, err := handler.Command.CreateOrCheckTransactionIdempotency(ctx, params.OrganizationID, params.LedgerID, idempotencyKey, idempotencyHash, idempotencyTTL)
	if err != nil {
		return http.WithError(c, err)
	}

	if idempotencyResult.Replay != nil {
		c.Set(libConstants.IdempotencyReplayed, "true")

		return http.Created(c, *idempotencyResult.Replay)
	}

	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return http.WithError(c, err)
	}

	ledgerSettings, err := handler.Query.GetParsedLedgerSettings(ctx, params.OrganizationID, params.LedgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return http.WithError(c, err)
	}

	if ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, validate, transactionStatus)
	}

	action := mtransaction.StatusToAction(transactionStatus)
	if isRevert {
		action = constant.ActionRevert
	}

	err = handler.Command.SendTransactionToRedisQueue(ctx, params.OrganizationID, params.LedgerID, transactionID, transactionInput, validate, transactionStatus, action, transactionDate, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to send transaction to backup cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to send transaction to backup cache", libLog.Err(err))

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return http.WithError(c, pkg.ValidateBusinessError(err, constant.EntityTransaction))
	}

	balances, err := handler.Query.GetBalances(ctx, params.OrganizationID, params.LedgerID, validate.Aliases)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balances", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get balances", libLog.Err(err))

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return http.WithError(c, err)
	}

	// Scope protection on the CREATE path: SendTransactionToRedisQueue above
	// runs with nil balances (the queue seed precedes GetBalances), so its
	// built-in scope guard is a no-op for user-created transactions. Re-check
	// here now that balances are loaded. Rejecting a direct operation on an
	// internal-scope balance BEFORE enrichment runs keeps the companion
	// balance isolated from client-initiated mutations.
	if err := rejectInternalScopeBalances(ctx, balances); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rejected transaction targeting internal-scope balance", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected transaction targeting internal-scope balance", libLog.Err(err))

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return http.WithError(c, err)
	}

	balanceOps := buildBalanceOperations(ctx, params.OrganizationID, params.LedgerID, validate, balances)

	// Overdraft enrichment: when a source debit exceeds available funds on a
	// credit-direction balance with AllowOverdraft=true, append a debit op on
	// the companion #overdraft balance for the deficit. See
	// transaction_overdraft_enrichment.go for the full rationale. Disabled
	// balances and out-of-scope operations fall through as a no-op so legacy
	// transaction flows remain untouched.
	//
	// `companionFromTos` are returned so the caller can splice them into the
	// `fromTo` slice built below; without this, BuildOperations' match loop
	// never emits an Operation record for the companion balance and the
	// audit trail is missing the overdraft leg (DB balances still converge
	// correctly, but `response.operations` and Postgres `operation` rows do
	// not include the companion).
	balanceOps, companionFromTos, err := enrichOverdraftOperations(ctx, params.OrganizationID, params.LedgerID, balanceOps,
		validate, handler.Query.GetBalances)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to enrich overdraft operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to enrich overdraft operations", libLog.Err(err))

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return http.WithError(c, err)
	}

	routeCache, err := handler.Query.ValidateAccountingRules(ctx, params.OrganizationID, params.LedgerID, balanceOps, validate, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate accounting rules", libLog.Err(err))

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return http.WithError(c, err)
	}

	result, err := handler.Command.ProcessBalanceOperations(ctx, command.ProcessBalanceOperationsInput{
		OrganizationID:    params.OrganizationID,
		LedgerID:          params.LedgerID,
		TransactionID:     transactionID,
		TransactionInput:  &transactionInput,
		Validate:          validate,
		BalanceOperations: balanceOps,
		TransactionStatus: transactionStatus,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to process balance operations", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to process balance operations", libLog.Err(err))

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return http.WithError(c, err)
	}

	balancesBefore, balancesAfter := result.Before, result.After

	fromTo = append(fromTo, mtransaction.MutateSplitAliases(transactionInput.Send.Source.From)...)
	to = mtransaction.MutateSplitAliases(transactionInput.Send.Distribute.To)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	// Splice the enrichment-produced companion FromTo entries into the slice
	// BEFORE BuildOperations runs. Each companion carries an AccountAlias in
	// concat form ("<i>#@alias#overdraft") that matches the Lua-returned
	// `balance.Alias`, so the `balances × fromTo` loop in BuildOperations
	// now emits one Operation record per companion balance mutation. This
	// is the audit-trail half of the enrichment contract; the balance-state
	// half is handled by the enrichment engine up above.
	fromTo = append(fromTo, companionFromTos...)

	amount := transactionInput.Send.Value

	tran := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      buildParentTransactionID(params.TransactionID),
		OrganizationID:           params.OrganizationID.String(),
		LedgerID:                 params.LedgerID.String(),
		Description:              transactionInput.Description,
		Amount:                   &amount,
		AssetCode:                transactionInput.Send.Asset,
		ChartOfAccountsGroupName: transactionInput.ChartOfAccountsGroupName,
		CreatedAt:                transactionDate,
		UpdatedAt:                time.Now(),
		Route:                    transactionInput.Route,
		RouteID:                  transactionInput.RouteID,
		Metadata:                 transactionInput.Metadata,
		Status: transaction.Status{
			Code:        transactionStatus,
			Description: &transactionStatus,
		},
	}

	operations, _, err := handler.BuildOperations(ctx, balancesBefore, balancesAfter, fromTo, transactionInput, *tran, validate, transactionDate, transactionStatus == constant.NOTED, ledgerSettings.Accounting.ValidateRoutes, routeCache, action)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build operations", libLog.Err(err))

		// Idempotency key and backup queue entry are intentionally preserved here.
		// Balances were already mutated by the Lua script (ProcessBalanceOperations),
		// so the backup queue is the recovery mechanism — the Kiwi consumer will
		// reconstruct and persist the transaction from the backup entry.
		// Deleting the idempotency key would allow duplicate balance mutations on retry.
		return http.WithError(c, err)
	}

	// The companion overdraft balances participate in the transaction at the
	// ledger layer but are NOT user-facing sources or destinations — they are
	// system-managed liability ledgers. Filter them out of the alias-key
	// lists before stripping `#key` so `tran.Source` / `tran.Destination`
	// reflect only the client-submitted accounts (and do not produce duplicates
	// like `[@alice, @alice]` when the companion's bare alias collapses to
	// the same value after the strip).
	tran.Source = getAliasWithoutKey(filterCompanionAliases(validate.Sources))
	tran.Destination = getAliasWithoutKey(filterCompanionAliases(validate.Destinations))
	tran.Operations = operations

	handler.Command.UpdateTransactionBackupOperations(ctx, params.OrganizationID, params.LedgerID, transactionID.String(), operations, action)

	// Build a shallow copy with the promoted status for persistence and cache.
	// CREATED is a transient status that the DB layer promotes to APPROVED;
	// the cache must reflect the final status for consistent GET reads.
	// The original tran keeps CREATED for the HTTP response and idempotency key.
	writeTran := *tran

	if transactionStatus == constant.CREATED {
		approved := constant.APPROVED
		writeTran.Status = transaction.Status{Code: approved, Description: &approved}
	}

	handler.Command.CreateWriteBehindTransaction(ctx, params.OrganizationID, params.LedgerID, &writeTran, transactionInput)

	err = handler.Command.WriteTransaction(ctx, params.OrganizationID, params.LedgerID, &transactionInput, validate, balancesBefore, balancesAfter, &writeTran)
	if err != nil {
		// Log the original error for debugging. WriteTransaction may fail due to:
		// - msgpack serialization error
		// - RabbitMQ publish failure + DB fallback failure (async mode)
		// - Direct DB write failure (sync mode)
		// The sanitized error uses ErrMessageBrokerUnavailable as a generic
		// "persistence failed" signal — a more accurate error code should be
		// introduced to cover the sync/DB failure cases as well.
		libOpentelemetry.HandleSpanError(span, "Failed to write transaction", err)
		logger.Log(ctx, libLog.LevelError, "Failed to write transaction", libLog.String("transaction_id", tran.ID), libLog.Err(err))

		sanitizedErr := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, constant.EntityTransaction)

		return http.WithError(c, sanitizedErr)
	}

	bgCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))

	go handler.Command.SetTransactionIdempotencyValue(bgCtx, params.OrganizationID, params.LedgerID, idempotencyKey, idempotencyHash, *tran, idempotencyTTL)

	go handler.Command.SendLogTransactionAuditQueue(bgCtx, operations, params.OrganizationID, params.LedgerID, tran.IDtoUUID())

	return http.Created(c, tran)
}

func (handler *TransactionHandler) deleteIdempotencyKey(ctx context.Context, internalKey *string) {
	if internalKey != nil {
		_ = handler.Command.TransactionRedisRepo.Del(ctx, *internalKey)
	}
}
