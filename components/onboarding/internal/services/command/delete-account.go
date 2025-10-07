// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

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

// DeleteAccountByID soft-deletes an account from the repository.
//
// This method implements the delete account use case, which:
// 1. Fetches the account to validate it exists
// 2. Prevents deletion of external accounts (system-managed)
// 3. Performs soft delete by setting DeletedAt timestamp
// 4. Account remains in database for audit purposes
//
// Business Rules:
//   - External accounts cannot be deleted (type "external")
//   - Account must exist and not be already deleted
//   - Soft delete is idempotent (deleting already deleted account returns error)
//   - Account balances should be zero before deletion (enforced by transaction service)
//
// Soft Deletion:
//   - Sets DeletedAt timestamp to current time
//   - Account remains in database for audit and historical reporting
//   - Excluded from normal queries (WHERE deleted_at IS NULL)
//   - Cannot be undeleted (no restore operation)
//
// External Accounts:
//   - Created automatically with assets
//   - Have alias "@external/{ASSET_CODE}"
//   - Cannot be modified or deleted
//   - Used for tracking external system transactions
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - portfolioID: Optional portfolio ID filter
//   - id: UUID of the account to delete
//
// Returns:
//   - error: Business error if validation fails, database error if deletion fails
//
// Possible Errors:
//   - ErrAccountIDNotFound: Account doesn't exist or already deleted
//   - ErrForbiddenExternalAccountManipulation: Attempting to delete external account
//   - ErrAccountBalanceDeletion: Account has remaining balance (enforced elsewhere)
//   - Database errors: Connection failures
//
// Example:
//
//	err := useCase.DeleteAccountByID(ctx, orgID, ledgerID, nil, accountID)
//	if err != nil {
//	    return err
//	}
//	// Account is soft-deleted
//
// OpenTelemetry:
//   - Creates span "command.delete_account_by_id"
//   - Records errors as span events
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	logger.Infof("Remove account for id: %s", id.String())

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

		logger.Errorf("Error finding account by alias: %v", err)

		return err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

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
