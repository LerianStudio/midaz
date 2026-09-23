// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type crossLedgerPendingGroupPart struct {
	intent     CrossLedgerGroupIntentPart
	member     *transaction.Transaction
	run        *pendingTransitionRun
	transition pendingEngineTransition
	prepared   PreparedEngineExecution
}

//nolint:gocognit,gocyclo // lifecycle ordering keeps locks, reservations, idempotency, accounting, and recovery in one auditable boundary
func (uc *UseCase) transitionCrossLedgerGroupV2(
	ctx context.Context,
	in PendingTransitionInput,
	target *transaction.Transaction,
	status string,
) (*CreateAtomicTransactionBatchV2Result, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.transition_cross_ledger_group_v2")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if target == nil || target.GroupID == nil {
		return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}

	if uc.TransactionGroupRepo == nil {
		return nil, errors.New("cross-ledger transaction group repository is not configured")
	}

	groupID, err := uuid.Parse(*target.GroupID)
	if err != nil || groupID == uuid.Nil {
		return nil, fmt.Errorf("parse cross-ledger transaction group id: %w", ErrInvalidTransactionCompletionRecord)
	}

	group, err := uc.TransactionGroupRepo.FindByID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	if group == nil || group.ID != groupID {
		return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}

	if group.Status != constant.PENDING {
		return nil, pkg.ValidateBusinessError(
			constant.ErrCrossLedgerGroupNotPending,
			constant.EntityTransaction,
			group.Status,
		)
	}

	if status != constant.APPROVED && status != constant.CANCELED {
		return nil, fmt.Errorf("unsupported cross-ledger group transition status %q", status)
	}

	if uc.Engine == nil {
		return nil, errors.New("cross-ledger group lifecycle engine is not configured")
	}

	if isNilAppliedTransactionCompleter(uc.AppliedTransactionCompleter) {
		return nil, errors.New("cross-ledger group lifecycle completer is not configured")
	}

	intent, err := decodeCrossLedgerGroupIntent(group.Intent)
	if err != nil {
		return nil, fmt.Errorf("decode cross-ledger transaction group %s: %w", groupID, err)
	}

	if group.AssetCode != intent.Asset || group.OrganizationID == uuid.Nil || group.LedgerID == uuid.Nil {
		return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}

	reader, ok := uc.TransactionReader.(TransactionGroupReader)
	if !ok {
		return nil, errors.New("cross-ledger transaction group reader is not configured")
	}

	members, err := reader.FindTransactionsByGroupID(readrouting.WithPrimaryRead(ctx), groupID)
	if err != nil {
		return nil, err
	}

	origins, err := orderCrossLedgerPendingGroupParts(*intent, groupID, in.TransactionID, members)
	if err != nil {
		return nil, err
	}

	releaseLocks, err := uc.lockCrossLedgerPendingGroup(ctx, span, logger, origins, status)
	if err != nil {
		return nil, err
	}

	releaseOnPreparationError := true
	defer func() {
		if releaseOnPreparationError {
			releaseLocks()
		}
	}()

	// The origin and destination loads span ledgers. A cache miss admits its seed
	// only inside the engine, so every ownership they take has to survive until the
	// single execution answers.
	ctx, admissions := accountprotection.ContextWithSink(ctx)
	defer admissions.Release(ctx)

	for index := range origins {
		part := &origins[index]
		if status == constant.APPROVED && part.member.ID == in.TransactionID.String() {
			part.run.accountBlockExceptionID = cloneUUIDPointer(in.AccountBlockExceptionID)
		}

		part.run.accountBlockExceptionGrant, err = uc.resolveAccountBlockExceptionGrant(
			ctx,
			span,
			logger,
			part.run.organizationID,
			part.run.ledgerID,
			part.run.accountBlockExceptionID,
		)
		if err != nil {
			return nil, err
		}

		part.transition, err = uc.preparePendingEngineTransition(ctx, part.run)
		if err != nil {
			return nil, err
		}

		engineState, prepareErr := uc.prepareEngineTransaction(ctx, enginePreparationInput{
			organizationID: part.run.organizationID,
			ledgerID:       part.run.ledgerID,
			translation: EngineTranslationInput{
				TransactionID:              part.transition.transactionID,
				Action:                     part.transition.action,
				TransactionStatus:          status,
				RouteValidationEnabled:     part.transition.ledgerSettings.Accounting.ValidateRoutes,
				TransactionInput:           part.transition.input,
				Validate:                   part.transition.validate,
				AccountBlockExceptionGrant: part.run.accountBlockExceptionGrant,
			},
		})
		if prepareErr != nil {
			return nil, prepareErr
		}

		part.prepared, err = buildPendingEngineExecution(
			part.transition.persisted,
			part.transition.input,
			part.transition.validate,
			engineState,
			part.transition.stableContext,
			part.transition.action,
			part.transition.dependencies,
		)
		if err != nil {
			return nil, err
		}
	}

	var (
		destinationRun      *atomicTransactionBatchRun
		destinationPrepared PreparedEngineExecution
	)
	if status == constant.APPROVED {
		destinationRun, destinationPrepared, err = uc.prepareCrossLedgerGroupDestinations(ctx, span, logger, groupID, *intent)
		if err != nil {
			return nil, err
		}
	}

	executionID, err := crossLedgerGroupExecutionID(uc, destinationRun)
	if err != nil {
		return nil, err
	}

	fragments := make([]PreparedEngineExecution, 0, len(origins)+1)
	for index := range origins {
		fragments = append(fragments, origins[index].prepared)
	}

	if destinationRun != nil {
		fragments = append(fragments, destinationPrepared)
	}

	prepared, err := buildCrossLedgerGroupExecution(
		group.OrganizationID,
		group.LedgerID,
		groupID,
		executionID,
		fragments,
	)
	if err != nil {
		return nil, err
	}

	idempotencyRun, replay, err := uc.claimCrossLedgerGroupTransition(
		ctx,
		group,
		status,
		executionID,
		prepared.CompletionPlans,
	)
	if err != nil {
		return nil, err
	}

	if replay != nil {
		updated, updateErr := uc.TransactionGroupRepo.UpdateStatus(ctx, groupID, constant.PENDING, status)
		if updateErr != nil {
			return nil, updateErr
		}

		if !updated {
			return nil, errors.New("cross-ledger transaction group replay status compare-and-swap did not update")
		}

		return replay, nil
	}

	if err := uc.prepareAtomicTransactionBatchIdempotency(ctx, idempotencyRun); err != nil {
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, idempotencyRun, err)
	}

	if destinationRun != nil {
		if err := uc.reserveAtomicTransactionBatch(ctx, span, logger, destinationRun); err != nil {
			return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, idempotencyRun, err)
		}
	}

	if err := uc.handoffAtomicTransactionBatchExecution(ctx, idempotencyRun); err != nil {
		releaseOnPreparationError = false
		return nil, err
	}

	outcome, executeErr := ExecutePreparedEngine(ctx, uc.Engine, prepared)
	resolveEngineAdmissions(admissions, prepared.Execution.Execution, outcome, executeErr)

	if executeErr != nil {
		confirmedAbort := !outcome.Executed || confirmedPrecommitEngineFailure(prepared.Execution.Execution, executeErr)
		if confirmedAbort {
			if destinationRun != nil {
				uc.settleAtomicTransactionBatchReservations(
					ctx, span, logger, destinationRun, atomicTransactionBatchReservationConfirmedAbort,
				)
			}

			if err := uc.abortAtomicTransactionBatchConfirmedRefusal(ctx, idempotencyRun); err != nil {
				return nil, err
			}

			return nil, MapEngineError(prepared.Execution.Execution, executeErr)
		}

		releaseOnPreparationError = false

		return nil, MapEngineError(prepared.Execution.Execution, executeErr)
	}

	releaseOnPreparationError = false

	transactions, err := uc.completeCrossLedgerGroupTransition(ctx, logger, outcome)
	if err != nil {
		return nil, err
	}

	if err := uc.finalizeCrossLedgerGroupTransition(ctx, idempotencyRun, transactions); err != nil {
		return nil, err
	}

	if destinationRun != nil {
		uc.settleAtomicTransactionBatchReservations(
			ctx, span, logger, destinationRun, atomicTransactionBatchReservationKnownSuccess,
		)
	}

	uc.settleCrossLedgerOriginReservations(ctx, span, logger, status, origins)

	updated, err := uc.TransactionGroupRepo.UpdateStatus(ctx, groupID, constant.PENDING, status)
	if err != nil {
		return nil, err
	}

	if !updated {
		return nil, errors.New("cross-ledger transaction group status compare-and-swap did not update")
	}

	return &CreateAtomicTransactionBatchV2Result{
		BatchID:      groupID,
		Transactions: transactions,
	}, nil
}

