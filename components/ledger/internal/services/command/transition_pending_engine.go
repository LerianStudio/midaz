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

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
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

type pendingBalanceEngineFrozen struct {
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

type pendingBalanceEngineTransition struct {
	transactionID     uuid.UUID
	persisted         *transaction.Transaction
	input             mtransaction.Transaction
	validate          *mtransaction.Responses
	ledgerSettings    mmodel.LedgerSettings
	honoredTracerSkip bool
	action            string
	frozen            pendingBalanceEngineFrozen
}

func (uc *UseCase) transitionPendingWithBalanceEngine(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	run *pendingTransitionRun,
	unlock func(),
	tracerEligible bool,
) (*transaction.Transaction, error) {
	transition, err := uc.preparePendingBalanceEngineTransition(ctx, run)
	if err != nil {
		unlock()
		return nil, err
	}

	engineState, err := uc.prepareBalanceEngineTransaction(ctx, balanceEnginePreparationInput{
		organizationID: run.organizationID,
		ledgerID:       run.ledgerID,
		translation: BalanceEngineTranslationInput{
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

	prepared, err := buildPendingBalanceEngineExecution(transition.persisted, transition.input, transition.validate, engineState, transition.frozen, transition.action)
	if err != nil {
		unlock()
		return nil, err
	}

	outcome, executeErr := ExecutePreparedBalanceEngine(ctx, uc.BalanceEngine, prepared)
	if executeErr != nil {
		if !outcome.Executed {
			unlock()
			return nil, executeErr
		}

		if isConfirmedBalanceEngineGuardConflict(executeErr) {
			unlock()
			return nil, uc.resolvePendingGuardConflict(ctx, run.organizationID, run.ledgerID, transition.transactionID)
		}

		if confirmedPrecommitBalanceEngineFailure(prepared.Execution.Execution, executeErr) {
			unlock()
		}

		return nil, MapBalanceEngineError(prepared.Execution.Execution, executeErr)
	}

	if tracerEligible {
		switch run.status {
		case constant.APPROVED:
			uc.confirmReservationsByTransaction(ctx, span, logger, transition.ledgerSettings.Tracer, transition.transactionID, transition.honoredTracerSkip)
		case constant.CANCELED:
			uc.releaseReservationsByTransaction(ctx, span, logger, transition.ledgerSettings.Tracer, transition.transactionID, transition.honoredTracerSkip)
		}
	}

	return uc.finalizePendingBalanceEngineResult(ctx, run.status, outcome)
}

func (uc *UseCase) preparePendingBalanceEngineTransition(ctx context.Context, run *pendingTransitionRun) (pendingBalanceEngineTransition, error) {
	if err := ctx.Err(); err != nil {
		return pendingBalanceEngineTransition{}, err
	}

	if uc.TransactionReader == nil || isNilAppliedTransactionCompleter(uc.AppliedTransactionCompleter) {
		return pendingBalanceEngineTransition{}, fmt.Errorf("balance engine transition dependencies are not configured")
	}

	transactionID, err := uuid.Parse(run.tran.ID)
	if err != nil || transactionID == uuid.Nil {
		return pendingBalanceEngineTransition{}, fmt.Errorf("confirm pending transaction identity: %w", ErrInvalidTransactionCompletionRecord)
	}

	persisted, err := uc.TransactionReader.GetTransactionWithOperationsByID(
		readrouting.WithPrimaryRead(ctx), run.organizationID, run.ledgerID, transactionID,
	)
	if err != nil {
		return pendingBalanceEngineTransition{}, err
	}

	if err := validatePersistedCompletionTransition(persisted, run.organizationID, run.ledgerID, transactionID, run.status); err != nil {
		return pendingBalanceEngineTransition{}, err
	}

	guardBootstrapper, ok := uc.BalanceEngine.(BalanceEngineGuardBootstrapper)
	if !ok {
		return pendingBalanceEngineTransition{}, fmt.Errorf("balance engine transition guard bootstrapper is not configured")
	}

	prepared, err := uc.preparePendingBalanceEngineIntent(ctx, run, persisted, transactionID)
	if err != nil {
		return pendingBalanceEngineTransition{}, err
	}

	if err := guardBootstrapper.EnsureTransactionGuard(ctx, run.organizationID, run.ledgerID, transactionID, constant.PENDING); err != nil {
		return pendingBalanceEngineTransition{}, fmt.Errorf("ensure pending transaction guard: %w", err)
	}

	executionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return pendingBalanceEngineTransition{}, fmt.Errorf("generate balance engine execution id: %w", err)
	}

	_, _, headerID, _ := libObservability.NewTrackingFromContext(ctx)
	prepared.frozen = pendingBalanceEngineFrozen{
		executionID: executionID, organizationID: run.organizationID, ledgerID: run.ledgerID,
		tenantID: tmcore.GetTenantIDContext(ctx), headerID: headerID,
		enqueuedAt: time.Now(), actionDate: time.Now(), transactionUpdated: time.Now(), operationUpdated: time.Now(),
		parentID: prepared.frozen.parentID,
		guard:    ExecutionGuard{TransactionID: transactionID, ExpectedToken: constant.PENDING, NextToken: run.status},
	}

	return prepared, nil
}

func (uc *UseCase) preparePendingBalanceEngineIntent(ctx context.Context, run *pendingTransitionRun, persisted *transaction.Transaction, transactionID uuid.UUID) (pendingBalanceEngineTransition, error) {
	input, err := clonePendingTransactionInput(persisted.Body)
	if err != nil {
		return pendingBalanceEngineTransition{}, err
	}

	mtransaction.ApplyDefaultBalanceKeys(input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(input.Send.Distribute.To)
	mtransaction.MutateConcatAliases(input.Send.Source.From)
	mtransaction.MutateConcatAliases(input.Send.Distribute.To)

	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, input, run.status)
	if err != nil {
		return pendingBalanceEngineTransition{}, pkg.HandleKnownBusinessValidationErrors(err)
	}

	ledgerSettings, err := uc.TransactionReader.GetParsedLedgerSettings(ctx, run.organizationID, run.ledgerID)
	if err != nil {
		return pendingBalanceEngineTransition{}, err
	}

	if ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, validate, run.status)
	}

	if run.status == constant.CANCELED {
		applyPendingOverdraftCaps(validate, persisted.Operations)
	}

	honoredTracerSkip, _ := skip.ResolveSkipFor("tracer", persisted.Body.Skip != nil && persisted.Body.Skip.Tracer, ledgerSettings.Overrides.AllowTracerSkip)

	parentID, err := pendingBalanceEngineParentID(transactionID, persisted.ParentTransactionID)
	if err != nil {
		return pendingBalanceEngineTransition{}, err
	}

	action := constant.ActionCommit
	if run.status == constant.CANCELED {
		action = constant.ActionCancel
	}

	return pendingBalanceEngineTransition{
		transactionID: transactionID, persisted: persisted, input: input, validate: validate,
		ledgerSettings: ledgerSettings, honoredTracerSkip: honoredTracerSkip, action: action,
		frozen: pendingBalanceEngineFrozen{parentID: parentID},
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

func pendingBalanceEngineParentID(transactionID uuid.UUID, value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}

	parentID, err := uuid.Parse(*value)
	if err != nil || parentID == uuid.Nil || parentID == transactionID {
		return nil, fmt.Errorf("invalid pending parent transaction identity: %w", ErrInvalidTransactionCompletionRecord)
	}

	return &parentID, nil
}

func buildPendingBalanceEngineExecution(
	persisted *transaction.Transaction,
	input mtransaction.Transaction,
	validate *mtransaction.Responses,
	prepared balanceEnginePreparedTransaction,
	frozen pendingBalanceEngineFrozen,
	action string,
) (PreparedBalanceEngineExecution, error) {
	payload := TransactionCompletionPlan{
		FormatVersion:        TransactionCompletionFormatVersion,
		TenantID:             frozen.tenantID,
		HeaderID:             frozen.headerID,
		TransactionID:        prepared.transaction.ID,
		ParentTransactionID:  frozen.parentID,
		FeesSkipped:          persisted.FeesSkipped,
		TracerSkipped:        persisted.TracerSkipped,
		OrganizationID:       frozen.organizationID,
		LedgerID:             frozen.ledgerID,
		ExecutionID:          frozen.executionID,
		TransactionInput:     input,
		TTL:                  frozen.enqueuedAt,
		Validate:             validate,
		TransactionStatus:    frozen.guard.NextToken,
		Action:               action,
		TransactionDate:      frozen.actionDate,
		TransactionCreatedAt: persisted.CreatedAt,
		TransactionUpdatedAt: frozen.transactionUpdated,
		OperationUpdatedAt:   frozen.operationUpdated,
		OperationSpecs:       prepared.projection,
	}

	intent := BalanceEngineIntent{
		TenantID:       frozen.tenantID,
		OrganizationID: payload.OrganizationID,
		LedgerID:       payload.LedgerID,
		ExecutionID:    frozen.executionID,
		Transactions:   []BalanceEngineTransactionIntent{transactionCompletionIntent(prepared.transaction, payload)},
	}

	fingerprint, err := ComputeBalanceEngineIntentFingerprint(intent)
	if err != nil {
		return PreparedBalanceEngineExecution{}, err
	}

	payload.IntentFingerprint = fingerprint

	raw, err := EncodeTransactionCompletionPlan(payload)
	if err != nil {
		return PreparedBalanceEngineExecution{}, err
	}

	execution := EngineExecution{
		Execution: accounting.Execution{
			OrganizationID: payload.OrganizationID,
			LedgerID:       payload.LedgerID,
			ExecutionID:    frozen.executionID,
			Transactions:   []accounting.Transaction{prepared.transaction},
			Balances:       prepared.pool.Snapshots,
		},
		IntentFingerprint: fingerprint,
		Guards:            []ExecutionGuard{frozen.guard},
		CompletionPlans:   []CompletionPlanRecord{{TransactionID: payload.TransactionID, Payload: raw}},
	}

	return PreparedBalanceEngineExecution{Execution: execution, CompletionPlan: payload}, nil
}

func (uc *UseCase) finalizePendingBalanceEngineResult(ctx context.Context, expectedStatus string, outcome BalanceEngineExecutionOutcome) (*transaction.Transaction, error) {
	envelope, err := createBalanceEngineEnvelope(outcome)
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

	tenantCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))
	go uc.SendLogTransactionAuditQueue(tenantCtx, tran.Operations, envelope.OrganizationID, envelope.LedgerID, envelope.TransactionID)

	return tran, nil
}

func isConfirmedBalanceEngineGuardConflict(err error) bool {
	var technical balanceEngineTechnicalError
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
