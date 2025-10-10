// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for deleting an account type.
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
// This use case performs a soft delete by setting the DeletedAt timestamp, which
// preserves the record in the database for auditing purposes but excludes it
// from normal queries.
//
// Business Rules:
//   - The account type must exist and not have been previously deleted.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the account type to be deleted.
//
// Returns:
//   - error: An error if the account type is not found or if the deletion fails.
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
