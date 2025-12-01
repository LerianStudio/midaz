// Package command provides CQRS command handlers for the transaction component.
package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// DeleteBalance removes a balance record from the repository.
//
// This function performs a safe delete operation that validates the balance
// has no remaining funds before allowing deletion. This prevents orphaned
// funds and maintains ledger integrity.
//
// # Safety Check
//
// Before deletion, the function verifies:
//   - Available balance is zero
//   - OnHold balance is zero
//
// If either balance is non-zero, deletion is rejected with ErrBalancesCantBeDeleted.
// This ensures no funds are lost during balance cleanup operations.
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "exec.delete_balance"
//  3. Fetch the balance by ID to check current state
//  4. Validate both Available and OnHold are zero
//  5. If funds exist, return business error (cannot delete)
//  6. Delete the balance record from PostgreSQL
//  7. Return success or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing this balance
//   - balanceID: The unique identifier of the balance to delete
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Balance not found
//   - Balance has non-zero funds (Available or OnHold)
//   - Database operation fails
//
// # Error Scenarios
//
//   - ErrBalancesCantBeDeleted: Balance has remaining funds
//   - Database connection failure during Find
//   - Database connection failure during Delete
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "exec.delete_balance" with error events on failure.
// Logs operation start at info level, warnings for business errors, errors for failures.
//
// # Financial Integrity
//
// This check is critical for maintaining ledger integrity:
//   - Prevents orphaned funds that would cause balance sheet discrepancies
//   - Ensures proper cleanup workflow (transfer/zero funds before deletion)
//   - Maintains audit trail by requiring explicit fund handling
func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteBalance")
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
