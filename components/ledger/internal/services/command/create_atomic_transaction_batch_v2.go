// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//nolint:wsl_v5 // ordered preparation keeps invariant phases visually distinct.
package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const atomicTransactionBatchAbsoluteMaxSize = 50

// UUIDv7Generator is the identity seam used by ordered batch orchestration.
type UUIDv7Generator func() (uuid.UUID, error)

// Clock is the time seam used by ordered batch orchestration.
type Clock func() time.Time

// CreateAtomicTransactionBatchV2ItemInput is one transaction in execution
// order. Action, Order and OriginalIndex are optional only for compatibility
// with the already-published direct-only batch route; the revised HTTP decoder
// always supplies all three and has already sorted by Order.
type CreateAtomicTransactionBatchV2ItemInput struct {
	OrganizationID          uuid.UUID
	LedgerID                uuid.UUID
	Transaction             mtransaction.Transaction
	ParentTransactionID     *uuid.UUID
	Dependencies            []TransactionEvidenceReference
	AccountBlockExceptionID *uuid.UUID
	Action                  string
	Order                   int
	OriginalIndex           int
}

// CreateAtomicTransactionBatchV2Input carries one ordered atomic request. The
// canonical bytes and idempotency settings are retained for the batch-level
// claim introduced by the later pre-publication phase.
type CreateAtomicTransactionBatchV2Input struct {
	Transactions       []CreateAtomicTransactionBatchV2ItemInput
	GroupID            *uuid.UUID
	CrossLedgerGroup   bool
	CanonicalRequest   []byte
	RequestFingerprint string
	IdempotencyKey     string
	IdempotencyTTL     time.Duration
}

// CreateAtomicTransactionBatchV2Result preserves request order and carries the
// batch replay state without creating a persisted batch domain resource.
type CreateAtomicTransactionBatchV2Result struct {
	BatchID      uuid.UUID
	Transactions []*transaction.Transaction
	Replayed     bool
}

// atomicTransactionBatchRun owns the batch-wide state. Order-sensitive state
// exists only in items; maps may be used by later phases for lookup, but never
// to rebuild this slice or determine execution order.
type atomicTransactionBatchRun struct {
	batchID                 uuid.UUID
	groupID                 *uuid.UUID
	executionID             uuid.UUID
	organizationID          uuid.UUID
	ledgerID                uuid.UUID
	ledgerSettings          mmodel.LedgerSettings
	idempotencyTTL          time.Duration
	idempotencyEffectiveKey string
	idempotencyFingerprint  string
	idempotencyOwnerToken   string
	idempotencyClaimed      bool
	idempotencyHandedOff    bool
	engineIntentFingerprint string
	rejectionDimension      string
	budgetMeasurements      *atomicTransactionBatchBudgetMeasurements
	items                   []atomicTransactionBatchItemRun
}

type atomicTransactionBatchLedgerRef struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
}

// atomicTransactionBatchItemRun owns the stable per-item identity and temporal
// context that all subsequent preparation, engine, completion, and response
// phases must consume at this same slice index.
type atomicTransactionBatchItemRun struct {
	index                   int
	order                   int
	originalIndex           int
	revised                 bool
	organizationID          uuid.UUID
	ledgerID                uuid.UUID
	ledgerSettings          mmodel.LedgerSettings
	transactionID           uuid.UUID
	transactionDate         time.Time
	transactionCreatedAt    time.Time
	transactionUpdatedAt    time.Time
	operationUpdatedAt      time.Time
	input                   mtransaction.Transaction
	status                  string
	parentTransactionID     *uuid.UUID
	dependencies            []TransactionEvidenceReference
	accountBlockExceptionID *uuid.UUID
	validate                *mtransaction.Responses
	fromTo                  []mtransaction.FromTo
	action                  string
	honoredFeeSkip          bool
	honoredTracerSkip       bool
	accountBlockGrant       *mtransaction.AccountBlockExceptionGrant
	prepared                enginePreparedTransaction
	tracerReservation       reservationHandle
	guard                   ExecutionGuard
	completionPlan          TransactionCompletionPlan
	completionPlanPayload   []byte
}

