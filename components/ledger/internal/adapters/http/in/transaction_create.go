// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

func (handler *TransactionHandler) BuildOperations(
	ctx context.Context,
	balances []*mmodel.Balance,
	fromTo []pkgTransaction.FromTo,
	transactionInput pkgTransaction.Transaction,
	tran transaction.Transaction,
	validate *pkgTransaction.Responses,
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
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Route validation enabled for ledger %s, applying double-entry operations", tran.LedgerID))

		span.SetAttributes(attribute.Bool("app.route_validation_enabled", true))
	}

	// Track aliases that already had double-entry operations built, so we skip
	// the second Lua before-balance for the same source account.
	processedDoubleEntry := make(map[string]bool)

	for _, blc := range balances {
		for i := range fromTo {
			if blc.Alias == fromTo[i].AccountAlias {
				logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Creating operation for account id: %s and account alias: %s", blc.ID, blc.Alias))

				preBalances = append(preBalances, blc)

				amt, bat, err := pkgTransaction.ValidateFromToOperation(fromTo[i], *validate, blc.ToTransactionBalance())
				if err != nil {
					libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)

					logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate balance: %v", err.Error()))

					return nil, nil, err
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
func statusToAction(statusCode string) string {
	switch statusCode {
	case constant.PENDING:
		return constant.ActionHold
	case constant.APPROVED:
		return constant.ActionCommit
	case constant.CANCELED:
		return constant.ActionCancel
	default:
		return constant.ActionDirect
	}
}

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

	actionCache, ok := cache.Actions[action]
	if !ok {
		return
	}

	for _, op := range operations {
		if op.RouteID == nil || *op.RouteID == "" {
			continue
		}

		routeID := *op.RouteID

		if rc, ok := findRouteInActionCache(actionCache, routeID); ok {
			if rubric := resolveAccountingRubric(rc.AccountingEntries, action, op.Direction); rubric != nil && rubric.Code != "" {
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
func propagateRouteValidation(ctx context.Context, validate *pkgTransaction.Responses, isPending bool, transactionStatus string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.propagate_route_validation")
	defer span.End()

	if validate == nil {
		return
	}

	isCanceled := transactionStatus == constant.CANCELED
	isApproved := transactionStatus == constant.APPROVED

	if !isPending && !isCanceled && !isApproved {
		return
	}

	count := 0

	for key, amt := range validate.From {
		amt.RouteValidationEnabled = true

		// COMMIT with route validation: source uses ON_HOLD (debit) instead of DEBIT
		// to decrement onHold. This keeps ON_HOLD ops invisible to TransactionRevert,
		// which only considers DEBIT/CREDIT, naturally avoiding double-counting.
		if isApproved && amt.Operation == constant.DEBIT {
			amt.Operation = constant.ONHOLD
		}

		validate.From[key] = amt
		count++
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Propagated route validation to %d source entries (pending=%t, canceled=%t, approved=%t)", count, isPending, isCanceled, isApproved))
}

// buildDoubleEntryPendingOps generates two operations for a PENDING source entry
// when route validation is enabled:
// Op1: DEBIT (debit direction) - decreases Available only
// Op2: ONHOLD (credit direction) - increases OnHold only
// This ensures proper double-entry where each operation affects a single balance field.
func (handler *TransactionHandler) buildDoubleEntryPendingOps(
	ctx context.Context,
	blc *mmodel.Balance,
	ft pkgTransaction.FromTo,
	amt pkgTransaction.Amount,
	_ pkgTransaction.Balance,
	tran transaction.Transaction,
	transactionInput pkgTransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.build_double_entry_pending_ops")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Building double-entry pending ops for balance %s: DEBIT(debit) + ONHOLD(credit)", blc.ID))

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
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
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
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
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
	ft pkgTransaction.FromTo,
	amt pkgTransaction.Amount,
	_ pkgTransaction.Balance,
	tran transaction.Transaction,
	transactionInput pkgTransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.build_double_entry_canceled_ops")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Building double-entry canceled ops for balance %s: RELEASE(debit) + CREDIT(credit)", blc.ID))

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
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
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
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
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
	ft pkgTransaction.FromTo,
	amt pkgTransaction.Amount,
	bat pkgTransaction.Balance,
	tran transaction.Transaction,
	transactionInput pkgTransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
	routeValidationEnabled bool,
	processedDoubleEntry map[string]bool,
	fromToIndex int,
) ([]*operation.Operation, bool, error) {
	if !routeValidationEnabled || !ft.IsFrom {
		return nil, false, nil
	}

	if !pkgTransaction.IsDoubleEntrySource(amt) {
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
	ft pkgTransaction.FromTo,
	amt pkgTransaction.Amount,
	bat pkgTransaction.Balance,
	tran transaction.Transaction,
	transactionInput pkgTransaction.Transaction,
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
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
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

func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, transactionInput pkgTransaction.Transaction, transactionStatus string, actionOverride ...string) error {
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

	c.Set(libConstants.IdempotencyReplayed, "false")

	transactionDate, err := pkgTransaction.CheckTransactionDate(ctx, transactionInput, transactionStatus)
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

	pkgTransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	pkgTransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, pkgTransaction.MutateConcatAliases(transactionInput.Send.Source.From)...)
	to := pkgTransaction.MutateConcatAliases(transactionInput.Send.Distribute.To)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	// Idempotency: extract key/TTL from HTTP headers, hash the request body,
	// then check or claim the idempotency slot in Redis.
	idempotencyKey, idempotencyTTL := http.GetIdempotencyKeyAndTTL(c)

	ts, _ := libCommons.StructToJSONString(transactionInput)
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

	validate, err := pkgTransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return http.WithError(c, err)
	}

	ledgerSettings := handler.Query.GetParsedLedgerSettings(ctx, params.OrganizationID, params.LedgerID)
	if ledgerSettings.Accounting.ValidateRoutes {
		propagateRouteValidation(ctx, validate, transactionInput.Pending, transactionStatus)
	}

	action := statusToAction(transactionStatus)

	if len(actionOverride) > 0 && actionOverride[0] != "" {
		action = actionOverride[0]
	}

	err = handler.sendTransactionToRedisQueue(ctx, params.OrganizationID, params.LedgerID, transactionID, transactionInput, validate, transactionStatus, action, transactionDate, idempotencyResult.InternalKey)
	if err != nil {
		return http.WithError(c, err)
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")

	balancesBefore, balancesAfter, routeCache, err := handler.Query.GetBalances(ctx, params.OrganizationID, params.LedgerID, transactionID, &transactionInput, validate, transactionStatus, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanGetBalances, "Failed to get balances", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get balances: %v", err.Error()))

		handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())
		spanGetBalances.End()

		return http.WithError(c, err)
	}

	spanGetBalances.End()

	fromTo = append(fromTo, pkgTransaction.MutateSplitAliases(transactionInput.Send.Source.From)...)
	to = pkgTransaction.MutateSplitAliases(transactionInput.Send.Distribute.To)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

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

	operations, _, err := handler.BuildOperations(ctx, balancesBefore, fromTo, transactionInput, *tran, validate, transactionDate, transactionStatus == constant.NOTED, ledgerSettings.Accounting.ValidateRoutes, routeCache, action)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate balance: %v", err.Error()))

		return http.WithError(c, err)
	}

	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
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
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err))

		return http.WithError(c, err)
	}

	go handler.Command.SetTransactionIdempotencyValue(ctx, params.OrganizationID, params.LedgerID, idempotencyKey, idempotencyHash, *tran, idempotencyTTL)

	go handler.Command.SendLogTransactionAuditQueue(ctx, operations, params.OrganizationID, params.LedgerID, tran.IDtoUUID())

	return http.Created(c, tran)
}

