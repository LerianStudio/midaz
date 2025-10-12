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

// UpdateAccountType updates an existing account type in the repository.
//
// This function performs a partial update of account type properties. The keyValue
// field is immutable and cannot be changed after creation, as it's used as a reference
// in account creation and validation logic.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the account type
//   - ledgerID: The UUID of the ledger containing the account type
//   - id: The UUID of the account type to update
//   - input: The update input containing fields to modify (name, description, metadata)
//
// Returns:
//   - *mmodel.AccountType: The updated account type with refreshed metadata
//   - error: ErrAccountTypeNotFound if not found, or repository errors
func (uc *UseCase) UpdateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account_type")
	defer span.End()

	logger.Infof("Trying to update account type: %v", input)

	accountType := &mmodel.AccountType{
		Name:        input.Name,
		Description: input.Description,
	}

	accountTypeUpdated, err := uc.AccountTypeRepo.Update(ctx, organizationID, ledgerID, id, accountType)
	if err != nil {
		logger.Errorf("Error updating account type on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

			logger.Warnf("Account type ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account type on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account type on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.AccountType{}).Name(), id.String(), input.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata", err)

		return nil, err
	}

	accountTypeUpdated.Metadata = metadataUpdated

	logger.Infof("Successfully updated account type with ID: %s", accountTypeUpdated.ID)

	return accountTypeUpdated, nil
}