// CreateAtomicTransactionBatchV2 initializes the ordered batch command state.
// Later preparation phases extend this coordinator before the HTTP route is
// registered; this foundation deliberately performs no accounting mutation.
func (uc *UseCase) CreateAtomicTransactionBatchV2(
	ctx context.Context,
	in CreateAtomicTransactionBatchV2Input,
) (result *CreateAtomicTransactionBatchV2Result, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_atomic_transaction_batch_v2")
	defer span.End()

	startedAt := time.Now()
	scope := atomicTransactionBatchScope(in.CrossLedgerGroup)

	uc.recordAtomicTransactionBatchReceived(ctx, in)

	var run *atomicTransactionBatchRun
	defer func() {
		uc.recordAtomicTransactionBatchCompleted(ctx, scope, result, run, err, time.Since(startedAt))
	}()

	phaseStartedAt := time.Now()
	run, err = uc.initializeAtomicTransactionBatchIdentity(ctx, in)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "identity", time.Since(phaseStartedAt))

	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to initialize atomic transaction batch", err)
		return nil, err
	}

	phaseStartedAt = time.Now()
	replay, err := uc.claimAtomicTransactionBatch(ctx, in, run)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "idempotency", time.Since(phaseStartedAt))

	if err != nil {
		return nil, err
	}

	if replay != nil {
		return replay, nil
	}

	// Cache-miss seeds are admitted inside the engine, so keep every account
	// ownership taken by the batch loads until the single execution answers.
	ctx, admissions := accountprotection.ContextWithSink(ctx)
	defer admissions.Release(ctx)

	phaseStartedAt = time.Now()
	if err := uc.initializeAtomicTransactionBatchItemsAndSettings(ctx, in, run); err != nil {
		uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "preparation", time.Since(phaseStartedAt))
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, run, err)
	}

	if err := uc.prepareAtomicTransactionBatchItems(ctx, span, logger, run); err != nil {
		uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "preparation", time.Since(phaseStartedAt))
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, run, err)
	}

	prepared, err := buildAtomicTransactionBatchPreparedExecution(run)

	uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "preparation", time.Since(phaseStartedAt))

	if err != nil {
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, run, err)
	}

	if uc.Engine == nil {
		return nil, uc.abortAtomicTransactionBatchPrePublication(
			ctx,
			run,
			errors.New("atomic transaction batch engine is not configured"),
		)
	}

	if isNilAppliedTransactionCompleter(uc.AppliedTransactionCompleter) {
		return nil, uc.abortAtomicTransactionBatchPrePublication(
			ctx,
			run,
			errors.New("atomic transaction batch completer is not configured"),
		)
	}

	if err := uc.prepareAtomicTransactionBatchIdempotency(ctx, run); err != nil {
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, run, err)
	}

	phaseStartedAt = time.Now()
	if err := uc.reserveAtomicTransactionBatch(ctx, span, logger, run); err != nil {
		uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "reservation", time.Since(phaseStartedAt))
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, run, err)
	}

	uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "reservation", time.Since(phaseStartedAt))

	if err := uc.handoffAtomicTransactionBatchExecution(ctx, run); err != nil {
		return nil, err
	}

	phaseStartedAt = time.Now()
	outcome, err := uc.executeAtomicTransactionBatch(ctx, span, logger, run, prepared, admissions)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "accounting", time.Since(phaseStartedAt))

	if err != nil {
		return nil, err
	}

	phaseStartedAt = time.Now()
	transactions, err := uc.completeAtomicTransactionBatch(ctx, logger, run, outcome)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, scope, "completion", time.Since(phaseStartedAt))

	if err != nil {
		return nil, err
	}

	result = &CreateAtomicTransactionBatchV2Result{
		BatchID:      run.batchID,
		Transactions: transactions,
		Replayed:     false,
	}

	return result, nil
}

