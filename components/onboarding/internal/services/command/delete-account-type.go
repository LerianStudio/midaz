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

// DeleteAccountTypeByID soft-deletes an account type from the repository.
//
// This method implements the delete account type use case, which performs a soft delete
// by setting the DeletedAt timestamp. The account type record remains in the database
// but is excluded from normal queries.
//
// Business Rules:
//   - Account type must exist and not be already deleted
//   - Account type should not be referenced by existing accounts if validation is enabled
//   - Soft delete is idempotent (deleting already deleted type returns error)
//
// Soft Deletion:
//   - Sets DeletedAt timestamp to current time
//   - Account type remains in database for audit purposes
//   - Excluded from list and get operations (WHERE deleted_at IS NULL)
//   - Can be used for historical reporting
//   - Cannot be undeleted (no restore operation)
//
// Impact on Accounts:
//   - Existing accounts with this type are NOT affected
//   - New accounts cannot use this type (if validation is enabled)
//   - Consider the impact before deleting widely-used types
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the account type to delete
//
// Returns:
//   - error: Business error if account type not found, database error if deletion fails
//
// Possible Errors:
//   - ErrAccountTypeNotFound: Account type doesn't exist or already deleted
//   - Database errors: Connection failures
//
// Example:
//
//	err := useCase.DeleteAccountTypeByID(ctx, orgID, ledgerID, typeID)
//	if err != nil {
//	    return err
//	}
//	// Account type is soft-deleted
//
// OpenTelemetry:
//   - Creates span "command.delete_account_type_by_id"
//   - Records errors as span events
func (uc *UseCase) DeleteAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_type_by_id")
	defer span.End()

	logger.Infof("Initiating deletion of Account Type with Account Type ID: %s", id.String())

	if err := uc.AccountTypeRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		logger.Errorf("Failed to delete Account Type with Account Type ID: %s, Error: %s", id.String(), err.Error())

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

			logger.Warnf("Account Type ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete Account Type on repo", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete Account Type on repo", err)

		return err
	}

	logger.Infof("Successfully deleted Account Type with Account Type ID: %s", id.String())

	return nil
}
