// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateBalances updates multiple account balances after transaction validation.
//
// This method implements batch balance updates for transaction processing, which:
// 1. Merges validation responses (from and to amounts)
// 2. Calculates new balance values for each account
// 3. Increments version numbers for optimistic locking
// 4. Performs batch update in PostgreSQL
//
// Balance Calculation:
//   - Uses lib-commons OperateBalances to apply amount changes
//   - Handles available and on-hold amounts
//   - Ensures double-entry accounting principles
//
// Optimistic Locking:
//   - Version number is incremented on each update
//   - Prevents concurrent modification conflicts
//   - Database enforces version check in UPDATE WHERE clause
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - validate: Validation responses with calculated amounts
//   - balances: Current balances to update
//
// Returns:
//   - error: nil on success, error if update fails
//
// OpenTelemetry: Creates span "command.update_balances_new"
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

// Update updates a single balance in the repository.
//
// This method updates balance flags (allow_sending, allow_receiving) for a specific balance.
// It's used for administrative operations to enable/disable balance operations.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - balanceID: UUID of the balance to update
//   - update: Update input with allow_sending and allow_receiving flags
//
// Returns:
//   - error: nil on success, error if update fails
//
// OpenTelemetry: Creates span "exec.update_balance"
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
