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

// CreateAdditionalBalance creates a secondary balance for an existing account.
//
// Additional balances allow accounts to segregate funds for different purposes
// (e.g., escrow, pending settlements, reserved funds) while maintaining a
// single account identity. Each additional balance has a unique key.
//
// Use Cases for Additional Balances:
//
//	Escrow: "@merchant/escrow" holds funds during dispute resolution
//	Pending: "@user/pending" holds funds awaiting confirmation
//	Reserved: "@treasury/reserved" holds regulatory reserve requirements
//	Multi-currency: "@user/usd", "@user/eur" for currency segregation
//
// Creation Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Check Duplicate Key
//	  - Verify no balance exists with the requested key
//	  - Return ErrDuplicatedAliasKeyValue if duplicate
//
//	Step 3: Fetch Default Balance
//	  - Retrieve the account's default balance
//	  - Inherit properties (alias, asset code, account type)
//
//	Step 4: Validate Account Type
//	  - External accounts cannot have additional balances
//	  - Only internal/liability accounts support multiple balances
//
//	Step 5: Build Additional Balance
//	  - Generate UUIDv7 for the new balance
//	  - Inherit alias, asset code, and account type from default
//	  - Set transfer permissions (default: both enabled)
//
//	Step 6: Persist Balance
//	  - Store in PostgreSQL
//	  - Handle constraint violations
//
// Inheritance from Default Balance:
//
// Additional balances inherit read-only properties from the default balance:
//   - Alias: Same account alias for identification
//   - AssetCode: Same currency/asset (prevents mixed-currency accounts)
//   - AccountType: Same type for consistent behavior
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - accountID: Account to add the balance to
//   - cbi: Creation input with Key and optional AllowSending/AllowReceiving
//
// Returns:
//   - *mmodel.Balance: Created additional balance
//   - error: Validation or database error
//
// Error Scenarios:
//   - ErrDuplicatedAliasKeyValue: Balance key already exists for this account
//   - ErrAdditionalBalanceNotAllowed: Account type doesn't support additional balances
//   - Default balance not found: Account doesn't have a default balance
//   - Database errors: PostgreSQL unavailable
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
