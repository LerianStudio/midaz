package query

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// GetBalanceAtTimestamp retrieves the balance state at a specific point in time.
// It finds the last operation before the given timestamp and returns the balance state after that operation.
func (uc *UseCase) GetBalanceAtTimestamp(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, timestamp time.Time) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balance_at_timestamp")
	defer span.End()

	logger.Infof("Retrieving balance %s at timestamp %s", balanceID, timestamp.Format(time.RFC3339))

	// Validate timestamp is not in the future
	if timestamp.After(time.Now()) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTimestamp, "timestamp cannot be in the future")
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Timestamp is in the future", err)
		logger.Warnf("Timestamp is in the future: %s", timestamp)

		return nil, err
	}

	// First, verify the balance exists (current state)
	currentBalance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get current balance", err)
		logger.Errorf("Error getting current balance: %v", err)

		return nil, err
	}

	if currentBalance == nil {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance not found", err)
		logger.Warnf("Balance not found: %s", balanceID)

		return nil, err
	}

	// Find the last operation before the timestamp
	operation, err := uc.OperationRepo.FindLastOperationBeforeTimestamp(ctx, organizationID, ledgerID, balanceID, timestamp)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operation before timestamp", err)
		logger.Errorf("Error getting operation before timestamp: %v", err)

		return nil, err
	}

	var balance *mmodel.Balance

	if operation == nil {
		// No operation found before the timestamp
		// Check if the balance existed at the timestamp (created_at <= timestamp)
		if currentBalance.CreatedAt.After(timestamp) {
			// Balance didn't exist at that time
			err := pkg.ValidateBusinessError(constant.ErrNoBalanceDataAtTimestamp, timestamp.Format(time.RFC3339))
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "No balance data available for the specified timestamp", err)
			logger.Warnf("Balance %s was created after timestamp %s (created_at: %s)", balanceID, timestamp, currentBalance.CreatedAt)

			return nil, err
		}

		// Balance existed but had no operations yet - return with zero values
		logger.Infof("Balance %s existed at timestamp %s but had no operations, returning initial state", balanceID, timestamp.Format(time.RFC3339))

		balance = &mmodel.Balance{
			ID:             currentBalance.ID,
			OrganizationID: currentBalance.OrganizationID,
			LedgerID:       currentBalance.LedgerID,
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
	} else {
		// Build the balance response from the operation's "after" state
		// Dereference pointers with zero-value fallbacks for nil
		available := decimal.Zero
		if operation.BalanceAfter.Available != nil {
			available = *operation.BalanceAfter.Available
		}

		onHold := decimal.Zero
		if operation.BalanceAfter.OnHold != nil {
			onHold = *operation.BalanceAfter.OnHold
		}

		var version int64
		if operation.BalanceAfter.Version != nil {
			version = *operation.BalanceAfter.Version
		}

		balance = &mmodel.Balance{
			ID:             currentBalance.ID,
			OrganizationID: currentBalance.OrganizationID,
			LedgerID:       currentBalance.LedgerID,
			AccountID:      operation.AccountID,
			Alias:          currentBalance.Alias,
			Key:            operation.BalanceKey,
			AssetCode:      operation.AssetCode,
			Available:      available,
			OnHold:         onHold,
			Version:        version,
			AccountType:    currentBalance.AccountType,
			CreatedAt:      currentBalance.CreatedAt,
			UpdatedAt:      operation.CreatedAt, // The timestamp of the last operation
		}
	}

	logger.Infof("Successfully retrieved balance %s at timestamp %s", balanceID, timestamp.Format(time.RFC3339))

	return balance, nil
}