func (uc *UseCase) claimCrossLedgerGroupTransition(
	ctx context.Context,
	group *transactiongroup.TransactionGroup,
	status string,
	executionID uuid.UUID,
	plans []TransactionCompletionPlan,
) (*atomicTransactionBatchRun, *CreateAtomicTransactionBatchV2Result, error) {
	if uc.AtomicTransactionBatchIdempotencyRepo == nil {
		return nil, nil, errors.New("cross-ledger group lifecycle idempotency repository is not configured")
	}

	if group == nil || group.ID == uuid.Nil || group.OrganizationID == uuid.Nil || group.LedgerID == uuid.Nil || executionID == uuid.Nil {
		return nil, nil, errors.New("cross-ledger group lifecycle idempotency identity is incomplete")
	}

	var action string

	switch status {
	case constant.APPROVED:
		action = "commit"
	case constant.CANCELED:
		action = "cancel"
	default:
		return nil, nil, fmt.Errorf("unsupported cross-ledger group lifecycle idempotency status %q", status)
	}

	canonical, err := json.Marshal(struct {
		GroupID uuid.UUID       `json:"groupId"`
		Status  string          `json:"status"`
		Intent  json.RawMessage `json:"intent"`
	}{GroupID: group.ID, Status: status, Intent: group.Intent})
	if err != nil {
		return nil, nil, fmt.Errorf("encode cross-ledger group lifecycle idempotency identity: %w", err)
	}

	run := &atomicTransactionBatchRun{
		batchID:        group.ID,
		groupID:        cloneUUIDPointer(&group.ID),
		executionID:    executionID,
		organizationID: group.OrganizationID,
		ledgerID:       group.LedgerID,
		items:          make([]atomicTransactionBatchItemRun, len(plans)),
	}
	for index := range plans {
		if plans[index].TransactionID == uuid.Nil {
			return nil, nil, errors.New("cross-ledger group lifecycle idempotency transaction identity is incomplete")
		}

		run.items[index].transactionID = plans[index].TransactionID
	}

	replay, err := uc.claimAtomicTransactionBatch(ctx, CreateAtomicTransactionBatchV2Input{
		CanonicalRequest: canonical,
		IdempotencyKey:   "group-" + action + ":" + group.ID.String(),
	}, run)
	if err != nil {
		return nil, nil, err
	}

	return run, replay, nil
}

