package command

import (
	"context"

	"github.com/LerianStudio/midaz/common/mlog"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/google/uuid"
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

	return tran, nil
}
