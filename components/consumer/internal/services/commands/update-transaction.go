package commands

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/transaction"
	"github.com/LerianStudio/midaz/components/consumer/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/google/uuid"
	"reflect"
)

// UpdateTransactionStatus update a status transaction from the repository by given id.
func (uc *UseCase) UpdateTransactionStatus(ctx context.Context, tran *transaction.Transaction) (*transaction.Transaction, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "commands.update_transaction_status")
	defer span.End()

	logger.Infof("Trying to update transaction using status: : %v", tran.Status.Description)

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)
	transactionID := uuid.MustParse(tran.ID)

	updateTran, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update status transaction on repo by id", err)

		logger.Errorf("Error updating status transaction on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		return nil, err
	}

	return updateTran, nil
}