func (uc *UseCase) finalizeCrossLedgerGroupTransition(
	ctx context.Context,
	run *atomicTransactionBatchRun,
	transactions []*transaction.Transaction,
) error {
	if run == nil || len(run.items) != len(transactions) {
		return errors.New("cross-ledger group lifecycle idempotency response cardinality differs")
	}

	for index, tran := range transactions {
		if err := uc.captureAtomicTransactionBatchInitialResponse(ctx, run, run.items[index].transactionID, tran); err != nil {
			return err
		}
	}

	return uc.finalizeAtomicTransactionBatch(ctx, run, transactions)
}

func orderCrossLedgerPendingGroupParts(
	intent CrossLedgerGroupIntent,
	groupID, requestedID uuid.UUID,
	members []*transaction.Transaction,
) ([]crossLedgerPendingGroupPart, error) {
	originParts := make([]CrossLedgerGroupIntentPart, 0)

	for _, part := range intent.Parts {
		if part.Role == CrossLedgerGroupRoleOrigin {
			originParts = append(originParts, part)
		}
	}

	if len(originParts) == 0 || len(originParts) != len(members) {
		return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}

	byScope := make(map[atomicTransactionBatchLedgerRef]*transaction.Transaction, len(members))
	requestedFound := false

	for _, member := range members {
		if member == nil || member.GroupID == nil || *member.GroupID != groupID.String() {
			return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
		}

		organizationID, organizationErr := uuid.Parse(member.OrganizationID)
		ledgerID, ledgerErr := uuid.Parse(member.LedgerID)

		transactionID, transactionErr := uuid.Parse(member.ID)
		if organizationErr != nil || ledgerErr != nil || transactionErr != nil {
			return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
		}

		ref := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: ledgerID}
		if _, duplicate := byScope[ref]; duplicate {
			return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
		}

		byScope[ref] = member
		requestedFound = requestedFound || transactionID == requestedID
	}

	if !requestedFound {
		return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}

	ordered := make([]crossLedgerPendingGroupPart, len(originParts))
	for index, part := range originParts {
		ref := atomicTransactionBatchLedgerRef{organizationID: part.OrganizationID, ledgerID: part.LedgerID}

		member := byScope[ref]
		if member == nil {
			return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
		}

		ordered[index] = crossLedgerPendingGroupPart{intent: part, member: member}
	}

	return ordered, nil
}

