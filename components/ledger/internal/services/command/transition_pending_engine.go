// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/skip"
)

type pendingEngineStableContext struct {
	executionID        uuid.UUID
	organizationID     uuid.UUID
	ledgerID           uuid.UUID
	tenantID           string
	headerID           string
	enqueuedAt         time.Time
	actionDate         time.Time
	transactionUpdated time.Time
	operationUpdated   time.Time
	parentID           *uuid.UUID
	guard              ExecutionGuard
}

type pendingEngineTransition struct {
	transactionID     uuid.UUID
	persisted         *transaction.Transaction
	input             mtransaction.Transaction
	validate          *mtransaction.Responses
	ledgerSettings    mmodel.LedgerSettings
	honoredTracerSkip bool
	action            string
	stableContext     pendingEngineStableContext
}

func (uc *UseCase) transitionPendingWithEngine(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	run *pendingTransitionRun,
	unlock func(),
	tracerEligible bool,
) (*transaction.Transaction, error) {
	transition, err := uc.preparePendingEngineTransition(ctx, run)
	if err != nil {
		unlock()
		return nil, err
	}

	engineState, err := uc.prepareEngineTransaction(ctx, enginePreparationInput{
		organizationID: run.organizationID,
		ledgerID:       run.ledgerID,
		translation: EngineTranslationInput{
			TransactionID:          transition.transactionID,
			Action:                 transition.action,
			TransactionStatus:      run.status,
			RouteValidationEnabled: transition.ledgerSettings.Accounting.ValidateRoutes,
			TransactionInput:       transition.input,
			Validate:               transition.validate,
		},
	})
	if err != nil {
		unlock()
		return nil, err
	}

	prepared, err := buildPendingEngineExecution(transition.persisted, transition.input, transition.validate, engineState, transition.stableContext, transition.action)
	if err != nil {
		unlock()
		return nil, err
	}

	outcome, executeErr := ExecutePreparedEngine(ctx, uc.Engine, prepared)
	if executeErr != nil {
		if !outcome.Executed {
			unlock()
			return nil, executeErr
		}

		if isConfirmedEngineGuardConflict(executeErr) {
			unlock()
			return nil, uc.resolvePendingGuardConflict(ctx, run.organizationID, run.ledgerID, transition.transactionID)
		}

		if confirmedPrecommitEngineFailure(prepared.Execution.Execution, executeErr) {
			unlock()
		}

		return nil, MapEngineError(prepared.Execution.Execution, executeErr)
	}

	if tracerEligible {
		identity := run.reservationIdentity()

		switch run.status {
		case constant.APPROVED:
			uc.confirmReservationsByTransaction(ctx, span, logger, transition.ledgerSettings.Tracer, identity, transition.honoredTracerSkip)
		case constant.CANCELED:
			uc.releaseReservationsByTransaction(ctx, span, logger, transition.ledgerSettings.Tracer, identity, transition.honoredTracerSkip)
		}
	}

	return uc.finalizePendingEngineResult(ctx, logger, run.status, outcome)
}

func (uc *UseCase) preparePendingEngineTransition(ctx context.Context, run *pendingTransitionRun) (pendingEngineTransition, error) {
	if err := ctx.Err(); err != nil {
		return pendingEngineTransition{}, err
	}

	if uc.TransactionReader == nil || isNilAppliedTransactionCompleter(uc.AppliedTransactionCompleter) {
		return pendingEngineTransition{}, fmt.Errorf("engine transition dependencies are not configured")
	}

	transactionID, err := uuid.Parse(run.tran.ID)
	if err != nil || transactionID == uuid.Nil {
		return pendingEngineTransition{}, fmt.Errorf("confirm pending transaction identity: %w", ErrInvalidTransactionCompletionRecord)
	}

	persisted, err := uc.TransactionReader.GetTransactionWithOperationsByID(
		readrouting.WithPrimaryRead(ctx), run.organizationID, run.ledgerID, transactionID,
	)
	if err != nil {
		return pendingEngineTransition{}, err
	}

	if err := validatePersistedCompletionTransition(persisted, run.organizationID, run.ledgerID, transactionID, run.status); err != nil {
		return pendingEngineTransition{}, err
	}

	guardBootstrapper, ok := uc.Engine.(EngineGuardBootstrapper)
	if !ok {
		return pendingEngineTransition{}, fmt.Errorf("engine transition guard bootstrapper is not configured")
	}

	prepared, err := uc.preparePendingEngineIntent(ctx, run, persisted, transactionID)
	if err != nil {
		return pendingEngineTransition{}, err
	}

	if err := guardBootstrapper.EnsureTransactionGuard(ctx, run.organizationID, run.ledgerID, transactionID, constant.PENDING); err != nil {
		return pendingEngineTransition{}, fmt.Errorf("ensure pending transaction guard: %w", err)
	}

	executionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return pendingEngineTransition{}, fmt.Errorf("generate engine execution id: %w", err)
	}

	_, _, headerID, _ := libObservability.NewTrackingFromContext(ctx)
	prepared.stableContext = pendingEngineStableContext{
		executionID: executionID, organizationID: run.organizationID, ledgerID: run.ledgerID,
		tenantID: tmcore.GetTenantIDContext(ctx), headerID: headerID,
		enqueuedAt: time.Now(), actionDate: time.Now(), transactionUpdated: time.Now(), operationUpdated: time.Now(),
		parentID: prepared.stableContext.parentID,
		guard:    ExecutionGuard{TransactionID: transactionID, ExpectedToken: constant.PENDING, NextToken: run.status},
	}

	return prepared, nil
}

