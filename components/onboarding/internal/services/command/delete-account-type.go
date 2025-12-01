// Package command provides CQRS command handlers for the onboarding component.
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

// DeleteAccountTypeByID removes an account type from the repository.
//
// This function deletes an account type from the chart of accounts.
// Account types define the categorization structure for accounts and
// may have referential constraints with existing accounts.
//
// # Deletion Constraints
//
// Before deletion, consider:
//   - Accounts using this type should be reassigned or deleted
//   - Reporting configurations may reference this type
//   - External integrations may depend on the KeyValue
//
// # Referential Integrity
//
// The repository layer may enforce:
//   - Prevent deletion if accounts exist with this type
//   - Cascade updates to affected accounts
//   - Soft delete to preserve historical references
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.delete_account_type_by_id"
//  3. Log deletion initiation with account type ID
//  4. Call repository Delete method
//  5. Handle not found error (ErrAccountTypeNotFound)
//  6. Handle other database errors
//  7. Log success
//  8. Return success or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing this account type
//   - id: The UUID of the account type to delete
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Account type not found (ErrAccountTypeNotFound)
//   - Referential integrity violation (accounts use this type)
//   - Database operation fails
//
// # Error Scenarios
//
//   - ErrAccountTypeNotFound: No account type with given ID
//   - Referential integrity violation (has associated accounts)
//   - Database connection failure
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.delete_account_type_by_id" with error events.
// Logs account type ID at info level for both initiation and success,
// warnings for not found, errors for failures.
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
