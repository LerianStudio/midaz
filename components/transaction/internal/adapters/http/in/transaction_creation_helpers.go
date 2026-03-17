// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

type transactionScope struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	ParentID       uuid.UUID
}

type transactionIdempotencyState struct {
	key         string
	hash        string
	ttl         time.Duration
	internalKey *string
	replay      *transaction.Transaction
}

func readTransactionScope(c *fiber.Ctx) (*transactionScope, error) {
	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return nil, err
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return nil, err
	}

	parentID := uuid.Nil
	if c.Locals("transaction_id") != nil {
		parentID, err = http.GetUUIDFromLocals(c, "transaction_id")
		if err != nil {
			return nil, err
		}
	}

	return &transactionScope{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		ParentID:       parentID,
	}, nil
}

func generateTransactionID(ctx context.Context, logger libLog.Logger, span trace.Span) (uuid.UUID, error) {
	transactionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate transaction id", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to generate transaction id: %v", err))

		return uuid.Nil, pkg.InternalServerError{
			Code:    "INTERNAL_SERVER_ERROR",
			Title:   "Internal Server Error",
			Message: "Failed to generate transaction id",
			Err:     err,
		}
	}

	return transactionID, nil
}

func buildParentTransactionID(parentID uuid.UUID) *string {
	if parentID == uuid.Nil {
		return nil
	}

	parentTransactionID := parentID.String()

	return &parentTransactionID
}

func (handler *TransactionHandler) HandleAccountFields(entries []pkgTransaction.FromTo, isConcat bool) []pkgTransaction.FromTo {
	result := make([]pkgTransaction.FromTo, 0, len(entries))

	for i := range entries {
		var newAlias string
		if isConcat {
			newAlias = entries[i].ConcatAlias(i)
		} else {
			newAlias = entries[i].SplitAlias()
		}

		entries[i].AccountAlias = newAlias

		result = append(result, entries[i])
	}

	return result
}

func (handler *TransactionHandler) checkTransactionDate(ctx context.Context, logger libLog.Logger, transactionInput pkgTransaction.Transaction, transactionStatus string) (time.Time, error) {
	now := time.Now()
	transactionDate := now

	if transactionInput.TransactionDate != nil && !transactionInput.TransactionDate.IsZero() {
		if transactionInput.TransactionDate.After(now) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidFutureTransactionDate, "validateTransactionDate")

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("transaction date cannot be a future date: %v", err.Error()))

			return time.Time{}, err
		} else if transactionStatus == constant.PENDING {
			err := pkg.ValidateBusinessError(constant.ErrInvalidPendingFutureTransactionDate, "validateTransactionDate")

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("pending transaction cannot be used together a transaction date: %v", err.Error()))

			return time.Time{}, err
		} else {
			transactionDate = transactionInput.TransactionDate.Time()
		}
	}

	return transactionDate, nil
}

func (handler *TransactionHandler) BuildOperations(
	ctx context.Context,
	balances []*mmodel.Balance,
	fromTo []pkgTransaction.FromTo,
	transactionInput pkgTransaction.Transaction,
	tran transaction.Transaction,
	validate *pkgTransaction.Responses,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*operation.Operation, []*mmodel.Balance, error) {
	var operations []*operation.Operation

	var preBalances []*mmodel.Balance

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction_operations")
	defer span.End()

	orgID, _ := uuid.Parse(tran.OrganizationID)
	ledID, _ := uuid.Parse(tran.LedgerID)

	ledgerSettings := handler.Query.GetLedgerSettings(ctx, orgID, ledID)

	// Callers (createTransaction, commitOrCancelTransaction) must call propagateRouteValidation
	// before invoking BuildOperations so that Amount entries already carry the flag.
	routeValidationEnabled := ledgerSettings.Accounting.ValidateRoutes

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

	return operations, preBalances, nil
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
		Type:            libConstants.ONHOLD,
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
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
		Direction:       amt.Direction,
	}, nil
}

