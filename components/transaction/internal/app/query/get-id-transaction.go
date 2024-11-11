package query

import (
	"context"
	"github.com/LerianStudio/midaz/common"
	"reflect"

	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/google/uuid"
)

// GetTransactionByID gets data in the repository.
func (uc *UseCase) GetTransactionByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*t.Transaction, error) {
	logger := common.NewLoggerFromContext(ctx)
	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.Find(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		logger.Errorf("Error getting transaction: %v", err)
		return nil, err
	}

	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(t.Transaction{}).Name(), transactionID.String())
		if err != nil {
			logger.Errorf("Error get metadata on mongodb account: %v", err)
			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}
