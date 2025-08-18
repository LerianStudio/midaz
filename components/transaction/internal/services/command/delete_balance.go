package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// DeleteBalance delete balance in the repository.
func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	logger.Infof("Trying to delete balance")

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance on repo by id", err)

		logger.Errorf("Error getting balance: %v", err)

		return err
	}

	if balance != nil && (!balance.Available.IsZero() || !balance.OnHold.IsZero()) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantDeleted, "DeleteBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because it still has funds in it.", err)
		logger.Warnf("Error deleting balance: %v", err)

		return err
	}

	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)
		logger.Errorf("Error delete balance: %v", err)

		return err
	}

	return nil
}
