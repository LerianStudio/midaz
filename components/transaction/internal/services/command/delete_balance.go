package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteBalance delete balance in the repository.
func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "balance_delete_attempt",
		attribute.String("balance_id", balanceID.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to delete balance")

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get balance on repo by id", err)

		logger.Errorf("Error getting balance: %v", err)

		// Record error
		uc.recordTransactionError(ctx, "balance_find_error",
			attribute.String("balance_id", balanceID.String()),
			attribute.String("error_detail", err.Error()))

		// Record transaction duration with error status
		uc.recordTransactionDuration(ctx, startTime, "balance_delete", "error",
			attribute.String("balance_id", balanceID.String()),
			attribute.String("error", "find_error"))

		return err
	}

	if balance != nil && (balance.Available != 0 || balance.OnHold != 0) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantDeleted, "DeleteBalance")

		mopentelemetry.HandleSpanError(&span, "Balance cannot be deleted because it still has funds in it.", err)

		logger.Errorf("Error deleting balance: %v", err)

		// Record error
		uc.recordTransactionError(ctx, "balance_validation_error",
			attribute.String("balance_id", balanceID.String()),
			attribute.String("error_detail", "balance_has_funds"),
			attribute.Int64("available", int64(balance.Available)),
			attribute.Int64("on_hold", int64(balance.OnHold)))

		// Record transaction duration with error status
		uc.recordTransactionDuration(ctx, startTime, "balance_delete", "error",
			attribute.String("balance_id", balanceID.String()),
			attribute.String("error", "balance_has_funds"))

		return err
	}

	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete balance on repo", err)

		logger.Errorf("Error delete balance: %v", err)

		// Record error
		uc.recordTransactionError(ctx, "balance_delete_error",
			attribute.String("balance_id", balanceID.String()),
			attribute.String("error_detail", err.Error()))

		// Record transaction duration with error status
		uc.recordTransactionDuration(ctx, startTime, "balance_delete", "error",
			attribute.String("balance_id", balanceID.String()),
			attribute.String("error", "delete_error"))

		return err
	}

	// Record transaction duration with success status
	uc.recordTransactionDuration(ctx, startTime, "balance_delete", "success",
		attribute.String("balance_id", balanceID.String()),
		attribute.String("asset_code", balance.AssetCode),
		attribute.String("account_id", balance.AccountID))

	// Record business metric for balance delete success
	uc.recordBusinessMetrics(ctx, "balance_delete_success",
		attribute.String("balance_id", balanceID.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", balance.AssetCode),
		attribute.String("account_id", balance.AccountID))

	return nil
}
