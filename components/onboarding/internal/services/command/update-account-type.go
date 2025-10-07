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

// UpdateAccountType updates an existing account type in the repository.
//
// This method implements the update account type use case, which:
// 1. Updates the account type in PostgreSQL
// 2. Updates associated metadata in MongoDB using merge semantics
// 3. Returns the updated account type with merged metadata
//
// Business Rules:
//   - Only provided fields are updated (partial updates supported)
//   - Name can be updated
//   - Description can be updated
//   - Key value cannot be updated (immutable, not in update input)
//   - Metadata is merged with existing
//
// Update Behavior:
//   - Empty strings in input are treated as "clear the field"
//   - Metadata is merged with existing metadata (RFC 7396)
//
// Data Storage:
//   - Primary data: PostgreSQL (account_types table)
//   - Metadata: MongoDB (merged with existing)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the account type to update
//   - input: Update account type input with fields to update
//
// Returns:
//   - *mmodel.AccountType: Updated account type with merged metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrAccountTypeNotFound: Account type doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.UpdateAccountTypeInput{
//	    Name:        "Current Assets - Updated",
//	    Description: "Updated description",
//	}
//	accountType, err := useCase.UpdateAccountType(ctx, orgID, ledgerID, typeID, input)
//
// OpenTelemetry:
//   - Creates span "command.update_account_type"
//   - Records errors as span events
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