func (uc *UseCase) lockCrossLedgerPendingGroup(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	parts []crossLedgerPendingGroupPart,
	status string,
) (func(), error) {
	lockOrder := make([]int, len(parts))
	for index := range parts {
		lockOrder[index] = index
	}

	sort.Slice(lockOrder, func(left, right int) bool {
		leftPart, rightPart := parts[lockOrder[left]], parts[lockOrder[right]]
		leftScope := leftPart.member.OrganizationID + ":" + leftPart.member.LedgerID + ":" + leftPart.member.ID
		rightScope := rightPart.member.OrganizationID + ":" + rightPart.member.LedgerID + ":" + rightPart.member.ID

		return leftScope < rightScope
	})

	unlocks := make([]func(), 0, len(parts))
	release := func() {
		for index := len(unlocks) - 1; index >= 0; index-- {
			unlocks[index]()
		}
	}

	for _, partIndex := range lockOrder {
		part := &parts[partIndex]
		part.run = &pendingTransitionRun{
			organizationID: part.intent.OrganizationID,
			ledgerID:       part.intent.LedgerID,
			tran:           part.member,
			status:         status,
		}

		unlock, err := uc.lockPendingTransaction(ctx, span, logger, part.run)
		if err != nil {
			release()
			return nil, err
		}

		unlocks = append(unlocks, unlock)
	}

	return release, nil
}

func (uc *UseCase) prepareCrossLedgerGroupDestinations(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	groupID uuid.UUID,
	intent CrossLedgerGroupIntent,
) (*atomicTransactionBatchRun, PreparedEngineExecution, error) {
	items := make([]CreateAtomicTransactionBatchV2ItemInput, 0)

	for _, part := range intent.Parts {
		if part.Role != CrossLedgerGroupRoleDestination {
			continue
		}

		index := len(items)
		items = append(items, CreateAtomicTransactionBatchV2ItemInput{
			OrganizationID: part.OrganizationID,
			LedgerID:       part.LedgerID,
			Transaction:    part.Transaction,
			Action:         constant.ActionDirect,
			Order:          index + 1,
			OriginalIndex:  index,
		})
	}

	if len(items) == 0 {
		return nil, PreparedEngineExecution{}, pkg.ValidateBusinessError(
			constant.ErrCrossLedgerGroupIncomplete,
			constant.EntityTransaction,
		)
	}

	input := CreateAtomicTransactionBatchV2Input{
		Transactions:     items,
		GroupID:          &groupID,
		CrossLedgerGroup: true,
	}

	run, err := uc.initializeAtomicTransactionBatchIdentity(ctx, input)
	if err != nil {
		return nil, PreparedEngineExecution{}, err
	}

	if err := uc.initializeAtomicTransactionBatchItemsAndSettings(ctx, input, run); err != nil {
		return nil, PreparedEngineExecution{}, err
	}

	if err := uc.prepareAtomicTransactionBatchItems(ctx, span, logger, run); err != nil {
		return nil, PreparedEngineExecution{}, err
	}

	prepared, err := buildAtomicTransactionBatchPreparedExecution(run)
	if err != nil {
		return nil, PreparedEngineExecution{}, err
	}

	return run, prepared, nil
}

