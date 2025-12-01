// Package command provides CQRS command handlers for the transaction component.
package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateBalances applies validated transaction amounts to multiple balances atomically.
//
// This function is the core balance update mechanism for transaction processing.
// It takes pre-validated transaction amounts (from/to) and applies them to the
// corresponding balances, incrementing version numbers for optimistic locking.
//
// # Double-Entry Accounting
//
// The function processes both sides of a transaction:
//   - From accounts: Amounts to debit (reduce balance)
//   - To accounts: Amounts to credit (increase balance)
//
// The validate parameter contains pre-computed amounts that have already been
// verified to balance (sum of debits == sum of credits).
//
// # Optimistic Locking
//
// Each balance has a version field that is incremented with every update.
// The repository layer uses this for optimistic locking:
//   - Read balance with current version
//   - Calculate new values
//   - Write with version + 1, WHERE version = current_version
//   - If no rows updated, another process modified the balance (retry needed)
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.update_balances_new"
//  3. Merge From and To amounts into single lookup map by alias
//  4. For each balance in the input slice:
//     a. Start child span for individual balance processing
//     b. Look up the amount for this balance by alias
//     c. Calculate new Available and OnHold using OperateBalances
//     d. Create updated balance record with incremented version
//  5. Batch update all balances in a single database call
//  6. Return success or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing these balances
//   - validate: Pre-validated transaction amounts (From/To maps by alias)
//   - balances: The balance records to update
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Balance calculation fails (OperateBalances)
//   - Database batch update fails
//   - Context cancellation/timeout
//
// # Error Scenarios
//
//   - OperateBalances calculation error (invalid amounts)
//   - Database connection failure
//   - Optimistic lock failure (concurrent modification)
//   - Context cancellation/timeout
//
// # Observability
//
// Creates parent span "command.update_balances_new" and child spans for
// each individual balance calculation. Error events recorded on failures.
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

// Update modifies a single balance record in the repository.
//
// This function provides a simple update mechanism for individual balance
// modifications that don't require the full transaction validation flow.
// Use this for administrative updates or corrections, not for transaction processing.
//
// # Use Cases
//
//   - Administrative balance corrections
//   - Status updates (active/inactive)
//   - Balance holds/releases outside transaction flow
//
// For transaction-based balance updates, use UpdateBalances instead.
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "exec.update_balance"
//  3. Call repository Update with the provided changes
//  4. Return success or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing this balance
//   - balanceID: The unique identifier of the balance to update
//   - update: The fields to update (partial update supported)
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Balance not found
//   - Database operation fails
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "exec.update_balance" with error events on failure.
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
