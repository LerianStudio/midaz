package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateBalances atomically updates multiple account balances using optimistic locking.
//
// This is a **CRITICAL FINANCIAL FUNCTION** that ensures atomic, consistent balance updates
// across all accounts involved in a transaction. It implements optimistic concurrency control
// to prevent race conditions when multiple transactions affect the same accounts.
//
// Balance Update Mechanics:
// 1. Merge validated amounts from source (from) and destination (to) operations
// 2. Calculate new balance values using double-entry accounting rules
// 3. Increment version number for optimistic locking
// 4. Atomically update all balances (uses Redis Lua scripts in repository layer)
//
// Double-Entry Accounting Rules Applied by OperateBalances:
// - DEBIT: Decreases available balance (e.g., withdrawal, payment)
// - CREDIT: Increases available balance (e.g., deposit, receipt)
// - ON_HOLD: Moves from available to onHold (reservation)
// - RELEASE: Moves from onHold back to available (release reservation)
//
// Optimistic Locking:
// - Each balance has a version number that increments on every update
// - The repository layer uses "WHERE version = N" in UPDATE statements
// - If version doesn't match, update fails (ErrLockVersionAccountBalance)
// - This prevents lost updates when concurrent transactions affect same accounts
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID for scoping updates
//   - ledgerID: Ledger UUID for scoping updates
//   - validate: Validated transaction amounts per account alias
//   - balances: Current balance states fetched from Redis/PostgreSQL
//
// Returns:
//   - error: Balance calculation errors, insufficient funds, or optimistic lock failures
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate libTransaction.Responses, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	// Step 1: Merge source (from) and destination (to) amounts into a unified map.
	// This combines debits, credits, holds, and releases for all accounts in the transaction.
	fromTo := make(map[string]libTransaction.Amount, len(validate.From)+len(validate.To))
	for k, v := range validate.From {
		fromTo[k] = v
	}

	for k, v := range validate.To {
		fromTo[k] = v
	}

	newBalances := make([]*mmodel.Balance, 0, len(balances))

	// Step 2: Calculate new balance values for each affected account.
	// OperateBalances applies double-entry accounting rules based on operation type.
	for _, balance := range balances {
		_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

		calculateBalances, err := libTransaction.OperateBalances(fromTo[balance.Alias], *balance.ConvertToLibBalance())
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

			return err
		}

		// Step 3: Increment version for optimistic locking
		newBalances = append(newBalances, &mmodel.Balance{
			ID:        balance.ID,
			Alias:     balance.Alias,
			Available: calculateBalances.Available,
			OnHold:    calculateBalances.OnHold,
			Version:   balance.Version + 1,
		})

		spanBalance.End()
	}

	// Step 4: Atomically update all balances in PostgreSQL with version checking.
	// This uses optimistic locking to prevent concurrent modification issues.
	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, newBalances); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances on database", err)
		logger.Errorf("Failed to update balances on database: %v", err.Error())

		return err
	}

	return nil
}

// Update modifies balance permissions (allowSending/allowReceiving) for a specific balance.
//
// This function updates the operational flags that control whether an account balance
// can participate in transactions as a source (sending) or destination (receiving).
// It does NOT modify balance amounts - only permissions.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning the balance
//   - ledgerID: Ledger UUID containing the balance
//   - balanceID: UUID of the balance to update
//   - update: New permission flags (allowSending, allowReceiving)
//
// Returns:
//   - error: Repository errors or balance not found
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
