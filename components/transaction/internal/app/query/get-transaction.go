package query

import (
	"context"
	"github.com/LerianStudio/midaz/common/mlog"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/google/uuid"
	"reflect"
)

// GetTransactionByID gets data in the repository.
func (uc *UseCase) GetTransactionByID(ctx context.Context, organizationID, ledgerID, transactionID string) (*t.Transaction, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(transactionID))
	if err != nil {
		logger.Errorf("Error getting transaction: %v", err)
		return nil, err
	}

	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(t.Transaction{}).Name(), transactionID)
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
