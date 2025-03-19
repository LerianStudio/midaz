package command

import (
	"context"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteBalance delete balance in the repository.
func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a balance operation telemetry entity
	op := uc.Telemetry.NewBalanceOperation("delete", balanceID.String())

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Trying to delete balance")

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		// Record error
		op.RecordError(ctx, "balance_find_error", err)
		op.End(ctx, "failed")

		logger.Errorf("Error getting balance: %v", err)

		return err
	}

	if balance != nil && (balance.Available != 0 || balance.OnHold != 0) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantDeleted, "DeleteBalance")

		// Record validation error with funds detail
		op.RecordError(ctx, "balance_validation_error", err)
		op.WithAttributes(
			attribute.String("error_detail", "balance_has_funds"),
			attribute.Int64("available", balance.Available),
			attribute.Int64("on_hold", balance.OnHold),
		)
		op.End(ctx, "failed")

		logger.Errorf("Error deleting balance: %v", err)

		return err
	}

	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		// Record error
		op.RecordError(ctx, "balance_delete_error", err)
		op.End(ctx, "failed")

		logger.Errorf("Error delete balance: %v", err)

		return err
	}

	// Add balance details to telemetry
	if balance != nil {
		op.WithAttributes(
			attribute.String("asset_code", balance.AssetCode),
			attribute.String("account_id", balance.AccountID),
		)
	}

	// Mark operation as successful
	op.End(ctx, "success")

	return nil
}
