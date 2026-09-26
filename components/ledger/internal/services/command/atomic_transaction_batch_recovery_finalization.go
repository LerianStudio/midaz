// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//nolint:wsl_v5 // recovery phases are deliberately separated by durable evidence boundaries.
package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// AtomicTransactionBatchProjectionReader is the recovery-facing query seam.
// It returns durable transactions with operations and metadata for the exact
// scoped ID set; the caller restores request order from the idempotency record.
type AtomicTransactionBatchProjectionReader interface {
	GetAtomicTransactionBatchProjections(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		transactionIDs []uuid.UUID,
	) ([]*transaction.Transaction, error)
}

type atomicTransactionBatchFinalizationCandidateRepository interface {
	GetAtomicTransactionBatchFinalizationCandidate(
		ctx context.Context,
		organizationID, ledgerID, executionID, transactionID uuid.UUID,
	) (*txRedis.AtomicTransactionBatchFinalizationCandidateResult, error)
}

// AtomicTransactionBatchRecoveryFinalization is passed to the Redis protected
// ACK. An empty ReceiptToken means this member was not a finalization candidate
// at read time; Lua may acknowledge it unless a concurrent ACK made it last.
type AtomicTransactionBatchRecoveryFinalization struct {
	ReceiptToken string
	Transactions map[uuid.UUID]json.RawMessage
}

// AtomicTransactionBatchRecoveryFinalizer is the only batch-specific command
// capability exposed to the recovery consumer. It has no accounting engine
// method and cannot reapply balance movements.
type AtomicTransactionBatchRecoveryFinalizer interface {
	PrepareAtomicTransactionBatchRecoveryFinalization(
		ctx context.Context,
		record *TransactionCompletionRecord,
		completion TransactionCompletionResult,
	) (*AtomicTransactionBatchRecoveryFinalization, error)
}

// PrepareAtomicTransactionBatchRecoveryFinalization resolves the execution
// index, reconciles Tracer for the durable member by stable identity, and only
// reads all projections when the frozen receipt proves every other member was
// already acknowledged. The Redis ACK revalidates the exact receipt token.
//
//nolint:gocyclo // each branch protects a distinct recovery compatibility invariant.
func (uc *UseCase) PrepareAtomicTransactionBatchRecoveryFinalization(
	ctx context.Context,
	record *TransactionCompletionRecord,
	completion TransactionCompletionResult,
) (*AtomicTransactionBatchRecoveryFinalization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if record == nil || record.OrganizationID == uuid.Nil || record.LedgerID == uuid.Nil ||
		record.ExecutionID == uuid.Nil || record.TransactionID == uuid.Nil {
		return nil, errors.New("atomic transaction batch recovery identity is incomplete")
	}

	repository, ok := uc.AtomicTransactionBatchIdempotencyRepo.(atomicTransactionBatchFinalizationCandidateRepository)
	if !ok {
		return nil, errors.New("atomic transaction batch recovery finalizer is not configured")
	}

	coordinationOrganizationID, coordinationLedgerID := record.OrganizationID, record.LedgerID
	if record.CoordinationOrganizationID != nil && record.CoordinationLedgerID != nil {
		coordinationOrganizationID, coordinationLedgerID = *record.CoordinationOrganizationID, *record.CoordinationLedgerID
	}

	candidate, err := repository.GetAtomicTransactionBatchFinalizationCandidate(
		ctx,
		coordinationOrganizationID,
		coordinationLedgerID,
		record.ExecutionID,
		record.TransactionID,
	)
	if err != nil || candidate == nil {
		return nil, err
	}

	uc.recordAtomicTransactionBatchRecovering(ctx, atomicTransactionBatchScope(
		completion.Record.Transaction != nil && completion.Record.Transaction.GroupID != nil,
	))

	if err := validateAtomicTransactionBatchRecoveredMember(candidate.Record, record, completion); err != nil {
		return nil, err
	}

	if candidate.Record.State != txRedis.AtomicTransactionBatchStateComplete {
		initialResponse, err := atomicTransactionBatchRecoveredInitialResponse(record, completion, candidate.Record.LifecycleAction)
		if err != nil {
			return nil, err
		}

		captured, err := uc.AtomicTransactionBatchIdempotencyRepo.CaptureAtomicTransactionBatchInitialResponse(
			ctx,
			coordinationOrganizationID,
			coordinationLedgerID,
			record.ExecutionID,
			candidate.Record.OwnerToken,
			record.TransactionID,
			initialResponse,
		)
		if err != nil {
			return nil, fmt.Errorf("capture recovered atomic transaction batch initial response: %w", err)
		}
		if captured == nil {
			return nil, errors.New("capture recovered atomic transaction batch initial response returned no result")
		}

		candidate.Record = captured.Record
	}

	uc.reconcileAtomicTransactionBatchRecoveredMember(ctx, completion.Record.Transaction, completion.Outcome.TransactionStatus)

	prepared := &AtomicTransactionBatchRecoveryFinalization{}
	if candidate.Record.State == txRedis.AtomicTransactionBatchStateComplete || !candidate.Candidate {
		return prepared, nil
	}

	if candidate.ReceiptToken == "" {
		return nil, errors.New("atomic transaction batch finalization candidate has no receipt token")
	}

	if candidate.Record.FormatVersion == txRedis.AtomicTransactionBatchIdempotencyFormatVersion {
		// The final ACK seals the response from the immutable captures. No
		// primary read may substitute a lifecycle-mutated projection.
		prepared.ReceiptToken = candidate.ReceiptToken

		return prepared, nil
	}

	if uc.AtomicTransactionBatchProjectionReader == nil {
		return nil, errors.New("atomic transaction batch projection reader is not configured")
	}

	transactions, err := uc.AtomicTransactionBatchProjectionReader.GetAtomicTransactionBatchProjections(
		ctx,
		record.OrganizationID,
		record.LedgerID,
		append([]uuid.UUID(nil), candidate.Record.TransactionIDs...),
	)
	if err != nil {
		return nil, fmt.Errorf("read durable atomic transaction batch projections: %w", err)
	}

	responses, err := atomicTransactionBatchRecoveryResponses(
		candidate.Record,
		record.OrganizationID,
		record.LedgerID,
		transactions,
	)
	if err != nil {
		return nil, err
	}

	prepared.ReceiptToken = candidate.ReceiptToken
	prepared.Transactions = responses

	return prepared, nil
}