func (uc *UseCase) initializeAtomicTransactionBatchIdentity(
	ctx context.Context,
	in CreateAtomicTransactionBatchV2Input,
) (*atomicTransactionBatchRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	organizationID, ledgerID, err := validateAtomicTransactionBatchScope(in.Transactions, in.CrossLedgerGroup)
	if err != nil {
		return nil, err
	}
	if err := validateAtomicTransactionBatchItemCorrelationForGroup(in.Transactions, in.CrossLedgerGroup); err != nil {
		return nil, err
	}

	if uc.UUIDv7Generator == nil {
		return nil, errors.New("atomic transaction batch UUIDv7 generator is not configured")
	}

	var batchID uuid.UUID
	if in.GroupID != nil {
		batchID = *in.GroupID
	} else {
		batchID, err = uc.UUIDv7Generator()
		if err != nil {
			return nil, fmt.Errorf("generate atomic transaction batch id: %w", err)
		}
	}

	if batchID == uuid.Nil {
		return nil, errors.New("atomic transaction batch UUIDv7 generator returned a nil batch id")
	}

	return &atomicTransactionBatchRun{
		batchID:        batchID,
		groupID:        in.GroupID,
		organizationID: organizationID,
		ledgerID:       ledgerID,
		idempotencyTTL: in.IdempotencyTTL,
	}, nil
}

func (uc *UseCase) initializeAtomicTransactionBatchItemsAndSettings(
	ctx context.Context,
	in CreateAtomicTransactionBatchV2Input,
	run *atomicTransactionBatchRun,
) error {
	if uc.Clock == nil {
		return errors.New("atomic transaction batch clock is not configured")
	}

	if uc.TransactionReader == nil {
		return errors.New("atomic transaction batch transaction reader is not configured")
	}

	run.items = make([]atomicTransactionBatchItemRun, len(in.Transactions))

	cursor := atomicTransactionBatchTimestampCursor{clock: uc.Clock}
	for index := range in.Transactions {
		item, itemErr := initializeAtomicTransactionBatchItem(in.Transactions[index], index, uc.UUIDv7Generator, &cursor)
		if itemErr != nil {
			return itemErr
		}

		run.items[index] = item
	}

	refs := atomicTransactionBatchLedgerRefs(in.Transactions)
	settingsByRef := make(map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings, len(refs))
	for _, ref := range refs {
		settings, err := uc.TransactionReader.GetParsedLedgerSettings(ctx, ref.organizationID, ref.ledgerID)
		if err != nil {
			return fmt.Errorf("get atomic transaction batch ledger settings: %w", err)
		}
		if len(refs) > 1 && !settings.CrossLedger.Enabled {
			return pkg.ValidateBusinessError(constant.ErrCrossLedgerNotEnabled, constant.EntityLedger, ref.ledgerID.String())
		}
		if in.CrossLedgerGroup && settings.Accounting.ValidateRoutes {
			return pkg.ValidateBusinessError(constant.ErrCrossLedgerRouteValidationUnsupported, constant.EntityLedger)
		}
		settingsByRef[ref] = settings
	}
	run.ledgerSettings = settingsByRef[refs[0]]
	for index := range run.items {
		item := &run.items[index]
		item.ledgerSettings = settingsByRef[atomicTransactionBatchLedgerRef{organizationID: item.organizationID, ledgerID: item.ledgerID}]
	}

	// State-dependent validation starts only after the complete ordered run is
	// frozen. It stops at the first actual failure and never evaluates later
	// items speculatively.
	for index := range run.items {
		item := &run.items[index]

		var err error
		item.transactionDate, err = resolveTransactionDateAt(item.input, item.status, item.transactionCreatedAt)
		if err != nil {
			return withAtomicTransactionBatchRunItemError(err, item, "transaction date validation failed")
		}
	}

	return nil
}

func (uc *UseCase) prepareAtomicTransactionBatchItems(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	run *atomicTransactionBatchRun,
) error {
	for index := range run.items {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := uc.prepareAtomicTransactionBatchItem(ctx, span, logger, &run.items[index]); err != nil {
			return withAtomicTransactionBatchRunItemError(err, &run.items[index], "transaction preparation failed")
		}
	}

	if err := uc.enforceAtomicTransactionBatchExpandedPostings(run); err != nil {
		return err
	}

	if err := uc.prepareAtomicTransactionBatchEngineItems(ctx, run); err != nil {
		return err
	}

	return uc.prepareAndEnforceAtomicTransactionBatchBudgets(ctx, run)
}

