// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	// UpdateTransaction update a transaction from the repository by given id.
	libLog "github.com/LerianStudio/lib-observability/v2/log"
)

// TransactionUpdateStatusGate is evaluated while the PostgreSQL transaction
// row is locked. It may acquire a rollout lease for an APPROVED transaction and
// returns its owner-safe release function.
type TransactionUpdateStatusGate func(context.Context, string) (func() error, error)

type transactionUpdateCommitState uint8

const (
	transactionUpdateCommitUnknown transactionUpdateCommitState = iota
	transactionUpdateCommitApplied
	transactionUpdateCommitRolledBack
)

const (
	transactionUpdateCommitReadAttempts = 3
	transactionUpdateCommitReadDelay    = 10 * time.Millisecond
)

func (uc *UseCase) UpdateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, uti *transaction.UpdateTransactionInput) (_ *transaction.Transaction, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_transaction", start, err)
	}()

	trans := &transaction.Transaction{
		Description: uti.Description,
	}

	transUpdated, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error updating transaction on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, constant.EntityTransaction)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, "Error updating transaction on repo by id", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateTransactionMetadata(ctx, constant.EntityTransaction, transactionID.String(), uti.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	transUpdated.Metadata = metadataUpdated

	return transUpdated, nil
}

// UpdateTransactionSerialized holds the transaction row lock across the status
// decision, MongoDB metadata write, and PostgreSQL description write. Every
// PENDING-to-terminal PostgreSQL update therefore waits until the PATCH has
// fully decided, including backup-consumer recovery after an HTTP crash.
func (uc *UseCase) UpdateTransactionSerialized(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	uti *transaction.UpdateTransactionInput,
	gate TransactionUpdateStatusGate,
) (result *transaction.Transaction, retErr error) {
	dbTx, err := uc.TransactionRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin serialized transaction update: %w", err)
	}

	committed := false
	releaseGateOnReturn := true

	defer func() {
		if !committed {
			_ = dbTx.Rollback()
		}
	}()

	current, err := uc.TransactionRepo.FindForUpdate(ctx, dbTx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	var releaseGate func() error
	if gate != nil {
		releaseGate, err = gate(ctx, current.Status.Code)
		if err != nil {
			return nil, err
		}
	}

	if releaseGate != nil {
		defer func() {
			if !releaseGateOnReturn {
				return
			}

			if err := releaseGate(); err != nil {
				retErr = errors.Join(retErr, err)
				result = nil
			}
		}()
	}

	updateVersion := nextTransactionUpdateVersion(current.UpdatedAt)

	result, err = uc.TransactionRepo.UpdateTx(ctx, dbTx, organizationID, ledgerID, transactionID, &transaction.Transaction{
		Description: uti.Description,
		UpdatedAt:   updateVersion,
	})
	if err != nil {
		return nil, err
	}

	metadataBefore := map[string]any{}

	existingMetadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, constant.EntityTransaction, transactionID.String())
	if err != nil {
		return nil, err
	}

	if existingMetadata != nil {
		metadataBefore = maps.Clone(existingMetadata.Data)
	}

	metadataUpdated, err := uc.updateTransactionMetadataFromSnapshot(ctx, constant.EntityTransaction,
		transactionID.String(), uti.Metadata, existingMetadata)
	if err != nil {
		releaseGateOnReturn = false
		reconciliationErr := pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)

		return nil, fmt.Errorf("transaction metadata update is ambiguous: %w: %w", err, reconciliationErr)
	}

	if commitErr := dbTx.Commit(); commitErr != nil {
		state, persisted, reconcileErr := uc.reconcileTransactionUpdateCommit(ctx, organizationID, ledgerID,
			transactionID, current, updateVersion, uti.Description)
		switch state {
		case transactionUpdateCommitApplied:
			committed = true
			persisted.Metadata = metadataUpdated

			return persisted, nil
		case transactionUpdateCommitRolledBack:
			if !reflect.DeepEqual(metadataBefore, metadataUpdated) {
				releaseGateOnReturn = false
				reconciliationErr := pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)

				return nil, fmt.Errorf("serialized transaction update rolled back after metadata changed: %w: %w",
					commitErr, reconciliationErr)
			}

			return nil, fmt.Errorf("commit serialized transaction update: %w", commitErr)
		default:
			releaseGateOnReturn = false

			reconciliationErr := pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
			if reconcileErr != nil {
				return nil, fmt.Errorf("reconcile ambiguous serialized transaction update: %w: %w", reconcileErr, reconciliationErr)
			}

			return nil, fmt.Errorf("reconcile ambiguous serialized transaction update after commit error: %w: %w",
				commitErr, reconciliationErr)
		}
	}

	committed = true
	result.Metadata = metadataUpdated

	return result, nil
}

