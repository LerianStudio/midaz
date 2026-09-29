// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
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
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	// feeDebtCollectionDescription describes the transaction of a standalone collection.
	feeDebtCollectionDescription = "Fee debt collection"
	// feeDebtCollectionFingerprintDomain keeps a collection's idempotency fingerprint
	// apart from every transaction create sharing the ledger's key space.
	feeDebtCollectionFingerprintDomain = "midaz.fee_debt.collect" + IdempotencyDiscriminatorSep
)

// CollectFeeDebtInput names the debtor balance ("alias#key") of a standalone
// collection; a nil MaxAmount collects up to everything the debtor owes. An empty
// IdempotencyKey has no body-hash fallback: identical keyless collects stay distinct.
type CollectFeeDebtInput struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	BalanceRef     string
	MaxAmount      *decimal.Decimal
	IdempotencyKey string
	IdempotencyTTL time.Duration
}

// CollectFeeDebtResult is what a collection settled and the transaction recording it,
// nil when it settled nothing; Replayed marks the first answer of a reused key.
type CollectFeeDebtResult struct {
	Collected   decimal.Decimal
	Transaction *transaction.Transaction
	Replayed    bool
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

	run := &createTransactionRun{organizationID: in.OrganizationID, ledgerID: in.LedgerID, idempotencyKey: in.IdempotencyKey, idempotencyTTL: in.IdempotencyTTL}
	if run.idempotencyKey != "" {
		maxAmount := ""
		if in.MaxAmount != nil {
			maxAmount = in.MaxAmount.String()
		}

		fingerprint := libCommons.HashSHA256(feeDebtCollectionFingerprintDomain + in.BalanceRef + IdempotencyDiscriminatorSep + maxAmount)

		replay, err := uc.claimTransactionIdempotency(ctx, span, logger, run, fingerprint, fingerprint)
		if err != nil {
			recordCommandError(ctx, span, logger, "Failed to claim fee debt collection idempotency", err)

			return nil, err
		}

		if replay != nil {
			return &CollectFeeDebtResult{Collected: *replay.Amount, Transaction: replay, Replayed: true}, nil
		}
	}

	result, committed, err := uc.collectFeeDebt(ctx, logger, in, run)
	if !committed {
		uc.rollbackCreateClaim(ctx, run)
	}

	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to collect fee debt", err)
	}

	return result, err
}

// collectFeeDebt reports committed once balances moved or may have moved; only then
// does the idempotency claim outlive the call.
func (uc *UseCase) collectFeeDebt(ctx context.Context, logger libLog.Logger, in CollectFeeDebtInput, run *createTransactionRun) (*CollectFeeDebtResult, bool, error) {
	ctx, admissions := accountprotection.ContextWithSink(ctx)
	defer admissions.Release(ctx)

	pool, err := loadPreparedEngineSnapshots(readrouting.WithPrimaryRead(ctx), uc.TransactionReader, in.OrganizationID, in.LedgerID,
		[]string{in.BalanceRef}, feeDebtPoolRefs{debtors: []string{in.BalanceRef}})
	if err != nil {
		return nil, false, err
	}

	prepared, owed, err := composeFeeDebtCollection(in, pool, run)
	if err != nil {
		return nil, false, err
	}

	if !owed {
		return &CollectFeeDebtResult{Collected: decimal.Zero}, false, nil
	}

	executionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, false, fmt.Errorf("generate engine execution id: %w", err)
	}

	_, _, headerID, _ := libObservability.NewTrackingFromContext(ctx)
	frozen := createBalanceExecutionContext{
		executionID: executionID, tenantID: tmcore.GetTenantIDContext(ctx), headerID: headerID,
		enqueuedAt: run.transactionDate, transactionUpdated: run.transactionDate, operationUpdated: run.transactionDate,
		guard: ExecutionGuard{TransactionID: run.transactionID, NextToken: constant.APPROVED},
	}

	execution, err := uc.buildCreateEngineExecution(run, frozen, prepared)
	if err != nil {
		return nil, false, err
	}

	outcome, err := ExecutePreparedEngine(ctx, uc.Engine, execution)
	resolveEngineAdmissions(admissions, execution.Execution.Execution, outcome, err)

	if err != nil {
		if !outcome.Executed {
			return nil, false, err
		}

		return nil, !confirmedPrecommitEngineFailure(execution.Execution.Execution, err), MapEngineError(execution.Execution.Execution, err)
	}

	if len(outcome.Result.Movements) == 0 {
		return &CollectFeeDebtResult{Collected: decimal.Zero}, false, nil
	}

	tran, err := uc.finalizeCreateEngineResult(ctx, logger, run, outcome)
	if err != nil {
		return nil, true, err
	}

	return &CollectFeeDebtResult{Collected: *tran.Amount, Transaction: tran}, true, nil
}

// composeFeeDebtCollection fills run with a direct transaction holding one collect of
// the debtor's seeded debts, capped at the lesser of in.MaxAmount and their remaining
// total; owed is false when the seed names nothing to collect.
func composeFeeDebtCollection(in CollectFeeDebtInput, pool EngineSnapshotPool, run *createTransactionRun) (_ enginePreparedTransaction, owed bool, _ error) {
	balances, err := indexTranslationBalances(pool.Balances)
	if err != nil {
		return enginePreparedTransaction{}, false, err
	}

	debtor, exists := balances[in.BalanceRef]
	if !exists {
		return enginePreparedTransaction{}, false, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)
	}

	ceiling := decimal.Zero
	for _, item := range pool.FeeDebtSeeds[in.BalanceRef] {
		ceiling = ceiling.Add(item.Remaining)
	}

	if in.MaxAmount != nil && in.MaxAmount.LessThan(ceiling) {
		ceiling = *in.MaxAmount
	}

	if !ceiling.IsPositive() {
		return enginePreparedTransaction{}, false, nil
	}

	transactionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return enginePreparedTransaction{}, false, fmt.Errorf("generate transaction id: %w", err)
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
		return enginePreparedTransaction{}, false, nil
	}

	run.transactionID, run.transactionDate, run.input = transactionID, time.Now(), input
	run.validate = &mtransaction.Responses{Sources: []string{in.BalanceRef}}
	run.status, run.action = constant.CREATED, constant.ActionDirect

	return enginePreparedTransaction{pool: pool, transaction: engineTransaction, projection: projection}, true, nil
}