func (uc *UseCase) prepareAtomicTransactionBatchItem(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	item *atomicTransactionBatchItemRun,
) error {
	if err := validatePositiveTransactionValue(ctx, span, logger, item.input.Send.Value); err != nil {
		return err
	}

	mtransaction.ApplyDefaultBalanceKeys(item.input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(item.input.Send.Distribute.To)

	if _, err := mtransaction.ValidateSendSourceAndDistribute(ctx, item.input, item.status); err != nil {
		return pkg.HandleKnownBusinessValidationErrors(err)
	}

	feeSkip, tracerSkip, _, err := resolveTransactionSkips(item.input, item.ledgerSettings)
	if err != nil {
		return err
	}

	item.honoredFeeSkip = feeSkip
	item.honoredTracerSkip = tracerSkip

	if item.action != constant.ActionRevert {
		if err := uc.applyFees(
			ctx,
			&item.input,
			item.organizationID,
			item.ledgerID,
			item.input.Pending,
			item.honoredFeeSkip,
		); err != nil {
			return err
		}
	}

	normalizeTransactionSendLegs(&item.input)

	item.validate, err = mtransaction.ValidateSendSourceAndDistribute(ctx, item.input, item.status)
	if err != nil {
		return pkg.HandleKnownBusinessValidationErrors(err)
	}

	item.fromTo = append(item.fromTo, mtransaction.MutateConcatAliases(item.input.Send.Source.From)...)

	item.fromTo = append(item.fromTo, mtransaction.MutateConcatAliases(item.input.Send.Distribute.To)...)
	if item.ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, item.validate, item.status)
	}

	if item.action == "" {
		item.action = mtransaction.StatusToAction(item.status)
	}

	item.accountBlockGrant, err = uc.resolveAccountBlockExceptionGrant(
		ctx,
		span,
		logger,
		item.organizationID,
		item.ledgerID,
		item.accountBlockExceptionID,
	)
	if err != nil {
		return err
	}

	return nil
}

func (uc *UseCase) prepareAtomicTransactionBatchEngineItems(
	ctx context.Context,
	run *atomicTransactionBatchRun,
) error {
	readCtx := readrouting.WithPrimaryRead(ctx)
	refs, aliasesByRef := firstSeenAtomicTransactionBatchAliasesByLedger(run)
	pools := make(map[atomicTransactionBatchLedgerRef]EngineSnapshotPool, len(refs))
	for _, ref := range refs {
		pool, err := loadPreparedEngineSnapshots(readCtx, uc.TransactionReader, ref.organizationID, ref.ledgerID, aliasesByRef[ref])
		if err != nil {
			return err
		}
		pools[ref] = pool
	}

	for index := range run.items {
		item := &run.items[index]
		preparation := createEnginePreparationInput(run.createTransactionRun(item))
		ref := atomicTransactionBatchLedgerRef{organizationID: item.organizationID, ledgerID: item.ledgerID}
		var err error
		item.prepared, err = uc.prepareEngineTransactionWithPool(readCtx, preparation, pools[ref])
		if err != nil {
			return withAtomicTransactionBatchRunItemError(err, item, "transaction preparation failed")
		}
	}

	return nil
}

func firstSeenAtomicTransactionBatchAliasesByLedger(run *atomicTransactionBatchRun) ([]atomicTransactionBatchLedgerRef, map[atomicTransactionBatchLedgerRef][]string) {
	seenRefs := make(map[atomicTransactionBatchLedgerRef]struct{})
	seenAliases := make(map[atomicTransactionBatchLedgerRef]map[string]struct{})
	refs := make([]atomicTransactionBatchLedgerRef, 0)
	aliasesByRef := make(map[atomicTransactionBatchLedgerRef][]string)

	for index := range run.items {
		item := &run.items[index]
		ref := atomicTransactionBatchLedgerRef{organizationID: item.organizationID, ledgerID: item.ledgerID}
		if _, exists := seenRefs[ref]; !exists {
			seenRefs[ref] = struct{}{}
			seenAliases[ref] = make(map[string]struct{})
			refs = append(refs, ref)
		}
		preparation := createEnginePreparationInput(run.createTransactionRun(item))
		for _, alias := range enginePreparationAliases(preparation) {
			if _, exists := seenAliases[ref][alias]; exists {
				continue
			}

			seenAliases[ref][alias] = struct{}{}
			aliasesByRef[ref] = append(aliasesByRef[ref], alias)
		}
	}

	return refs, aliasesByRef
}