func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, transactionInput pkgTransaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	scope, err := readTransactionScope(c)
	if err != nil {
		return http.WithError(c, err)
	}

	transactionID, err := generateTransactionID(ctx, logger, span)
	if err != nil {
		return http.WithError(c, err)
	}

	c.Set(libConstants.IdempotencyReplayed, "false")

	transactionDate, err := handler.checkTransactionDate(ctx, logger, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to check transaction date", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to check transaction date: %v", err))

		return http.WithError(c, err)
	}

	recordSafePayloadAttributes(span, transactionInput)

	if transactionInput.Send.Value.LessThanOrEqual(decimal.Zero) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction with non-positive value", err)

		logger.Log(ctx, libLog.LevelWarn, "Transaction value must be greater than zero")

		return http.WithError(c, err)
	}

	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, true)...)
	to := handler.HandleAccountFields(transactionInput.Send.Distribute.To, true)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	idempotencyState, err := handler.prepareIdempotency(ctx, c, scope.OrganizationID, scope.LedgerID, transactionInput)
	if err != nil {
		return http.WithError(c, err)
	}

	if idempotencyState.replay != nil {
		return http.Created(c, *idempotencyState.replay)
	}

	validate, err := pkgTransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to validate send source and distribute: %v", err.Error()))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		handler.deleteIdempotencyKey(ctx, idempotencyState.internalKey)

		return http.WithError(c, err)
	}

	ledgerSettings := handler.Query.GetLedgerSettings(ctx, scope.OrganizationID, scope.LedgerID)
	if ledgerSettings.Accounting.ValidateRoutes {
		propagateRouteValidation(ctx, validate, transactionInput.Pending, transactionStatus)
	}

	err = handler.sendTransactionToRedisQueue(ctx, scope.OrganizationID, scope.LedgerID, transactionID, transactionInput, validate, transactionStatus, transactionDate, idempotencyState.internalKey)
	if err != nil {
		return http.WithError(c, err)
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")

	action := constant.ActionDirect
	if transactionStatus == constant.PENDING {
		action = constant.ActionHold
	}

	balancesBefore, balancesAfter, _, err := handler.Query.GetBalances(ctx, scope.OrganizationID, scope.LedgerID, transactionID, &transactionInput, validate, transactionStatus, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanGetBalances, "Failed to get balances", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get balances: %v", err.Error()))

		handler.deleteIdempotencyKey(ctx, idempotencyState.internalKey)

		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, scope.OrganizationID, scope.LedgerID, transactionID.String())
		spanGetBalances.End()

		return http.WithError(c, err)
	}

	spanGetBalances.End()

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, false)...)
	to = handler.HandleAccountFields(transactionInput.Send.Distribute.To, false)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	tran := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      buildParentTransactionID(scope.ParentID),
		OrganizationID:           scope.OrganizationID.String(),
		LedgerID:                 scope.LedgerID.String(),
		Description:              transactionInput.Description,
		Amount:                   &transactionInput.Send.Value,
		AssetCode:                transactionInput.Send.Asset,
		ChartOfAccountsGroupName: transactionInput.ChartOfAccountsGroupName,
		CreatedAt:                transactionDate,
		UpdatedAt:                time.Now(),
		Route:                    transactionInput.Route,
		Metadata:                 transactionInput.Metadata,
		Status: transaction.Status{
			Code:        transactionStatus,
			Description: &transactionStatus,
		},
	}

	operations, _, err := handler.BuildOperations(ctx, balancesBefore, fromTo, transactionInput, *tran, validate, transactionDate, transactionStatus == constant.NOTED)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate balance: %v", err.Error()))

		return http.WithError(c, err)
	}

	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
	tran.Operations = operations

	handler.Command.UpdateTransactionBackupOperations(ctx, scope.OrganizationID, scope.LedgerID, transactionID.String(), operations)

	originalStatus := tran.Status

	if transactionStatus == constant.CREATED {
		approved := constant.APPROVED
		tran.Status = transaction.Status{Code: approved, Description: &approved}
	}

	handler.Command.CreateWriteBehindTransaction(ctx, scope.OrganizationID, scope.LedgerID, tran, transactionInput)

	err = handler.Command.WriteTransaction(ctx, scope.OrganizationID, scope.LedgerID, &transactionInput, validate, balancesBefore, balancesAfter, tran)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err))

		return http.WithError(c, err)
	}

	tran.Status = originalStatus

	go handler.Command.SetValueOnExistingIdempotencyKey(ctx, scope.OrganizationID, scope.LedgerID, idempotencyState.key, idempotencyState.hash, *tran, idempotencyState.ttl)

	go handler.Command.SendLogTransactionAuditQueue(ctx, operations, scope.OrganizationID, scope.LedgerID, tran.IDtoUUID())

	return http.Created(c, tran)
}