func validateAtomicTransactionBatchRecoveredMember(
	batch txRedis.AtomicTransactionBatchIdempotencyRecord,
	record *TransactionCompletionRecord,
	completion TransactionCompletionResult,
) error {
	if completion.Record.Transaction == nil || completion.Record.Transaction.ID != record.TransactionID.String() {
		return errors.New("atomic transaction batch recovery completion identity differs")
	}

	if completion.Record.Transaction.OrganizationID != record.OrganizationID.String() ||
		completion.Record.Transaction.LedgerID != record.LedgerID.String() {
		return errors.New("atomic transaction batch recovery completion scope differs")
	}

	found := false

	for _, transactionID := range batch.TransactionIDs {
		if transactionID == record.TransactionID {
			found = true
			break
		}
	}

	if !found {
		return errors.New("atomic transaction batch recovery transaction is not indexed")
	}

	// CANCELED is the terminal outcome of a grouped cancel, the only batch
	// execution that releases members instead of creating them.
	if completion.Outcome.TransactionStatus != constant.APPROVED &&
		completion.Outcome.TransactionStatus != constant.PENDING &&
		completion.Outcome.TransactionStatus != constant.CANCELED {
		return fmt.Errorf(
			"atomic transaction batch recovery requires durable APPROVED, PENDING, or CANCELED status, got %q",
			completion.Outcome.TransactionStatus,
		)
	}

	return nil
}