func (run *atomicTransactionBatchRun) createTransactionRun(item *atomicTransactionBatchItemRun) *createTransactionRun {
	organizationID, ledgerID := run.itemScope(item)
	return &createTransactionRun{
		organizationID:             organizationID,
		ledgerID:                   ledgerID,
		transactionID:              item.transactionID,
		transactionDate:            item.transactionDate,
		input:                      item.input,
		status:                     item.status,
		action:                     item.action,
		parentTransactionID:        uuidPointerValue(item.parentTransactionID),
		dependencies:               append([]TransactionEvidenceReference(nil), item.dependencies...),
		validate:                   item.validate,
		fromTo:                     item.fromTo,
		ledgerSettings:             run.itemLedgerSettings(item),
		idempotencyTTL:             run.idempotencyTTL,
		honoredFeeSkip:             item.honoredFeeSkip,
		honoredTracerSkip:          item.honoredTracerSkip,
		accountBlockExceptionID:    item.accountBlockExceptionID,
		accountBlockExceptionGrant: item.accountBlockGrant,
	}
}

func (run *atomicTransactionBatchRun) itemScope(item *atomicTransactionBatchItemRun) (uuid.UUID, uuid.UUID) {
	if item.organizationID == uuid.Nil && item.ledgerID == uuid.Nil {
		return run.organizationID, run.ledgerID
	}

	return item.organizationID, item.ledgerID
}

func (run *atomicTransactionBatchRun) itemLedgerSettings(item *atomicTransactionBatchItemRun) mmodel.LedgerSettings {
	if item.organizationID == uuid.Nil && item.ledgerID == uuid.Nil {
		return run.ledgerSettings
	}

	return item.ledgerSettings
}

func validateAtomicTransactionBatchScope(items []CreateAtomicTransactionBatchV2ItemInput, crossLedgerGroup bool) (uuid.UUID, uuid.UUID, error) {
	if len(items) == 0 || len(items) > atomicTransactionBatchAbsoluteMaxSize {
		return uuid.Nil, uuid.Nil, pkg.ValidateBusinessError(
			constant.ErrTransactionBatchCardinality,
			constant.EntityTransaction,
			len(items),
			atomicTransactionBatchAbsoluteMaxSize,
		)
	}

	organizationID := items[0].OrganizationID
	ledgerID := items[0].LedgerID
	refs := atomicTransactionBatchLedgerRefs(items)
	for index := range items {
		if items[index].OrganizationID == uuid.Nil || items[index].LedgerID == uuid.Nil {
			err := pkg.ValidateBusinessError(constant.ErrTransactionScopeMismatch, constant.EntityTransaction)

			return uuid.Nil, uuid.Nil, withAtomicTransactionBatchItemError(
				err,
				index,
				"transaction scope must be complete",
			)
		}
	}
	if len(refs) > 1 {
		for index, item := range items {
			action := item.Action
			if action == "" {
				action = constant.ActionDirect
			}
			if action != constant.ActionDirect && (!crossLedgerGroup || (action != constant.ActionRevert && action != constant.ActionHold)) {
				err := pkg.ValidateBusinessError(constant.ErrTransactionScopeMismatch, constant.EntityTransaction)
				message := "cross-ledger action is not supported"
				if action == constant.ActionHold {
					message = "cross-ledger hold is not supported"
				}

				return uuid.Nil, uuid.Nil, withAtomicTransactionBatchItemError(err, index, message)
			}
		}
	}

	return organizationID, ledgerID, nil
}

