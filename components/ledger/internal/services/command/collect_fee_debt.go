// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"slices"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// feeDebtCollectionDescription describes the transaction of a standalone collection.
const feeDebtCollectionDescription = "Fee debt collection"

// CollectFeeDebtInput names the debtor balance ("alias#key") of a standalone
// collection; a nil MaxAmount collects up to everything the debtor owes.
type CollectFeeDebtInput struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	BalanceRef     string
	MaxAmount      *decimal.Decimal
}

// CollectFeeDebtResult is what a collection settled and the transaction recording it,
// nil when it settled nothing.
type CollectFeeDebtResult struct {
	Collected   decimal.Decimal
	Transaction *transaction.Transaction
}

// CollectFeeDebt settles the debtor's open fee debts, oldest first, from its live
// available balance, up to the lesser of MaxAmount and what it owes. It originates
// no fee and reserves nothing; a collection that settles nothing writes nothing.
func (uc *UseCase) CollectFeeDebt(ctx context.Context, in CollectFeeDebtInput) (_ *CollectFeeDebtResult, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.collect_fee_debt")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "collect_fee_debt", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.organization_id", in.OrganizationID.String()),
		attribute.String("app.request.ledger_id", in.LedgerID.String()),
	)

	result, err := uc.collectFeeDebt(ctx, logger, in)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to collect fee debt", err)
	}

	return result, err
}

func (uc *UseCase) collectFeeDebt(ctx context.Context, logger libLog.Logger, in CollectFeeDebtInput) (*CollectFeeDebtResult, error) {
	ctx, admissions := accountprotection.ContextWithSink(ctx)
	defer admissions.Release(ctx)

	if isNilAppliedTransactionCompleter(uc.AppliedTransactionCompleter) {
		return nil, fmt.Errorf("applied transaction completer is not configured")
	}

	pool, err := loadPreparedEngineSnapshots(readrouting.WithPrimaryRead(ctx), uc.TransactionReader, in.OrganizationID, in.LedgerID,
		[]string{in.BalanceRef}, feeDebtPoolRefs{debtors: []string{in.BalanceRef}})
	if err != nil {
		return nil, err
	}

	run, prepared, err := composeFeeDebtCollection(in, pool)
	if err != nil {
		return nil, err
	}

	if run == nil {
		return &CollectFeeDebtResult{Collected: decimal.Zero}, nil
	}

	executionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("generate engine execution id: %w", err)
	}

	_, _, headerID, _ := libObservability.NewTrackingFromContext(ctx)
	frozen := createBalanceExecutionContext{
		executionID: executionID, tenantID: tmcore.GetTenantIDContext(ctx), headerID: headerID,
		enqueuedAt: run.transactionDate, transactionUpdated: run.transactionDate, operationUpdated: run.transactionDate,
		guard: ExecutionGuard{TransactionID: run.transactionID, NextToken: constant.APPROVED},
	}

	execution, err := uc.buildCreateEngineExecution(run, frozen, prepared)
	if err != nil {
		return nil, err
	}

	outcome, err := ExecutePreparedEngine(ctx, uc.Engine, execution)
	resolveEngineAdmissions(admissions, execution.Execution.Execution, outcome, err)

	if err != nil {
		if !outcome.Executed {
			return nil, err
		}

		return nil, MapEngineError(execution.Execution.Execution, err)
	}

	if len(outcome.Result.Movements) == 0 {
		return &CollectFeeDebtResult{Collected: decimal.Zero}, nil
	}

	tran, err := uc.finalizeCreateEngineResult(ctx, logger, run, outcome)
	if err != nil {
		return nil, err
	}

	return &CollectFeeDebtResult{Collected: *tran.Amount, Transaction: tran}, nil
}

// composeFeeDebtCollection builds a direct transaction holding one collect of the
// debtor's seeded debts, capped at the lesser of in.MaxAmount and their remaining
// total; a nil run means the seed names nothing to collect.
func composeFeeDebtCollection(in CollectFeeDebtInput, pool EngineSnapshotPool) (*createTransactionRun, enginePreparedTransaction, error) {
	balances, err := indexTranslationBalances(pool.Balances)
	if err != nil {
		return nil, enginePreparedTransaction{}, err
	}

	debtor, exists := balances[in.BalanceRef]
	if !exists {
		return nil, enginePreparedTransaction{}, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)
	}

	ceiling := decimal.Zero
	for _, item := range pool.FeeDebtSeeds[in.BalanceRef] {
		ceiling = ceiling.Add(item.Remaining)
	}

	if in.MaxAmount != nil && in.MaxAmount.LessThan(ceiling) {
		ceiling = *in.MaxAmount
	}

	if !ceiling.IsPositive() {
		return nil, enginePreparedTransaction{}, nil
	}

	transactionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, enginePreparedTransaction{}, fmt.Errorf("generate transaction id: %w", err)
	}

	input := mtransaction.Transaction{
		Description: feeDebtCollectionDescription,
		Send:        mtransaction.Send{Asset: debtor.AssetCode, Value: ceiling},
		Metadata:    map[string]any{constant.MetadataKeyFeeDebtCollection: "true"},
	}
	engineTransaction := accounting.Transaction{
		ID: transactionID, RejectBlockedBalances: true,
		BalanceRequirements: []accounting.BalanceRequirement{}, Postings: []accounting.Posting{},
	}
	projection := []OperationRecordSpec{}

	composition := newFeeDebtComposition(EngineTranslationInput{TransactionID: transactionID, TransactionInput: input, FeeDebtSeeds: pool.FeeDebtSeeds}, balances)
	if !composition.collect(&engineTransaction, &projection, "collect", in.BalanceRef, ceiling) {
		return nil, enginePreparedTransaction{}, nil
	}

	validate := &mtransaction.Responses{Sources: []string{in.BalanceRef}}
	for _, spec := range projection {
		if spec.Role == accounting.RoleFeeDebtCredit && !slices.Contains(validate.Destinations, spec.BalanceRef) {
			validate.Destinations = append(validate.Destinations, spec.BalanceRef)
		}
	}

	run := &createTransactionRun{
		organizationID: in.OrganizationID, ledgerID: in.LedgerID, transactionID: transactionID,
		transactionDate: time.Now(), input: input, validate: validate,
		status: constant.CREATED, action: constant.ActionDirect, idempotencyTTL: pkgHTTP.ParseIdempotencyTTL(""),
	}

	return run, enginePreparedTransaction{pool: pool, transaction: engineTransaction, projection: projection}, nil
}