func (uc *UseCase) preparePendingEngineIntent(ctx context.Context, run *pendingTransitionRun, persisted *transaction.Transaction, transactionID uuid.UUID) (pendingEngineTransition, error) {
	input, err := clonePendingTransactionInput(persisted.Body)
	if err != nil {
		return pendingEngineTransition{}, err
	}

	mtransaction.ApplyDefaultBalanceKeys(input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(input.Send.Distribute.To)
	mtransaction.MutateConcatAliases(input.Send.Source.From)
	mtransaction.MutateConcatAliases(input.Send.Distribute.To)

	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, input, run.status)
	if err != nil {
		return pendingEngineTransition{}, pkg.HandleKnownBusinessValidationErrors(err)
	}

	ledgerSettings, err := uc.TransactionReader.GetParsedLedgerSettings(ctx, run.organizationID, run.ledgerID)
	if err != nil {
		return pendingEngineTransition{}, err
	}

	if ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, validate, run.status)
	}

	if run.status == constant.CANCELED {
		applyPendingOverdraftCaps(validate, persisted.Operations)
	}

	honoredTracerSkip, _ := skip.ResolveSkipFor("tracer", persisted.Body.Skip != nil && persisted.Body.Skip.Tracer, ledgerSettings.Overrides.AllowTracerSkip)

	parentID, err := pendingEngineParentID(transactionID, persisted.ParentTransactionID)
	if err != nil {
		return pendingEngineTransition{}, err
	}

	action := constant.ActionCommit
	if run.status == constant.CANCELED {
		action = constant.ActionCancel
	}

	return pendingEngineTransition{
		transactionID: transactionID, persisted: persisted, input: input, validate: validate,
		ledgerSettings: ledgerSettings, honoredTracerSkip: honoredTracerSkip, action: action,
		stableContext: pendingEngineStableContext{parentID: parentID},
	}, nil
}

func validatePersistedCompletionTransition(persisted *transaction.Transaction, organizationID, ledgerID, transactionID uuid.UUID, status string) error {
	if persisted == nil || persisted.ID == "" {
		return pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, constant.EntityTransaction)
	}

	if err := validatePersistedTransitionScope(persisted, organizationID, ledgerID, transactionID); err != nil {
		return err
	}

	if persisted.Status.Code != constant.PENDING {
		return pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
	}

	if persisted.Body.IsEmpty() || persisted.CreatedAt.IsZero() {
		return fmt.Errorf("confirm pending transaction body and creation date: %w", ErrInvalidTransactionCompletionRecord)
	}

	if status == constant.CANCELED && len(persisted.Operations) == 0 {
		return fmt.Errorf("cancel pending transaction without persisted operations: %w", ErrInvalidTransactionCompletionRecord)
	}

	if status != constant.APPROVED && status != constant.CANCELED {
		return fmt.Errorf("unsupported pending transition status %q: %w", status, ErrInvalidTransactionCompletionRecord)
	}

	return nil
}

func validatePersistedTransitionScope(persisted *transaction.Transaction, organizationID, ledgerID, transactionID uuid.UUID) error {
	persistedID, idErr := uuid.Parse(persisted.ID)
	persistedOrganizationID, organizationErr := uuid.Parse(persisted.OrganizationID)

	persistedLedgerID, ledgerErr := uuid.Parse(persisted.LedgerID)
	if idErr != nil || organizationErr != nil || ledgerErr != nil || persistedID != transactionID || persistedOrganizationID != organizationID || persistedLedgerID != ledgerID {
		return fmt.Errorf("confirm pending transaction scope: %w", ErrInvalidTransactionCompletionRecord)
	}

	return nil
}

func clonePendingTransactionInput(input mtransaction.Transaction) (mtransaction.Transaction, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return mtransaction.Transaction{}, fmt.Errorf("clone pending transaction body: %w", err)
	}

	var clone mtransaction.Transaction

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := decoder.Decode(&clone); err != nil {
		return mtransaction.Transaction{}, fmt.Errorf("clone pending transaction body: %w", err)
	}

	return clone, nil
}

func applyPendingOverdraftCaps(validate *mtransaction.Responses, operations []*operation.Operation) {
	usageByAlias := pendingOverdraftUsageByAlias(operations)
	for alias, amount := range validate.From {
		usage := usageByAlias[accountAliasFromOperationAlias(alias)]
		if !usage.IsPositive() {
			continue
		}

		amount.OverdraftAmount = usage
		validate.From[alias] = amount
	}
}

func pendingEngineParentID(transactionID uuid.UUID, value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}

	parentID, err := uuid.Parse(*value)
	if err != nil || parentID == uuid.Nil || parentID == transactionID {
		return nil, fmt.Errorf("invalid pending parent transaction identity: %w", ErrInvalidTransactionCompletionRecord)
	}

	return &parentID, nil
}