func crossLedgerGroupExecutionID(uc *UseCase, destinations *atomicTransactionBatchRun) (uuid.UUID, error) {
	if destinations != nil {
		return destinations.executionID, nil
	}

	if uc.UUIDv7Generator == nil {
		return uuid.Nil, errors.New("cross-ledger group lifecycle UUIDv7 generator is not configured")
	}

	executionID, err := uc.UUIDv7Generator()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate cross-ledger group execution id: %w", err)
	}

	if executionID == uuid.Nil {
		return uuid.Nil, errors.New("cross-ledger group lifecycle UUIDv7 generator returned a nil id")
	}

	return executionID, nil
}

func (uc *UseCase) completeCrossLedgerGroupTransition(
	ctx context.Context,
	logger libLog.Logger,
	outcome EngineExecutionOutcome,
) ([]*transaction.Transaction, error) {
	envelopes, err := atomicTransactionBatchWriteBehindEnvelopes(outcome)
	if err != nil {
		return nil, err
	}

	transactions := make([]*transaction.Transaction, len(envelopes))
	for index, envelope := range envelopes {
		views, err := BuildTransactionEvidenceViews(envelope.Record)
		if err != nil {
			return nil, fmt.Errorf("compose cross-ledger group item %d: %w", index, err)
		}

		if views.Lookup == nil {
			return nil, invalidTransactionCompletionRecord("cross-ledger group evidence returned no lookup transaction")
		}

		transactions[index] = views.Lookup
	}

	dispatched := uc.TransactionWriteBehindAsync && uc.TransactionWriteBehindDispatcher != nil
	if dispatched {
		for _, envelope := range envelopes {
			if err := uc.TransactionWriteBehindDispatcher.DispatchTransactionWriteBehind(ctx, envelope); err != nil {
				dispatched = false
				break
			}
		}
	}

	if dispatched {
		return transactions, nil
	}

	completions, err := uc.completeAtomicTransactionBatchFallback(ctx, envelopes)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Cross-ledger group projection deferred to recovery", libLog.Err(err))
		return transactions, nil
	}

	for index, completion := range completions {
		expected := outcome.Prepared.CompletionPlans[index].TransactionStatus
		if expected == constant.CREATED {
			expected = constant.APPROVED
		}

		if completion.Outcome.TransactionStatus != expected {
			return nil, fmt.Errorf(
				"%w: cross-ledger group completer confirmed %q for item %d, expected %q",
				ErrTransactionCompletionConflict,
				completion.Outcome.TransactionStatus,
				index,
				expected,
			)
		}

		uc.acknowledgeEngineRecovery(ctx, logger, &envelopes[index].Record, completion)
	}

	return transactions, nil
}

func (uc *UseCase) settleCrossLedgerOriginReservations(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	status string,
	parts []crossLedgerPendingGroupPart,
) {
	for index := range parts {
		part := &parts[index]

		identity := part.run.reservationIdentity()
		if status == constant.APPROVED {
			uc.confirmReservationsByTransaction(
				ctx, span, logger, part.transition.ledgerSettings.Tracer, identity, part.transition.honoredTracerSkip,
			)
		} else {
			uc.releaseReservationsByTransaction(
				ctx, span, logger, part.transition.ledgerSettings.Tracer, identity, part.transition.honoredTracerSkip,
			)
		}
	}
}
