package command

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/google/uuid"
	"reflect"
)

// UpdateTransaction update a transaction from the repository by given id.
func (uc *UseCase) UpdateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, uti *transaction.UpdateTransactionInput) (*transaction.Transaction, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction")
	defer span.End()

	logger.Infof("Trying to update transaction: %v", uti)

	trans := &transaction.Transaction{
		Description: uti.Description,
	}

	transUpdated, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update transaction on repo by id", err)

		logger.Errorf("Error updating transaction on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		return nil, err
	}

	if len(uti.Metadata) > 0 {
		if err := libCommons.CheckMetadataKeyAndValueLength(100, uti.Metadata); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

		err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String(), uti.Metadata)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to update metadata on mongodb transaction", err)

			return nil, err
		}

		transUpdated.Metadata = uti.Metadata
	}

	return transUpdated, nil
}

// UpdateTransactionStatus update a status transaction from the repository by given id.
func (uc *UseCase) UpdateTransactionStatus(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, description string) (*transaction.Transaction, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_status")
	defer span.End()

	logger.Infof("Trying to update transaction using status: : %v", description)

	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	trans := &transaction.Transaction{
		Status: status,
	}

	_, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update status transaction on repo by id", err)

		logger.Errorf("Error updating status transaction on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		return nil, err
	}

	return nil, nil
}
