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

// UpdateAccount updates an existing account in the repository.
//
// This method implements the update account use case, which:
// 1. Fetches the existing account to validate it exists
// 2. Prevents updates to external accounts (system-managed)
// 3. Updates the account in PostgreSQL
// 4. Updates associated metadata in MongoDB using merge semantics
// 5. Returns the updated account with merged metadata
//
// Business Rules:
//   - External accounts cannot be updated (type "external")
//   - This performs a full update. Fields omitted from the input will be overwritten with their zero value (e.g., an empty string for Name), clearing existing data.
//   - Account type cannot be changed (immutable, enforced at HTTP layer)
//   - Asset code cannot be changed (immutable, enforced at HTTP layer)
//   - Alias cannot be changed (immutable, enforced at HTTP layer)
//   - Portfolio and segment can be changed
//   - Status can be updated
//
// Update Behavior:
//   - All fields from input are set, including zero values (empty strings, nil pointers)
//   - Nil pointers in input mean "don't update this field"
//   - Metadata is merged with existing metadata (RFC 7396)
//
// Data Storage:
//   - Primary data: PostgreSQL (accounts table)
//   - Metadata: MongoDB (merged with existing)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - portfolioID: Optional portfolio ID filter
//   - id: UUID of the account to update
//   - uai: Update account input with fields to update
//
// Returns:
//   - *mmodel.Account: Updated account with merged metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrAccountIDNotFound: Account doesn't exist
//   - ErrForbiddenExternalAccountManipulation: Attempting to update external account
//   - ErrPortfolioIDNotFound: New portfolio doesn't exist
//   - ErrSegmentIDNotFound: New segment doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.UpdateAccountInput{
//	    Name:   "Updated Account Name",
//	    Status: mmodel.Status{Code: "INACTIVE"},
//	}
//	account, err := useCase.UpdateAccount(ctx, orgID, ledgerID, nil, accountID, input)
//
// OpenTelemetry:
//   - Creates span "command.update_account"
//   - Records errors as span events
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, uai *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account")
	defer span.End()

	logger.Infof("Trying to update account: %v", uai)

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		logger.Errorf("Error finding account by alias: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

		return nil, err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return nil, pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	account := &mmodel.Account{
		Name:        uai.Name,
		Status:      uai.Status,
		EntityID:    uai.EntityID,
		SegmentID:   uai.SegmentID,
		PortfolioID: uai.PortfolioID,
		Metadata:    uai.Metadata,
	}

	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, portfolioID, id, account)
	if err != nil {
		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("Account ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String(), uai.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata", err)

		return nil, err
	}

	accountUpdated.Metadata = metadataUpdated

	return accountUpdated, nil
}
