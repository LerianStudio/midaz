// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// DeleteBalance soft-deletes a balance from the repository.
//
// This method implements the delete balance use case with safety checks:
// 1. Fetches the balance to validate it exists
// 2. Checks that both available and on-hold amounts are zero
// 3. Performs soft delete if balance is empty
// 4. Returns error if balance has funds
//
// Business Rules:
//   - Balance can only be deleted if both available and on-hold are zero
//   - This prevents accidental deletion of balances with funds
//   - Soft delete sets deleted_at timestamp
//   - Balance remains in database for audit purposes
//
// Safety Check:
//   - Validates balance.Available.IsZero() AND balance.OnHold.IsZero()
//   - Returns ErrBalancesCantDeleted if balance has any funds
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - balanceID: UUID of the balance to delete
//
// Returns:
//   - error: nil on success, business error if balance has funds or deletion fails
//
// OpenTelemetry: Creates span "exec.delete_balance"
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