func nextTransactionUpdateVersion(previous time.Time) time.Time {
	next := time.Now().UTC().Truncate(time.Microsecond)

	previous = previous.UTC().Truncate(time.Microsecond)
	if !next.After(previous) {
		return previous.Add(time.Microsecond)
	}

	return next
}

func (uc *UseCase) reconcileTransactionUpdateCommit(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	before *transaction.Transaction,
	updateVersion time.Time,
	description string,
) (transactionUpdateCommitState, *transaction.Transaction, error) {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()

	var lastErr error

	for attempt := 0; attempt < transactionUpdateCommitReadAttempts; attempt++ {
		persisted, err := uc.TransactionRepo.Find(readrouting.WithPrimaryRead(reconcileCtx), organizationID, ledgerID, transactionID)
		if err == nil {
			expectedDescription := description
			if expectedDescription == "" {
				expectedDescription = before.Description
			}

			if exactTransactionUpdateVersion(persisted, before, updateVersion, expectedDescription) {
				return transactionUpdateCommitApplied, persisted, nil
			}

			if exactTransactionUpdateVersion(persisted, before, before.UpdatedAt, before.Description) {
				return transactionUpdateCommitRolledBack, persisted, nil
			}

			return transactionUpdateCommitUnknown, persisted, nil
		}

		lastErr = err

		if attempt+1 == transactionUpdateCommitReadAttempts {
			break
		}

		timer := time.NewTimer(transactionUpdateCommitReadDelay)
		select {
		case <-reconcileCtx.Done():
			timer.Stop()

			return transactionUpdateCommitUnknown, nil, errors.Join(lastErr, reconcileCtx.Err())
		case <-timer.C:
		}
	}

	return transactionUpdateCommitUnknown, nil, lastErr
}

func exactTransactionUpdateVersion(
	persisted, before *transaction.Transaction,
	version time.Time,
	description string,
) bool {
	if persisted == nil || before == nil {
		return false
	}

	return persisted.ID == before.ID && persisted.OrganizationID == before.OrganizationID &&
		persisted.LedgerID == before.LedgerID && persisted.Description == description &&
		persisted.Status.Code == before.Status.Code && equalOptionalString(persisted.Status.Description, before.Status.Description) &&
		persisted.UpdatedAt.Equal(version)
}

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

// UpdateTransactionStatus update a status transaction from the repository by given id.
func (uc *UseCase) UpdateTransactionStatus(ctx context.Context, tran *transaction.Transaction) (_ *transaction.Transaction, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_status")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_transaction_status", start, err)
	}()

	if tran == nil {
		err := errors.New("transaction cannot be nil")
		libOpentelemetry.HandleSpanError(span, "Nil transaction provided", err)

		return nil, err
	}

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)
	transactionID := uuid.MustParse(tran.ID)

	// Status transitions can wait behind a concurrent serialized PATCH. Write only
	// the transition so a stale transaction snapshot cannot overwrite the PATCHed
	// description or accounting body after the row lock is released.
	updateTran, err := uc.TransactionRepo.UpdateStatusFromPending(ctx, organizationID, ledgerID, transactionID, &transaction.Transaction{
		Status: tran.Status,
	})
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error updating status transaction on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, constant.EntityTransaction)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update status transaction on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, "Error updating status transaction on repo by id", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update status transaction on repo by id", err)

		return nil, err
	}

	return updateTran, nil
}

// UpdateTransactionStatusTx applies a PENDING-to-terminal transition through
// the caller's row-locking PostgreSQL transaction. It is committed by the
// money-path caller immediately after the atomic Redis balance movement.
func (uc *UseCase) UpdateTransactionStatusTx(
	ctx context.Context,
	dbTx repository.DBTransaction,
	tran *transaction.Transaction,
) (*transaction.Transaction, error) {
	if tran == nil {
		return nil, errors.New("transaction cannot be nil")
	}

	organizationID, ledgerID, transactionID, err := terminalTransactionIdentity(tran)
	if err != nil {
		return nil, err
	}

	return uc.TransactionRepo.UpdateStatusFromPendingTx(ctx, dbTx, organizationID, ledgerID, transactionID, &transaction.Transaction{
		Status: tran.Status,
	})
}
