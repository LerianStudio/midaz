package command

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
)

// DeleteBalance delete balance in the repository.
func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	logger.Infof("Trying to delete balance")

	err := uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete balance on repo", err)

		logger.Errorf("Error delete balance: %v", err)

		return err
	}

	return nil
}
