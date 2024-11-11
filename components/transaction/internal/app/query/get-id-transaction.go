package query

import (
	"context"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/google/uuid"
)

// GetTransactionByID gets data in the repository.
func (uc *UseCase) GetTransactionByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*t.Transaction, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_by_id")
	defer span.End()

	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.Find(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get transaction on repo by id", err)

		logger.Errorf("Error getting transaction: %v", err)

		return nil, err
	}

	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(t.Transaction{}).Name(), transactionID.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb account", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}
