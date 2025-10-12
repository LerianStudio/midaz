package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// DeleteAccountByID performs a soft delete of an account from the repository.
//
// This function implements soft delete (setting DeletedAt timestamp) rather than
// hard delete to maintain audit trails and referential integrity. Deleted accounts
// remain in the database but are excluded from normal queries.
//
// Critical financial safeguards:
//   - External accounts (e.g., "@external/USD") cannot be deleted as they are
//     system-managed boundary accounts required for asset flow tracking
//   - Accounts with remaining balances should not be deleted (enforced by repository layer)
//   - Sub-accounts cascade considerations handled by repository layer
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the account
//   - ledgerID: The UUID of the ledger containing the account
//   - portfolioID: Optional portfolio UUID for scoped deletion
//   - id: The UUID of the account to delete
//
// Returns:
//   - error: ErrForbiddenExternalAccountManipulation if external account,
//     ErrAccountIDNotFound if not found, or repository errors
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	logger.Infof("Remove account for id: %s", id.String())

	// Step 1: Verify account exists and check if it's an external account.
	// External accounts are system-managed and cannot be deleted.
	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

		logger.Errorf("Error finding account by alias: %v", err)

		return err
	}

	// Prevent deletion of external accounts which are required system boundaries
	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	// Step 2: Perform soft delete by setting DeletedAt timestamp.
	// Repository layer enforces balance checks and other constraints.
	if err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("Account ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete account on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete account on repo by id", err)

		logger.Errorf("Error deleting account: %v", err)

		return err
	}

	return nil
}