func (handler *TransactionHandler) prepareIdempotency(
	ctx context.Context,
	c *fiber.Ctx,
	organizationID, ledgerID uuid.UUID,
	transactionInput pkgTransaction.Transaction,
) (*transactionIdempotencyState, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxIdempotency, spanIdempotency := tracer.Start(ctx, "handler.create_transaction_idempotency")
	defer spanIdempotency.End()

	ts, _ := libCommons.StructToJSONString(transactionInput)
	state := &transactionIdempotencyState{
		hash: libCommons.HashSHA256(ts),
	}

	state.key, state.ttl = http.GetIdempotencyKeyAndTTL(c)

	value, internalKey, err := handler.Command.CreateOrCheckIdempotencyKey(ctxIdempotency, organizationID, ledgerID, state.key, state.hash, state.ttl)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanIdempotency, "Error on create or check redis idempotency key", err)
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Error on create or check redis idempotency key: %v", err.Error()))

		return nil, err
	}

	state.internalKey = internalKey
	if libCommons.IsNilOrEmpty(value) {
		return state, nil
	}

	replay := &transaction.Transaction{}
	if err := json.Unmarshal([]byte(*value), replay); err != nil {
		libOpentelemetry.HandleSpanError(spanIdempotency, "Error to deserialization idempotency transaction json on redis", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to deserialization idempotency transaction json on redis: %v", err))

		return nil, err
	}

	c.Set(libConstants.IdempotencyReplayed, "true")

	state.replay = replay

	return state, nil
}

func (handler *TransactionHandler) sendTransactionToRedisQueue(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionInput pkgTransaction.Transaction,
	validate *pkgTransaction.Responses,
	transactionStatus string,
	transactionDate time.Time,
	internalKey *string,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxSendTransactionToRedisQueue, spanSendTransactionToRedisQueue := tracer.Start(ctx, "handler.create_transaction.send_transaction_to_redis_queue")
	defer spanSendTransactionToRedisQueue.End()

	err := handler.Command.SendTransactionToRedisQueue(ctxSendTransactionToRedisQueue, organizationID, ledgerID, transactionID, transactionInput, validate, transactionStatus, transactionDate, nil)
	if err == nil {
		return nil
	}

	libOpentelemetry.HandleSpanError(spanSendTransactionToRedisQueue, "Failed to send transaction to backup cache", err)
	logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to send transaction to backup cache: %v", err.Error()))

	if errors.Is(err, constant.ErrTransactionBackupCacheMarshalFailed) {
		handler.deleteIdempotencyKey(ctxSendTransactionToRedisQueue, internalKey)
	}

	return pkg.ValidateBusinessError(err, reflect.TypeOf(transaction.Transaction{}).Name())
}

func (handler *TransactionHandler) deleteIdempotencyKey(ctx context.Context, internalKey *string) {
	if internalKey != nil {
		_ = handler.Command.RedisRepo.Del(ctx, *internalKey)
	}
}

func (handler *TransactionHandler) ApplyDefaultBalanceKeys(entries []pkgTransaction.FromTo) {
	for i := range entries {
		if entries[i].BalanceKey == "" {
			entries[i].BalanceKey = constant.DefaultBalanceKey
		}
	}
}

func getAliasWithoutKey(array []string) []string {
	result := make([]string, len(array))

	for i, str := range array {
		parts := strings.Split(str, "#")
		result[i] = parts[0]
	}

	return result
}