func (handler *TransactionHandler) sendTransactionToRedisQueue(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionInput pkgTransaction.Transaction,
	validate *pkgTransaction.Responses,
	transactionStatus string,
	action string,
	transactionDate time.Time,
	internalKey *string,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxSendTransactionToRedisQueue, spanSendTransactionToRedisQueue := tracer.Start(ctx, "handler.create_transaction.send_transaction_to_redis_queue")
	defer spanSendTransactionToRedisQueue.End()

	err := handler.Command.SendTransactionToRedisQueue(ctxSendTransactionToRedisQueue, organizationID, ledgerID, transactionID, transactionInput, validate, transactionStatus, action, transactionDate, nil)
	if err == nil {
		return nil
	}

	libOpentelemetry.HandleSpanError(spanSendTransactionToRedisQueue, "Failed to send transaction to backup cache", err)
	logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to send transaction to backup cache: %v", err.Error()))

	if errors.Is(err, constant.ErrTransactionBackupCacheMarshalFailed) {
		handler.deleteIdempotencyKey(ctxSendTransactionToRedisQueue, internalKey)
	}

	return pkg.ValidateBusinessError(err, constant.EntityTransaction)
}

func (handler *TransactionHandler) deleteIdempotencyKey(ctx context.Context, internalKey *string) {
	if internalKey != nil {
		_ = handler.Command.TransactionRedisRepo.Del(ctx, *internalKey)
	}
}
