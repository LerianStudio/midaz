// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type createBalanceExecutionContext struct {
	executionID        uuid.UUID
	tenantID           string
	headerID           string
	enqueuedAt         time.Time
	transactionUpdated time.Time
	operationUpdated   time.Time
	parentID           *uuid.UUID
	guard              ExecutionGuard
}

// createTransactionWithBalanceEngine is the opt-in create path. Everything
// before it remains the version-specific validation/control pipeline; everything
// after it is engine execution plus recovery completion, never legacy balance
// mutation or write-behind persistence.
func (uc *UseCase) createTransactionWithBalanceEngine(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	run *createTransactionRun,
	tracerEligible bool,
) (*transaction.Transaction, error) {
	tran, err := uc.executeCreateBalanceEngine(ctx, span, logger, run, tracerEligible)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to create transaction with balance engine", err)
	}

	return tran, err
}

func (uc *UseCase) executeCreateBalanceEngine(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	run *createTransactionRun,
	tracerEligible bool,
) (*transaction.Transaction, error) {
	if isNilTransactionCompleter(uc.TransactionCompleter) {
		uc.rollbackCreateClaim(ctx, run)
		return nil, fmt.Errorf("balance engine completer is not configured")
	}

	executionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		uc.rollbackCreateClaim(ctx, run)
		return nil, fmt.Errorf("generate balance engine execution id: %w", err)
	}

	_, _, headerID, _ := libObservability.NewTrackingFromContext(ctx)
	parentID := buildBalanceEngineParentID(run.parentTransactionID)

	nextToken := constant.APPROVED
	if run.status == constant.PENDING {
		nextToken = constant.PENDING
	}

	frozen := createBalanceExecutionContext{
		executionID: executionID, tenantID: tmcore.GetTenantIDContext(ctx), headerID: headerID,
		enqueuedAt: time.Now(), transactionUpdated: time.Now(), operationUpdated: time.Now(),
		parentID: parentID,
		guard:    ExecutionGuard{TransactionID: run.transactionID, ExpectedToken: "", NextToken: nextToken},
	}

	firstPrepared, err := uc.prepareCreateBalanceEngineAttempt(ctx, run)
	if err != nil {
		uc.rollbackCreateClaim(ctx, run)
		return nil, err
	}

	firstAttempt, err := uc.buildCreateBalanceEngineAttempt(run, frozen, firstPrepared)
	if err != nil {
		uc.rollbackCreateClaim(ctx, run)
		return nil, err
	}

	reservation := reservationOutcome{Kind: reservationProceed}
	if tracerEligible {
		reservation = uc.reserveTransaction(ctx, span, logger, run.ledgerSettings.Tracer, run.transactionID,
			run.input.Send.Value, run.input.Send.Asset,
			firstSourceAccountID(run.validate.Sources, firstPrepared.pool.ExplicitBalances),
			run.transactionDate, reservationTTLForStatus(run.status), run.honoredTracerSkip)
		if reservation.Kind == reservationReject {
			uc.rollbackCreateClaim(ctx, run)
			return nil, reservation.Err
		}
	}

	first := true
	state := &createBalanceEngineExecutionState{delegate: uc.BalanceEngine, allConfirmedPrecommit: true}

	retryResult, executeErr := ExecuteBalanceEngineWithRetry(ctx, state, func(buildCtx context.Context) (BalanceEngineAttempt, error) {
		state.lastStep = createBalanceEngineStepBuild

		if first {
			first = false
			return firstAttempt, nil
		}

		prepared, prepareErr := uc.prepareCreateBalanceEngineAttempt(buildCtx, run)
		if prepareErr != nil {
			return BalanceEngineAttempt{}, prepareErr
		}

		return uc.buildCreateBalanceEngineAttempt(run, frozen, prepared)
	})
	if executeErr != nil {
		if state.executionCount == 0 || state.allConfirmedPrecommit {
			uc.rollbackCreateClaim(ctx, run)

			if tracerEligible {
				uc.releaseReservations(ctx, span, logger, reservation.Handle)
			}
		}

		if state.lastStep == createBalanceEngineStepExecute {
			return nil, MapBalanceEngineError(retryResult.Attempt.Execution.Request, executeErr)
		}

		return nil, executeErr
	}

	if run.status != constant.PENDING && tracerEligible {
		uc.confirmReservations(ctx, span, logger, reservation.Handle)
	}

	return uc.finalizeCreateBalanceEngineResult(ctx, run, retryResult)
}

