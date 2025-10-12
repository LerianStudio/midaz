package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// DeleteBalance deletes a balance with fund verification.
//
// Critical Financial Rule: Balances can only be deleted if they have zero funds
// (both Available and OnHold must be zero). This prevents accidental loss of funds.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning the balance
//   - ledgerID: Ledger UUID containing the balance
//   - balanceID: UUID of the balance to delete
//
// Returns:
//   - error: ErrBalancesCantDeleted if funds remain, or repository errors
func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	logger.Infof("Trying to delete balance")

	// Step 1: Verify balance exists and check fund amounts
	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance on repo by id", err)

		logger.Errorf("Error getting balance: %v", err)

		return err
	}

	// Step 2: Prevent deletion if any funds remain (critical financial safeguard)
	if balance != nil && (!balance.Available.IsZero() || !balance.OnHold.IsZero()) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantDeleted, "DeleteBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because it still has funds in it.", err)
		logger.Warnf("Error deleting balance: %v", err)

		return err
	}

	// Step 3: Perform soft delete
	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)
		logger.Errorf("Error delete balance: %v", err)

		return err
	}

	return nil
}
