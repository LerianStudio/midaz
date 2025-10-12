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

// DeleteAccountTypeByID performs a soft delete of an account type from the repository.
//
// Soft deletes the account type by setting its DeletedAt timestamp. Existing accounts
// using this type remain valid; the deletion prevents new accounts from using this type.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the account type
//   - ledgerID: The UUID of the ledger containing the account type
//   - id: The UUID of the account type to delete
//
// Returns:
//   - error: ErrAccountTypeNotFound if not found, or repository errors
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
