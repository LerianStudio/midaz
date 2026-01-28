package query

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// GetAccountBalancesAtTimestamp retrieves all balance states for an account at a specific point in time.
// It finds the last operation for each balance before the given timestamp and returns the balance states.
func (uc *UseCase) GetAccountBalancesAtTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "query.get_account_balances_at_timestamp")
	defer span.End()

	logger.Infof("Retrieving balances for account %s at timestamp %s", accountID, timestamp.Format(time.RFC3339))

	// Validate timestamp is not in the future
	if timestamp.After(time.Now()) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTimestamp, "timestamp cannot be in the future")
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Timestamp is in the future", err)
		logger.Warnf("Timestamp is in the future: %s", timestamp)
		return nil, libHTTP.CursorPagination{}, err
	}

	// Find the last operations for each balance of the account before the timestamp
	operations, cur, err := uc.OperationRepo.FindLastOperationsForAccountBeforeTimestamp(ctx, organizationID, ledgerID, accountID, timestamp, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operations for account before timestamp", err)
		logger.Errorf("Error getting operations for account before timestamp: %v", err)
		return nil, libHTTP.CursorPagination{}, err
	}

	// Get all balances for this account to check for balances without operations
	allAccountBalances, err := uc.BalanceRepo.ListByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get all balances for account", err)
		logger.Errorf("Error getting all balances for account: %v", err)
		return nil, libHTTP.CursorPagination{}, err
	}

	// Create a map of balance IDs that have operations
	balanceIDsWithOperations := make(map[string]bool)
	for _, op := range operations {
		balanceIDsWithOperations[op.BalanceID] = true
	}

	// Create a map of balance ID to current balance for quick lookup
	currentBalanceMap := make(map[string]*mmodel.Balance)
	for _, b := range allAccountBalances {
		currentBalanceMap[b.ID] = b
	}

	// Build balance responses
	balances := make([]*mmodel.Balance, 0)

	// First, add balances from operations (balances with transaction history)
	for _, op := range operations {
		currentBalance := currentBalanceMap[op.BalanceID]
		if currentBalance == nil {
			// Balance might have been deleted after the timestamp, use operation data
			logger.Warnf("Balance %s not found in current state, using operation data", op.BalanceID)
			currentBalance = &mmodel.Balance{
				ID: op.BalanceID,
			}
		}

		// Dereference pointers with zero-value fallbacks for nil
		available := decimal.Zero
		if op.BalanceAfter.Available != nil {
			available = *op.BalanceAfter.Available
		}

		onHold := decimal.Zero
		if op.BalanceAfter.OnHold != nil {
			onHold = *op.BalanceAfter.OnHold
		}

		var version int64
		if op.BalanceAfter.Version != nil {
			version = *op.BalanceAfter.Version
		}

		balance := &mmodel.Balance{
			ID:             currentBalance.ID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      op.AccountID,
			Alias:          currentBalance.Alias,
			Key:            op.BalanceKey,
			AssetCode:      op.AssetCode,
			Available:      available,
			OnHold:         onHold,
			Version:        version,
			AccountType:    currentBalance.AccountType,
			CreatedAt:      currentBalance.CreatedAt,
			UpdatedAt:      op.CreatedAt, // The timestamp of the last operation
		}
		balances = append(balances, balance)
	}

	// Then, add balances without operations that existed at the timestamp
	for _, currentBalance := range allAccountBalances {
		// Skip if this balance already has operations
		if balanceIDsWithOperations[currentBalance.ID] {
			continue
		}

		// Check if the balance existed at the timestamp (created_at <= timestamp)
		if currentBalance.CreatedAt.After(timestamp) {
			logger.Debugf("Balance %s was created after timestamp %s, skipping", currentBalance.ID, timestamp.Format(time.RFC3339))
			continue
		}

		// Balance existed but had no operations yet - add with zero values
		logger.Infof("Balance %s existed at timestamp %s but had no operations, adding with initial state", currentBalance.ID, timestamp.Format(time.RFC3339))
		balance := &mmodel.Balance{
			ID:             currentBalance.ID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      currentBalance.AccountID,
			Alias:          currentBalance.Alias,
			Key:            currentBalance.Key,
			AssetCode:      currentBalance.AssetCode,
			Available:      decimal.Zero,
			OnHold:         decimal.Zero,
			Version:        0,
			AccountType:    currentBalance.AccountType,
			CreatedAt:      currentBalance.CreatedAt,
			UpdatedAt:      currentBalance.CreatedAt, // Use created_at as updated_at for initial state
		}
		balances = append(balances, balance)
	}

	// Check if we have any balances to return
	if len(balances) == 0 {
		// No balances existed at that time
		err := pkg.ValidateBusinessError(constant.ErrNoBalanceDataAtTimestamp, timestamp.Format(time.RFC3339))
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "No balance data available for the specified timestamp", err)
		logger.Warnf("No balances found for account %s at timestamp %s", accountID, timestamp)
		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Infof("Successfully retrieved %d balances for account %s at timestamp %s", len(balances), accountID, timestamp.Format(time.RFC3339))

	return balances, cur, nil
}
