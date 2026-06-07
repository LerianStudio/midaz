// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"

	// UpdateTransaction update a transaction from the repository by given id.
	libLog "github.com/LerianStudio/lib-observability/log"
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

	updateTran, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, tran)
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
