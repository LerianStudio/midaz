package command

import (
	"context"
	"errors"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// DeleteAllBalancesByAccountID safely deletes all balances associated with an account.
//
// This function implements a safe deletion process with multiple validation checks
// to prevent data loss or inconsistency. Balances can only be deleted when they
// have zero funds and no active transactions.
//
// Safety Validation Process:
//
//	Step 1: Fetch All Balances
//	  - Retrieve all balances for the account
//	  - Return early if no balances exist
//
//	Step 2: Check Active Transactions
//	  - For each balance, check Redis cache for pending transactions
//	  - If any balance has cached data, abort (transactions in progress)
//
//	Step 3: Verify Zero Funds
//	  - Check that Available and OnHold amounts are zero
//	  - Balances with funds cannot be deleted (would cause fund loss)
//
//	Step 4: Disable Transfer Permissions
//	  - Set AllowSending and AllowReceiving to false
//	  - Prevents new transactions during deletion window
//
//	Step 5: Delete Balances
//	  - Bulk delete all balance records
//	  - On failure: Re-enable transfer permissions (rollback)
//
// Why These Checks Matter:
//
//	Active Transactions: Deleting a balance mid-transaction would cause
//	the transaction to fail or corrupt data. The Redis cache check
//	detects balances with pending operations.
//
//	Non-Zero Funds: Deleting a balance with funds would effectively
//	destroy money, breaking ledger integrity. This check ensures
//	funds are transferred out before deletion.
//
//	Transfer Permissions: Disabling transfers creates a deletion window
//	where no new transactions can start on these balances, preventing
//	race conditions.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - accountID: Account whose balances should be deleted
//
// Returns:
//   - error: Business validation or infrastructure error
//
// Error Scenarios:
//   - ErrBalancesCantBeDeleted: Active transactions or non-zero funds
//   - Redis errors: Cache check failed
//   - Database errors: PostgreSQL unavailable
func (uc *UseCase) DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID uuid.UUID, accountID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_all_balances_by_account_id")
	defer span.End()

	logger.Infof("Trying to delete all balances by account id: %s", accountID.String())

	balances, err := uc.BalanceRepo.ListByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances by account id on repo", err)

		logger.Errorf("Error getting balances by account id on repo: %v", err)

		return err
	}

	if len(balances) == 0 {
		return nil
	}

	for _, balance := range balances {
		cacheBalance, err := uc.RedisRepo.ListBalanceByKey(ctx, organizationID, ledgerID, fmt.Sprintf("%s#%s", balance.Alias, balance.Key))
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			} else {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance by key on redis", err)

				logger.Errorf("Error getting balance by key on redis: %v", err)

				return err
			}
		}

		if cacheBalance != nil {
			err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "ListBalanceByAccountIDAndKey")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because there is transactions happening.", err)

			logger.Warnf("Balance cannot be deleted because there is transactions happening: %v", err)

			return err
		}

		if !balance.Available.IsZero() || !balance.OnHold.IsZero() {
			err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteAllBalancesByAccountID")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because it still has funds in it.", err)

			logger.Warnf("Error deleting balances: %v", err)

			return err
		}
	}

	if err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, false); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to toggle balance transfers for account on repo", err)

		logger.Errorf("Error toggling balance transfers for account on repo: %v", err)

		return err
	}

	balanceIDs := make([]uuid.UUID, 0, len(balances))
	for _, balance := range balances {
		balanceIDs = append(balanceIDs, balance.IDtoUUID())
	}

	err = uc.BalanceRepo.DeleteAllByIDs(ctx, organizationID, ledgerID, balanceIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)

		logger.Errorf("Error delete balance: %v", err)

		toggleErr := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, true)
		if toggleErr != nil {
			logger.Errorf("Error toggling balance transfers for account %s: %v", accountID.String(), toggleErr)
		}

		return err
	}

	return nil
}

// toggleBalanceTransfers enables or disables transfer permissions for an account's balances.
//
// This function is used during balance deletion to create a safe deletion window.
// When disabled, no new transactions can debit or credit these balances.
//
// Rollback Behavior:
//
// The function implements deferred rollback - if the operation fails after
// permissions are changed, it attempts to restore the original state.
// This ensures balances aren't permanently locked due to partial failures.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - accountID: Account whose balance permissions to modify
//   - allow: true to enable transfers, false to disable
//
// Returns:
//   - error: Database update error
func (uc *UseCase) toggleBalanceTransfers(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, allow bool) (err error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.toggle_balance_transfers")
	defer span.End()

	logger.Infof("Trying to toggle balance transfers")

	allowTransfer := utils.BoolPtr(allow)

	defer func() {
		if err == nil {
			return
		}

		if rollbackErr := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, utils.BoolPtr(!allow)); rollbackErr != nil {
			logger.Errorf("Failed to rollback transfer permissions for account %s: %v", accountID.String(), rollbackErr)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to rollback balance transfer permission", rollbackErr)
		}
	}()

	if err = uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, allowTransfer); err != nil {
		return err
	}

	return nil
}

// updateBalanceTransferPermissions updates AllowSending and AllowReceiving for all account balances.
//
// This is the low-level function that performs the actual database update.
// It sets both sending and receiving permissions to the same value.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - accountID: Account whose balance permissions to modify
//   - allowTransfer: Pointer to bool value for both AllowSending and AllowReceiving
//
// Returns:
//   - error: Database update error
func (uc *UseCase) updateBalanceTransferPermissions(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, allowTransfer *bool) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance_transfer_permissions_for_account")
	defer span.End()

	logger.Infof("Trying to update balance transfer permissions for account %s", accountID.String())

	err := uc.BalanceRepo.UpdateAllByAccountID(ctx, organizationID, ledgerID, accountID, mmodel.UpdateBalance{
		AllowReceiving: allowTransfer,
		AllowSending:   allowTransfer,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance transfer permissions for account on repo", err)

		logger.Errorf("Error update balance transfer permissions for account: %v", err)

		return err
	}

	return nil
}