func buildPendingEngineExecution(
	persisted *transaction.Transaction,
	input mtransaction.Transaction,
	validate *mtransaction.Responses,
	prepared enginePreparedTransaction,
	stableContext pendingEngineStableContext,
	action string,
) (PreparedEngineExecution, error) {
	payload := TransactionCompletionPlan{
		FormatVersion:        TransactionCompletionFormatVersion,
		TenantID:             stableContext.tenantID,
		HeaderID:             stableContext.headerID,
		TransactionID:        prepared.transaction.ID,
		ParentTransactionID:  stableContext.parentID,
		FeesSkipped:          persisted.FeesSkipped,
		TracerSkipped:        persisted.TracerSkipped,
		OrganizationID:       stableContext.organizationID,
		LedgerID:             stableContext.ledgerID,
		ExecutionID:          stableContext.executionID,
		TransactionInput:     input,
		TTL:                  stableContext.enqueuedAt,
		Validate:             validate,
		TransactionStatus:    stableContext.guard.NextToken,
		Action:               action,
		TransactionDate:      stableContext.actionDate,
		TransactionCreatedAt: persisted.CreatedAt,
		TransactionUpdatedAt: stableContext.transactionUpdated,
		OperationUpdatedAt:   stableContext.operationUpdated,
		OperationSpecs:       prepared.projection,
	}

	intent := EngineIntent{
		TenantID:       stableContext.tenantID,
		OrganizationID: payload.OrganizationID,
		LedgerID:       payload.LedgerID,
		ExecutionID:    stableContext.executionID,
		Transactions:   []EngineTransactionIntent{transactionCompletionIntent(prepared.transaction, payload)},
	}

	fingerprint, err := ComputeEngineIntentFingerprint(intent)
	if err != nil {
		return PreparedEngineExecution{}, err
	}

	payload.IntentFingerprint = fingerprint

	raw, err := EncodeTransactionCompletionPlan(payload)
	if err != nil {
		return PreparedEngineExecution{}, err
	}

	execution := EngineExecution{
		Execution: accounting.Execution{
			OrganizationID: payload.OrganizationID,
			LedgerID:       payload.LedgerID,
			ExecutionID:    stableContext.executionID,
			Transactions:   []accounting.Transaction{prepared.transaction},
			Balances:       prepared.pool.Snapshots,
		},
		IntentFingerprint: fingerprint,
		Guards:            []ExecutionGuard{stableContext.guard},
		CompletionPlans:   []CompletionPlanRecord{{TransactionID: payload.TransactionID, Payload: raw}},
	}

	return PreparedEngineExecution{Execution: execution, CompletionPlan: payload}, nil
}

func (uc *UseCase) finalizePendingEngineResult(ctx context.Context, logger libLog.Logger, expectedStatus string, outcome EngineExecutionOutcome) (*transaction.Transaction, error) {
	envelope, err := createEngineEnvelope(outcome)
	if err != nil {
		return nil, err
	}

	completion, err := uc.AppliedTransactionCompleter.Complete(ctx, envelope)
	if err != nil {
		return nil, err
	}

	if completion.Outcome.TransactionStatus != expectedStatus {
		return nil, fmt.Errorf("%w: pending completer confirmed %q, expected %q", ErrTransactionCompletionConflict, completion.Outcome.TransactionStatus, expectedStatus)
	}

	tran := completion.Record.Transaction
	if tran == nil {
		return nil, invalidTransactionCompletionRecord("pending completer returned no materialized transaction")
	}

	uc.acknowledgeEngineRecovery(ctx, logger, envelope, completion)

	tenantCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))
	uc.sendLogTransactionAuditQueueAsync(tenantCtx, tran.Operations, envelope.OrganizationID, envelope.LedgerID, envelope.TransactionID)

	return tran, nil
}

func isConfirmedEngineGuardConflict(err error) bool {
	var technical engineTechnicalError
	return errors.As(err, &technical) && technical.EngineFailureCode() == "execution_guard_conflict" && !technical.OutcomeIndeterminate()
}

func (uc *UseCase) resolvePendingGuardConflict(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) error {
	persisted, err := uc.TransactionReader.GetTransactionWithOperationsByID(
		readrouting.WithPrimaryRead(ctx), organizationID, ledgerID, transactionID,
	)
	if err != nil {
		return err
	}

	if persisted == nil || persisted.ID == "" {
		return pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, constant.EntityTransaction)
	}

	if err := validatePersistedTransitionScope(persisted, organizationID, ledgerID, transactionID); err != nil {
		return err
	}

	if persisted.Status.Code == constant.PENDING {
		return pkg.ValidateBusinessError(constant.ErrPendingTransactionLocked, "ValidateTransactionNotPending")
	}

	if persisted.Status.Code == constant.APPROVED || persisted.Status.Code == constant.CANCELED {
		return pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
	}

	return fmt.Errorf("pending transaction guard conflicts with status %q: %w", persisted.Status.Code, ErrInvalidTransactionCompletionRecord)
}