// atomicTransactionBatchRecoveredInitialResponse freezes the member as the
// interrupted request would have answered it. A batch create answers an
// APPROVED member as CREATED; a grouped commit answers it as APPROVED.
func atomicTransactionBatchRecoveredInitialResponse(
	record *TransactionCompletionRecord,
	completion TransactionCompletionResult,
	lifecycle txRedis.AtomicTransactionBatchLifecycleAction,
) (json.RawMessage, error) {
	if record == nil || completion.Record.Transaction == nil || completion.Record.Transaction.ID != record.TransactionID.String() {
		return nil, errors.New("atomic transaction batch recovered initial response identity differs")
	}

	status := completion.Outcome.TransactionStatus
	if (lifecycle == txRedis.AtomicTransactionBatchLifecycleCommit && status != constant.APPROVED) ||
		(lifecycle == txRedis.AtomicTransactionBatchLifecycleCancel && status != constant.CANCELED) {
		return nil, fmt.Errorf("atomic transaction batch recovery status %q differs from its lifecycle %q", status, lifecycle)
	}

	public := *completion.Record.Transaction
	switch status {
	case constant.APPROVED:
		if lifecycle != txRedis.AtomicTransactionBatchLifecycleCommit {
			created := constant.CREATED
			public.Status = transaction.Status{Code: created, Description: &created}
		} else if public.Status.Code != constant.APPROVED {
			return nil, fmt.Errorf("atomic transaction batch recovery committed status differs: got %q", public.Status.Code)
		}
	case constant.PENDING:
		if public.Status.Code != constant.PENDING {
			return nil, fmt.Errorf("atomic transaction batch recovery hold status differs: got %q", public.Status.Code)
		}
	case constant.CANCELED:
		if public.Status.Code != constant.CANCELED {
			return nil, fmt.Errorf("atomic transaction batch recovery canceled status differs: got %q", public.Status.Code)
		}
	default:
		return nil, fmt.Errorf("atomic transaction batch recovery has unsupported initial status %q", status)
	}

	payload, err := json.Marshal(&public)
	if err != nil {
		return nil, fmt.Errorf("marshal recovered atomic transaction batch initial response: %w", err)
	}

	return payload, nil
}

func (uc *UseCase) reconcileAtomicTransactionBatchRecoveredMember(
	ctx context.Context,
	tran *transaction.Transaction,
	status string,
) {
	if tran == nil || tran.TracerSkipped || uc.TracerReserver == nil {
		return
	}

	transactionID, err := uuid.Parse(tran.ID)
	if err != nil || transactionID == uuid.Nil {
		return
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reconcile_atomic_transaction_batch_tracer")
	defer span.End()

	amount := decimal.Zero
	if tran.Amount != nil {
		amount = *tran.Amount
	}

	identity := reservationHandle{TransactionID: transactionID, Amount: amount, Asset: tran.AssetCode}

	// A canceled member moved no funds, so its capacity is returned rather than counted.
	action, settle := reservationActionConfirm, uc.TracerReserver.ConfirmByTransaction
	if status == constant.CANCELED {
		action, settle = reservationActionRelease, uc.TracerReserver.ReleaseByTransaction
	}

	if err := settle(ctx, transactionID); err != nil {
		uc.recordReservationByTransactionFailure(
			ctx,
			span,
			logger,
			identity.transitionByTransaction(action),
			err,
		)
	}
}

func atomicTransactionBatchRecoveryResponses(
	record txRedis.AtomicTransactionBatchIdempotencyRecord,
	organizationID, ledgerID uuid.UUID,
	transactions []*transaction.Transaction,
) (map[uuid.UUID]json.RawMessage, error) {
	if len(transactions) != len(record.TransactionIDs) {
		return nil, fmt.Errorf(
			"atomic transaction batch durable projection count differs: want %d, got %d",
			len(record.TransactionIDs),
			len(transactions),
		)
	}

	byID := make(map[uuid.UUID]*transaction.Transaction, len(transactions))
	for _, tran := range transactions {
		if tran == nil {
			return nil, errors.New("atomic transaction batch durable projection is nil")
		}

		transactionID, err := uuid.Parse(tran.ID)
		if err != nil || transactionID == uuid.Nil {
			return nil, errors.New("atomic transaction batch durable projection ID is invalid")
		}

		if _, duplicate := byID[transactionID]; duplicate {
			return nil, errors.New("atomic transaction batch durable projection ID is duplicated")
		}

		byID[transactionID] = tran
	}

	responses := make(map[uuid.UUID]json.RawMessage, len(record.TransactionIDs))
	for _, transactionID := range record.TransactionIDs {
		tran, found := byID[transactionID]
		if !found || tran.OrganizationID != organizationID.String() || tran.LedgerID != ledgerID.String() ||
			tran.Status.Code != constant.APPROVED || len(tran.Operations) == 0 {
			return nil, fmt.Errorf("atomic transaction batch transaction %s is not durably complete", transactionID)
		}

		public := *tran
		created := constant.CREATED
		public.Status = transaction.Status{Code: created, Description: &created}

		payload, err := json.Marshal(&public)
		if err != nil {
			return nil, fmt.Errorf("marshal recovered atomic transaction batch transaction %s: %w", transactionID, err)
		}

		responses[transactionID] = payload
	}

	return responses, nil
}
