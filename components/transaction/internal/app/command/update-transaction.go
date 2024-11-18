package command

import (
	"context"
	"errors"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/app"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/google/uuid"
)

// UpdateTransaction update a transaction from the repository by given id.
func (uc *UseCase) UpdateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, uti *t.UpdateTransactionInput) (*t.Transaction, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction")
	defer span.End()

	logger.Infof("Trying to update transaction: %v", uti)

	trans := &t.Transaction{
		Description: uti.Description,
	}

	transUpdated, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update transaction on repo by id", err)

		logger.Errorf("Error updating transaction on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrTransactionIDNotFound, reflect.TypeOf(t.Transaction{}).Name())
		}

		return nil, err
	}

	if len(uti.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, uti.Metadata); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

		err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(t.Transaction{}).Name(), transactionID.String(), uti.Metadata)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to update metadata on mongodb transaction", err)

			return nil, err
		}

		transUpdated.Metadata = uti.Metadata
	}

	return transUpdated, nil
}

// UpdateTransactionStatus update a status transaction from the repository by given id.
func (uc *UseCase) UpdateTransactionStatus(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, description string) (*t.Transaction, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_status")
	defer span.End()

	logger.Infof("Trying to update transaction using status: : %v", description)

	status := t.Status{
		Code:        description,
		Description: &description,
	}

	trans := &t.Transaction{
		Status: status,
	}

	transUpdated, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update status transaction on repo by id", err)

		logger.Errorf("Error updating status transaction on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrTransactionIDNotFound, reflect.TypeOf(t.Transaction{}).Name())
		}

		return nil, err
	}

	return transUpdated, nil
}