func atomicTransactionBatchLedgerRefs(items []CreateAtomicTransactionBatchV2ItemInput) []atomicTransactionBatchLedgerRef {
	seen := make(map[atomicTransactionBatchLedgerRef]struct{}, len(items))
	refs := make([]atomicTransactionBatchLedgerRef, 0, len(items))
	for _, item := range items {
		ref := atomicTransactionBatchLedgerRef{organizationID: item.OrganizationID, ledgerID: item.LedgerID}
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func validateAtomicTransactionBatchItemCorrelationForGroup(items []CreateAtomicTransactionBatchV2ItemInput, crossLedgerGroup bool) error {
	revised := false
	for _, item := range items {
		if item.Action != "" || item.Order != 0 || item.OriginalIndex != 0 {
			revised = true
			break
		}
	}
	if !revised {
		return nil
	}

	seenOriginalIndexes := make(map[int]struct{}, len(items))
	for index, item := range items {
		if item.Order != index+1 {
			return fmt.Errorf("atomic transaction batch item %d order must be %d", index, index+1)
		}
		if item.OriginalIndex < 0 || item.OriginalIndex >= len(items) {
			return fmt.Errorf("atomic transaction batch item %d has invalid original index %d", index, item.OriginalIndex)
		}
		if _, exists := seenOriginalIndexes[item.OriginalIndex]; exists {
			return fmt.Errorf("atomic transaction batch item %d repeats original index %d", index, item.OriginalIndex)
		}
		seenOriginalIndexes[item.OriginalIndex] = struct{}{}
		if item.Action != constant.ActionDirect && item.Action != constant.ActionHold && (!crossLedgerGroup || item.Action != constant.ActionRevert) {
			return fmt.Errorf("atomic transaction batch item %d has unsupported action %q", index, item.Action)
		}
	}

	return nil
}

func initializeAtomicTransactionBatchItem(
	in CreateAtomicTransactionBatchV2ItemInput,
	index int,
	generateUUIDv7 UUIDv7Generator,
	cursor *atomicTransactionBatchTimestampCursor,
) (atomicTransactionBatchItemRun, error) {
	action := in.Action
	if action == "" {
		action = constant.ActionDirect
	}

	transactionID, err := generateUUIDv7()
	if err != nil {
		return atomicTransactionBatchItemRun{}, fmt.Errorf("generate atomic transaction batch item %d id: %w", index, err)
	}

	if transactionID == uuid.Nil {
		return atomicTransactionBatchItemRun{}, fmt.Errorf("atomic transaction batch UUIDv7 generator returned a nil id for item %d", index)
	}

	input, err := clonePendingTransactionInput(in.Transaction)
	if err != nil {
		return atomicTransactionBatchItemRun{}, fmt.Errorf("clone atomic transaction batch item %d: %w", index, err)
	}
	input.Pending = action == constant.ActionHold

	createdAt, err := cursor.next()
	if err != nil {
		return atomicTransactionBatchItemRun{}, fmt.Errorf("freeze atomic transaction batch item %d creation time: %w", index, err)
	}

	updatedAt, err := cursor.next()
	if err != nil {
		return atomicTransactionBatchItemRun{}, fmt.Errorf("freeze atomic transaction batch item %d update time: %w", index, err)
	}

	operationUpdatedAt, err := cursor.next()
	if err != nil {
		return atomicTransactionBatchItemRun{}, fmt.Errorf("freeze atomic transaction batch item %d operation time: %w", index, err)
	}

	return atomicTransactionBatchItemRun{
		index:                   index,
		order:                   atomicTransactionBatchItemOrder(in, index),
		originalIndex:           atomicTransactionBatchItemOriginalIndex(in, index),
		revised:                 atomicTransactionBatchItemIsRevised(in),
		organizationID:          in.OrganizationID,
		ledgerID:                in.LedgerID,
		transactionID:           transactionID,
		transactionCreatedAt:    createdAt,
		transactionUpdatedAt:    updatedAt,
		operationUpdatedAt:      operationUpdatedAt,
		input:                   input,
		status:                  atomicTransactionBatchActionInitialStatus(action),
		action:                  action,
		parentTransactionID:     cloneUUIDPointer(in.ParentTransactionID),
		dependencies:            append([]TransactionEvidenceReference(nil), in.Dependencies...),
		accountBlockExceptionID: cloneUUIDPointer(in.AccountBlockExceptionID),
	}, nil
}

func atomicTransactionBatchItemOrder(in CreateAtomicTransactionBatchV2ItemInput, index int) int {
	if in.Order == 0 {
		return index + 1
	}

	return in.Order
}

func atomicTransactionBatchItemOriginalIndex(in CreateAtomicTransactionBatchV2ItemInput, index int) int {
	if !atomicTransactionBatchItemIsRevised(in) {
		return index
	}

	return in.OriginalIndex
}

func atomicTransactionBatchItemIsRevised(in CreateAtomicTransactionBatchV2ItemInput) bool {
	return in.Action != "" || in.Order != 0 || in.OriginalIndex != 0
}

func atomicTransactionBatchActionInitialStatus(action string) string {
	if action == constant.ActionHold {
		return constant.PENDING
	}

	return constant.CREATED
}

type atomicTransactionBatchTimestampCursor struct {
	clock Clock
	last  time.Time
}

func (cursor *atomicTransactionBatchTimestampCursor) next() (time.Time, error) {
	next := cursor.clock()
	if next.IsZero() {
		return time.Time{}, errors.New("clock returned a zero timestamp")
	}

	if !cursor.last.IsZero() && next.Before(cursor.last) {
		next = cursor.last
	}

	cursor.last = next

	return next, nil
}

func atomicTransactionBatchFoundationResult(item *atomicTransactionBatchItemRun) *transaction.Transaction {
	amount := item.input.Send.Value
	status := item.status
	var groupID *string
	if item.completionPlan.GroupID != nil {
		value := item.completionPlan.GroupID.String()
		groupID = &value
	}

	return &transaction.Transaction{
		ID:                       item.transactionID.String(),
		ParentTransactionID:      uuidStringPointer(item.completionPlan.ParentTransactionID),
		GroupID:                  groupID,
		Description:              item.input.Description,
		Status:                   transaction.Status{Code: status, Description: &status},
		Amount:                   &amount,
		AssetCode:                item.input.Send.Asset,
		ChartOfAccountsGroupName: item.input.ChartOfAccountsGroupName,
		Source:                   atomicTransactionBatchAliases(item.input.Send.Source.From),
		Destination:              atomicTransactionBatchAliases(item.input.Send.Distribute.To),
		LedgerID:                 item.ledgerID.String(),
		OrganizationID:           item.organizationID.String(),
		Body:                     item.input,
		Route:                    item.input.Route, //nolint:staticcheck // compatibility field mirrors singular transaction output
		RouteID:                  item.input.RouteID,
		FeesSkipped:              item.honoredFeeSkip,
		TracerSkipped:            item.honoredTracerSkip,
		CreatedAt:                item.transactionDate,
		UpdatedAt:                item.transactionUpdatedAt,
		Metadata:                 item.input.Metadata,
		Operations:               make([]*operation.Operation, 0),
	}
}

func atomicTransactionBatchAliases(entries []mtransaction.FromTo) []string {
	aliases := make([]string, len(entries))
	for index := range entries {
		aliases[index] = mtransaction.SplitAlias(entries[index].AccountAlias)
	}

	return aliases
}

func withAtomicTransactionBatchItemError(primary error, index int, message string) error {
	return pkg.WithFieldErrors(primary, []pkg.FieldError{{
		Location: fmt.Sprintf("body.transactions[%d]", index),
		Message:  message,
	}})
}

func withAtomicTransactionBatchRunItemError(primary error, item *atomicTransactionBatchItemRun, message string) error {
	if item == nil {
		return primary
	}
	if !item.revised {
		return withAtomicTransactionBatchItemError(primary, item.originalIndex, message)
	}

	return pkg.WithFieldErrors(primary, []pkg.FieldError{{
		Location: fmt.Sprintf("body.transactions[%d]", item.originalIndex),
		Message:  fmt.Sprintf("Transaction order %d: %s", item.order, message),
	}})
}

func cloneUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

func uuidPointerValue(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}

	return *value
}

func uuidStringPointer(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}

	text := value.String()

	return &text
}
