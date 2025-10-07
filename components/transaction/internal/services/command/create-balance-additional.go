// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateAdditionalBalance creates an additional balance entry for an account.
//
// This method implements additional balance creation, which allows accounts to have
// multiple balance entries with different keys (e.g., "available", "pending", "reserved").
// It:
// 1. Checks if balance with the key already exists (returns error if duplicate)
// 2. Fetches the default balance to get account details
// 3. Validates account type (external accounts cannot have additional balances)
// 4. Creates new balance entry with specified key
// 5. Initializes with zero available and on-hold amounts
// 6. Sets allow_sending and allow_receiving flags
//
// Business Rules:
//   - Balance key must be unique per account
//   - External accounts cannot have additional balances
//   - Key is normalized to lowercase
//   - Inherits account details from default balance
//   - Initial amounts are zero
//   - Allow flags default to true if not specified
//
// Use Cases:
//   - Segregating funds (available vs reserved)
//   - Holding funds temporarily
//   - Multi-currency sub-accounts
//   - Custom balance tracking
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - accountID: UUID of the account
//   - cbi: Create additional balance input with key and flags
//
// Returns:
//   - *mmodel.Balance: Created additional balance
//   - error: Business error if duplicate key or external account
//
// OpenTelemetry: Creates span "command.create_additional_balance"
func (uc *UseCase) CreateAdditionalBalance(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, cbi *mmodel.CreateAdditionalBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_additional_balance")
	defer span.End()

	existingBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, strings.ToLower(cbi.Key))
	if err != nil {
		var notFound pkg.EntityNotFoundError
		if !errors.As(err, &notFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check if additional balance already exists", err)

			logger.Errorf("Failed to check if additional balance already exists: %v", err)

			return nil, err
		}
	}

	if existingBalance != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Additional balance already exists", nil)

		logger.Infof("Additional balance already exists: %v", cbi.Key)

		return nil, pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, reflect.TypeOf(mmodel.Balance{}).Name(), cbi.Key)
	}

	defaultBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, constant.DefaultBalanceKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get default balance", err)

		logger.Errorf("Failed to get default balance: %v", err)

		return nil, err
	}

	if defaultBalance.AccountType == constant.ExternalAccountType {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Additional balance not allowed for external account type", nil)

		return nil, pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, reflect.TypeOf(mmodel.Balance{}).Name(), defaultBalance.Alias)
	}

	additionalBalance := &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		Alias:          defaultBalance.Alias,
		Key:            strings.ToLower(cbi.Key),
		OrganizationID: defaultBalance.OrganizationID,
		LedgerID:       defaultBalance.LedgerID,
		AccountID:      defaultBalance.AccountID,
		AssetCode:      defaultBalance.AssetCode,
		AccountType:    defaultBalance.AccountType,
		AllowSending:   cbi.AllowSending == nil || *cbi.AllowSending,
		AllowReceiving: cbi.AllowReceiving == nil || *cbi.AllowReceiving,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err = uc.BalanceRepo.Create(ctx, additionalBalance)
	if err != nil {
		logger.Errorf("Error creating additional balance on repo: %v", err)

		return nil, err
	}

	return additionalBalance, nil
}