func (uc *UseCase) finalizeCreateBalanceEngineResult(ctx context.Context, run *createTransactionRun, retryResult BalanceEngineRetryResult) (*transaction.Transaction, error) {
	envelope, err := createBalanceEngineEnvelope(retryResult)
	if err != nil {
		return nil, err
	}

	completion, err := uc.TransactionCompleter.Complete(ctx, envelope)
	if err != nil {
		return nil, err
	}

	expectedStatus := run.status
	if expectedStatus == constant.CREATED {
		expectedStatus = constant.APPROVED
	}

	if completion.Outcome.TransactionStatus != expectedStatus {
		return nil, fmt.Errorf("%w: create completer confirmed %q, expected %q", ErrTransactionCompletionConflict, completion.Outcome.TransactionStatus, expectedStatus)
	}

	tran := completion.Record.Transaction
	if tran == nil {
		return nil, invalidTransactionCompletionRecord("create completer returned no materialized transaction")
	}

	if run.status == constant.CREATED {
		created := constant.CREATED
		tran.Status = transaction.Status{Code: created, Description: &created}
	}

	bgCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))
	go uc.SetTransactionIdempotencyValue(bgCtx, run.organizationID, run.ledgerID, run.idempotencyKey, run.idempotencyHash, *tran, run.idempotencyTTL)
	go uc.SendLogTransactionAuditQueue(bgCtx, tran.Operations, run.organizationID, run.ledgerID, tran.IDtoUUID())

	return tran, nil
}

func (uc *UseCase) prepareCreateBalanceEngineAttempt(ctx context.Context, run *createTransactionRun) (balanceEnginePreparedTransaction, error) {
	return uc.prepareBalanceEngineTransaction(ctx, balanceEnginePreparationInput{
		organizationID: run.organizationID,
		ledgerID:       run.ledgerID,
		translation: BalanceEngineTranslationInput{
			TransactionID: run.transactionID, Action: run.action, TransactionStatus: run.status,
			RouteValidationEnabled: run.ledgerSettings.Accounting.ValidateRoutes,
			TransactionInput:       run.input, Validate: run.validate,
		},
		validateBalanceRules: true,
	})
}

func (uc *UseCase) buildCreateBalanceEngineAttempt(run *createTransactionRun, frozen createBalanceExecutionContext, prepared balanceEnginePreparedTransaction) (BalanceEngineAttempt, error) {
	payload := TransactionCompletionPlan{
		FormatVersion: TransactionCompletionFormatVersion,
		TenantID:      frozen.tenantID, HeaderID: frozen.headerID,
		TransactionID: run.transactionID, ParentTransactionID: frozen.parentID,
		FeesSkipped: run.honoredFeeSkip, TracerSkipped: run.honoredTracerSkip,
		OrganizationID: run.organizationID, LedgerID: run.ledgerID, ExecutionID: frozen.executionID,
		TransactionInput: run.input, TTL: frozen.enqueuedAt, Validate: run.validate,
		TransactionStatus: run.status, Action: run.action, TransactionDate: run.transactionDate,
		TransactionCreatedAt: run.transactionDate, TransactionUpdatedAt: frozen.transactionUpdated,
		OperationUpdatedAt: frozen.operationUpdated, OperationSpecs: prepared.projection,
	}

	intent := BalanceEngineIntent{
		TenantID: frozen.tenantID, OrganizationID: run.organizationID, LedgerID: run.ledgerID,
		ExecutionID:  frozen.executionID,
		Transactions: []BalanceEngineTransactionIntent{transactionCompletionIntent(prepared.transaction, payload)},
	}

	fingerprint, err := ComputeBalanceEngineIntentFingerprint(intent)
	if err != nil {
		return BalanceEngineAttempt{}, err
	}

	payload.IntentFingerprint = fingerprint

	raw, err := EncodeTransactionCompletionPlan(payload)
	if err != nil {
		return BalanceEngineAttempt{}, err
	}

	execution := EngineExecution{
		Request: engine.Request{
			OrganizationID: run.organizationID, LedgerID: run.ledgerID, ExecutionID: frozen.executionID,
			Transactions: []engine.Transaction{prepared.transaction}, Balances: prepared.pool.Snapshots,
		},
		IntentFingerprint: fingerprint,
		RetentionSeconds:  idempotencyRetentionSeconds(run.idempotencyTTL),
		Guards:            []ExecutionGuard{frozen.guard},
		CompletionPlans:   []CompletionPlanRecord{{TransactionID: run.transactionID, Payload: raw}},
	}

	return BalanceEngineAttempt{Execution: execution, Payload: payload}, nil
}

