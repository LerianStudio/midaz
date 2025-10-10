// Package command implements write operations (commands) for the transaction service.
// This file contains commands for updating account balances.
package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateBalances performs a batch update of account balances after a transaction.
//
// This use case applies the calculated amount changes from a transaction to multiple
// balances in a single, atomic operation. It also increments the version number for
// each balance to support optimistic locking and prevent race conditions.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - validate: The validation response containing the calculated amount changes.
//   - balances: The current balances to be updated.
//
// Returns:
//   - error: An error if the batch update fails.
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate libTransaction.Responses, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	fromTo := make(map[string]libTransaction.Amount, len(validate.From)+len(validate.To))
	for k, v := range validate.From {
		fromTo[k] = v
	}

	for k, v := range validate.To {
		fromTo[k] = v
	}

	newBalances := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

		calculateBalances, err := libTransaction.OperateBalances(fromTo[balance.Alias], *balance.ConvertToLibBalance())
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

			return err
		}

		newBalances = append(newBalances, &mmodel.Balance{
			ID:        balance.ID,
			Alias:     balance.Alias,
			Available: calculateBalances.Available,
			OnHold:    calculateBalances.OnHold,
			Version:   balance.Version + 1,
		})

		spanBalance.End()
	}

	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, newBalances); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances on database", err)
		logger.Errorf("Failed to update balances on database: %v", err.Error())

		return err
	}

	return nil
}

// Update updates the flags of a single balance.
//
// This use case is used for administrative purposes, such as enabling or disabling
// sending and receiving capabilities for a specific balance.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - balanceID: The UUID of the balance to update.
//   - update: The input data containing the new flag values.
//
// Returns:
//   - error: An error if the update fails.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Infof("Trying to update balance")

	err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance on repo", err)
		logger.Errorf("Error update balance: %v", err)

		return err
	}

	return nil
}
