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

// CreateAdditionalBalance creates an additional balance bucket for an account.
//
// Additional balances enable sub-categorization of funds within a single account.
// For example, a checking account might have:
// - "default": Main available balance
// - "rewards": Loyalty points balance
// - "pending_transfers": Incoming transfers not yet cleared
//
// Use Cases:
// - Separate balance buckets with different permissions (sending/receiving)
// - Hold management (freeze funds in a separate bucket)
// - Multi-purpose account segmentation
//
// Business Rules:
// - External accounts cannot have additional balances (system-managed only)
// - Each balance key must be unique per account
// - Inherits account properties (alias, asset code, etc.) from default balance
// - Permissions default to true if not specified
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning the account
//   - ledgerID: Ledger UUID containing the account
//   - accountID: Account UUID to create additional balance for
//   - cbi: Balance creation input with key and permission flags
//
// Returns:
//   - *mmodel.Balance: The created additional balance
//   - error: ErrDuplicatedAliasKeyValue if key exists, ErrAdditionalBalanceNotAllowed for external accounts
func (uc *UseCase) CreateAdditionalBalance(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, cbi *mmodel.CreateAdditionalBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_additional_balance")
	defer span.End()

	// Step 1: Check if balance with this key already exists
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

	// Step 2: Fetch default balance to inherit account properties
	defaultBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, constant.DefaultBalanceKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get default balance", err)

		logger.Errorf("Failed to get default balance: %v", err)

		return nil, err
	}

	// Step 3: Prevent additional balances for external accounts
	if defaultBalance.AccountType == constant.ExternalAccountType {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Additional balance not allowed for external account type", nil)

		return nil, pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, reflect.TypeOf(mmodel.Balance{}).Name(), defaultBalance.Alias)
	}

	// Step 4: Create additional balance inheriting from default
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