// idempotencyRetentionSeconds accepts the repository's historical seconds-count
// convention and a real time.Duration used by internal callers and fixtures.
func idempotencyRetentionSeconds(ttl time.Duration) int64 {
	if ttl >= time.Second || ttl <= -time.Second {
		return int64(ttl / time.Second)
	}

	return int64(ttl)
}

func createBalanceEngineEnvelope(result BalanceEngineRetryResult) (*TransactionCompletionRecord, error) {
	if result.Result == nil || len(result.Attempt.Execution.CompletionPlans) != 1 {
		return nil, invalidBalanceEngineResult(errors.New("successful create has no correlated recovery result"))
	}

	payload := result.Attempt.Payload

	return &TransactionCompletionRecord{
		FormatVersion: TransactionCompletionFormatVersion,
		TenantID:      payload.TenantID, OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID,
		ExecutionID: payload.ExecutionID, IntentFingerprint: payload.IntentFingerprint,
		TransactionID: payload.TransactionID,
		Payload:       string(result.Attempt.Execution.CompletionPlans[0].Payload),
		Result:        *result.Result,
	}, nil
}

func buildBalanceEngineParentID(parentID uuid.UUID) *uuid.UUID {
	if parentID == uuid.Nil {
		return nil
	}

	value := parentID

	return &value
}

func isNilTransactionCompleter(completer TransactionCompleter) bool {
	if completer == nil {
		return true
	}

	value := reflect.ValueOf(completer)

	return value.Kind() == reflect.Pointer && value.IsNil()
}

const (
	createBalanceEngineStepBuild   = "build"
	createBalanceEngineStepExecute = "execute"
)

type createBalanceEngineExecutionState struct {
	delegate              BalanceEngine
	executionCount        int
	allConfirmedPrecommit bool
	lastStep              string
}

func (state *createBalanceEngineExecutionState) Execute(ctx context.Context, input EngineExecution) (*engine.Result, error) {
	state.lastStep = createBalanceEngineStepExecute
	state.executionCount++

	result, err := state.delegate.Execute(ctx, input)
	if err == nil || result != nil || !confirmedPrecommitBalanceEngineFailure(input.Request, err) {
		state.allConfirmedPrecommit = false
	}

	return result, err
}

func confirmedPrecommitBalanceEngineFailure(request engine.Request, err error) bool {
	var technical balanceEngineTechnicalError
	if errors.As(err, &technical) {
		return !technical.OutcomeIndeterminate() && technical.EngineFailureCode() != "execution_guard_conflict"
	}

	var failure *engine.Failure
	if errors.As(err, &failure) && failure != nil {
		switch failure.Code {
		case engine.FailureInsufficientFunds,
			engine.FailureOverdraftLimitExceeded,
			engine.FailureOverdraftNotEligible,
			engine.FailureOverdraftCompanionMissing,
			engine.FailureStaleVersion,
			engine.FailureBalanceDeleted,
			engine.FailureOnHoldUnderflow,
			engine.FailureBalanceMissing:
		default:
			return false
		}

		_, valid := engineFailurePosting(request, failure)

		return valid
	}

	return false
}

var _ BalanceEngine = (*createBalanceEngineExecutionState)(nil)
