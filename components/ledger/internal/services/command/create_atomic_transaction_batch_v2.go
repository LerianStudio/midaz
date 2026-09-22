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

// atomicTransactionBatchItemRun owns the stable per-item identity and temporal
// context that all subsequent preparation, engine, completion, and response
// phases must consume at this same slice index.
type atomicTransactionBatchItemRun struct {
	index                   int
	order                   int
	originalIndex           int
	revised                 bool
	transactionID           uuid.UUID
	transactionDate         time.Time
	transactionCreatedAt    time.Time
	transactionUpdatedAt    time.Time
	operationUpdatedAt      time.Time
	input                   mtransaction.Transaction
	status                  string
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

	uc.recordAtomicTransactionBatchReceived(ctx, in)

	var run *atomicTransactionBatchRun
	defer func() {
		uc.recordAtomicTransactionBatchCompleted(ctx, result, run, err, time.Since(startedAt))
	}()

	phaseStartedAt := time.Now()
	run, err = uc.initializeAtomicTransactionBatchIdentity(ctx, in)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, "identity", time.Since(phaseStartedAt))

	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to initialize atomic transaction batch", err)
		return nil, err
	}

	phaseStartedAt = time.Now()
	replay, err := uc.claimAtomicTransactionBatch(ctx, in, run)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, "idempotency", time.Since(phaseStartedAt))

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
		uc.recordAtomicTransactionBatchPhaseDuration(ctx, "preparation", time.Since(phaseStartedAt))
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, run, err)
	}

	if err := uc.prepareAtomicTransactionBatchItems(ctx, span, logger, run); err != nil {
		uc.recordAtomicTransactionBatchPhaseDuration(ctx, "preparation", time.Since(phaseStartedAt))
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, run, err)
	}

	prepared, err := buildAtomicTransactionBatchPreparedExecution(run)

	uc.recordAtomicTransactionBatchPhaseDuration(ctx, "preparation", time.Since(phaseStartedAt))

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
		uc.recordAtomicTransactionBatchPhaseDuration(ctx, "reservation", time.Since(phaseStartedAt))
		return nil, uc.abortAtomicTransactionBatchPrePublication(ctx, run, err)
	}

	uc.recordAtomicTransactionBatchPhaseDuration(ctx, "reservation", time.Since(phaseStartedAt))

	if err := uc.handoffAtomicTransactionBatchExecution(ctx, run); err != nil {
		return nil, err
	}

	phaseStartedAt = time.Now()
	outcome, err := uc.executeAtomicTransactionBatch(ctx, span, logger, run, prepared, admissions)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, "accounting", time.Since(phaseStartedAt))

	if err != nil {
		return nil, err
	}

	phaseStartedAt = time.Now()
	transactions, err := uc.completeAtomicTransactionBatch(ctx, logger, run, outcome)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, "completion", time.Since(phaseStartedAt))

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

	organizationID, ledgerID, err := validateAtomicTransactionBatchScope(in.Transactions)
	if err != nil {
		return nil, err
	}
	if err := validateAtomicTransactionBatchItemCorrelation(in.Transactions); err != nil {
		return nil, err
	}

	if uc.UUIDv7Generator == nil {
		return nil, errors.New("atomic transaction batch UUIDv7 generator is not configured")
	}

	batchID, err := uc.UUIDv7Generator()
	if err != nil {
		return nil, fmt.Errorf("generate atomic transaction batch id: %w", err)
	}

	if batchID == uuid.Nil {
		return nil, errors.New("atomic transaction batch UUIDv7 generator returned a nil batch id")
	}

	return &atomicTransactionBatchRun{
		batchID:        batchID,
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

	var err error

	run.ledgerSettings, err = uc.TransactionReader.GetParsedLedgerSettings(ctx, run.organizationID, run.ledgerID)
	if err != nil {
		return fmt.Errorf("get atomic transaction batch ledger settings: %w", err)
	}

	// State-dependent validation starts only after the complete ordered run is
	// frozen. It stops at the first actual failure and never evaluates later
	// items speculatively.
	for index := range run.items {
		item := &run.items[index]

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

		if err := uc.prepareAtomicTransactionBatchItem(ctx, span, logger, run, &run.items[index]); err != nil {
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
	run *atomicTransactionBatchRun,
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

	feeSkip, tracerSkip, _, err := resolveTransactionSkips(item.input, run.ledgerSettings)
	if err != nil {
		return err
	}

	item.honoredFeeSkip = feeSkip
	item.honoredTracerSkip = tracerSkip

	if err := uc.applyFees(
		ctx,
		&item.input,
		run.organizationID,
		run.ledgerID,
		item.input.Pending,
		item.honoredFeeSkip,
	); err != nil {
		return err
	}

	normalizeTransactionSendLegs(&item.input)

	item.validate, err = mtransaction.ValidateSendSourceAndDistribute(ctx, item.input, item.status)
	if err != nil {
		return pkg.HandleKnownBusinessValidationErrors(err)
	}

	item.fromTo = append(item.fromTo, mtransaction.MutateConcatAliases(item.input.Send.Source.From)...)

	item.fromTo = append(item.fromTo, mtransaction.MutateConcatAliases(item.input.Send.Distribute.To)...)
	if run.ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, item.validate, item.status)
	}

	item.action = mtransaction.StatusToAction(item.status)

	item.accountBlockGrant, err = uc.resolveAccountBlockExceptionGrant(
		ctx,
		span,
		logger,
		run.organizationID,
		run.ledgerID,
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
	aliases := firstSeenAtomicTransactionBatchAliases(run)
	readCtx := readrouting.WithPrimaryRead(ctx)

	sharedPool, err := loadPreparedEngineSnapshots(
		readCtx,
		uc.TransactionReader,
		run.organizationID,
		run.ledgerID,
		aliases,
	)
	if err != nil {
		return err
	}

	for index := range run.items {
		item := &run.items[index]
		preparation := createEnginePreparationInput(run.createTransactionRun(item))

		item.prepared, err = uc.prepareEngineTransactionWithPool(readCtx, preparation, sharedPool)
		if err != nil {
			return withAtomicTransactionBatchRunItemError(err, item, "transaction preparation failed")
		}
	}

	return nil
}

func firstSeenAtomicTransactionBatchAliases(run *atomicTransactionBatchRun) []string {
	seen := make(map[string]struct{})
	aliases := make([]string, 0)

	for index := range run.items {
		preparation := createEnginePreparationInput(run.createTransactionRun(&run.items[index]))
		for _, alias := range enginePreparationAliases(preparation) {
			if _, exists := seen[alias]; exists {
				continue
			}

			seen[alias] = struct{}{}
			aliases = append(aliases, alias)
		}
	}

	return aliases
}

func (run *atomicTransactionBatchRun) createTransactionRun(item *atomicTransactionBatchItemRun) *createTransactionRun {
	return &createTransactionRun{
		organizationID:             run.organizationID,
		ledgerID:                   run.ledgerID,
		transactionID:              item.transactionID,
		transactionDate:            item.transactionDate,
		input:                      item.input,
		status:                     item.status,
		action:                     item.action,
		validate:                   item.validate,
		fromTo:                     item.fromTo,
		ledgerSettings:             run.ledgerSettings,
		idempotencyTTL:             run.idempotencyTTL,
		honoredFeeSkip:             item.honoredFeeSkip,
		honoredTracerSkip:          item.honoredTracerSkip,
		accountBlockExceptionID:    item.accountBlockExceptionID,
		accountBlockExceptionGrant: item.accountBlockGrant,
	}
}

func validateAtomicTransactionBatchScope(items []CreateAtomicTransactionBatchV2ItemInput) (uuid.UUID, uuid.UUID, error) {
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
	for index := range items {
		if items[index].OrganizationID == uuid.Nil || items[index].LedgerID == uuid.Nil ||
			items[index].OrganizationID != organizationID || items[index].LedgerID != ledgerID {
			err := pkg.ValidateBusinessError(constant.ErrTransactionScopeMismatch, constant.EntityTransaction)

			return uuid.Nil, uuid.Nil, withAtomicTransactionBatchItemError(
				err,
				index,
				"transaction scope must match the first batch item",
			)
		}
	}

	return organizationID, ledgerID, nil
}

func validateAtomicTransactionBatchItemCorrelation(items []CreateAtomicTransactionBatchV2ItemInput) error {
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
		if item.Action != constant.ActionDirect && item.Action != constant.ActionHold {
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
		transactionID:           transactionID,
		transactionCreatedAt:    createdAt,
		transactionUpdatedAt:    updatedAt,
		operationUpdatedAt:      operationUpdatedAt,
		input:                   input,
		status:                  atomicTransactionBatchActionInitialStatus(action),
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

func atomicTransactionBatchFoundationResult(run *atomicTransactionBatchRun, item *atomicTransactionBatchItemRun) *transaction.Transaction {
	amount := item.input.Send.Value
	status := item.status

	return &transaction.Transaction{
		ID:                       item.transactionID.String(),
		Description:              item.input.Description,
		Status:                   transaction.Status{Code: status, Description: &status},
		Amount:                   &amount,
		AssetCode:                item.input.Send.Asset,
		ChartOfAccountsGroupName: item.input.ChartOfAccountsGroupName,
		Source:                   atomicTransactionBatchAliases(item.input.Send.Source.From),
		Destination:              atomicTransactionBatchAliases(item.input.Send.Distribute.To),
		LedgerID:                 run.ledgerID.String(),
		OrganizationID:           run.organizationID.String(),
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
