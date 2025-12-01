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

// UpdateAccountType updates an existing account type's properties and metadata.
//
// Account types define categories for accounts within a ledger (e.g., "checking",
// "savings", "liability"). This method allows updating the name, description,
// and metadata of an existing account type.
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "command.update_account_type"
//
//	Step 2: Input Mapping
//	  - Map UpdateAccountTypeInput to AccountType model
//	  - Only name and description fields are updateable
//
//	Step 3: PostgreSQL Update
//	  - Call AccountTypeRepo.Update with organization and ledger scope
//	  - If account type not found: Return ErrAccountTypeNotFound business error
//	  - If other error: Return wrapped error with span event
//
//	Step 4: Metadata Update
//	  - Call UpdateMetadata for MongoDB metadata merge
//	  - If metadata update fails: Return error
//
//	Step 5: Response Assembly
//	  - Attach updated metadata to account type entity
//	  - Log success with account type ID
//	  - Return complete updated account type
//
// Business Rules:
//
//   - Account type must exist within the specified organization and ledger
//   - Name updates are allowed (uniqueness enforced by repository)
//   - Description is optional and can be cleared by passing empty string
//   - Metadata follows merge semantics (see UpdateMetadata)
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the account type
//   - id: UUID of the account type to update
//   - input: Update input containing optional name, description, and metadata
//
// Returns:
//   - *mmodel.AccountType: Updated account type with merged metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrAccountTypeNotFound: Account type does not exist
//   - Database connection failure
//   - MongoDB metadata update failure
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
