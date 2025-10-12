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
// This function performs a partial update of account properties. Only the fields
// provided in the input will be updated; omitted fields remain unchanged.
//
// Important business rules:
// - External accounts (system-managed boundary accounts) cannot be modified
// - Account type and asset code are immutable and cannot be changed after creation
// - Metadata updates follow JSON Merge Patch semantics (RFC 7396)
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the account
//   - ledgerID: The UUID of the ledger containing the account
//   - portfolioID: Optional portfolio UUID for scoped queries
//   - id: The UUID of the account to update
//   - uai: The update input containing fields to modify
//
// Returns:
//   - *mmodel.Account: The updated account with refreshed metadata
//   - error: ErrForbiddenExternalAccountManipulation if external, ErrAccountIDNotFound if not found
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, uai *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account")
	defer span.End()

	logger.Infof("Trying to update account: %v", uai)

	// Step 1: Verify account exists and check if it's an external account.
	// External accounts cannot be modified as they are system-managed.
	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		logger.Errorf("Error finding account by alias: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

		return nil, err
	}

	// Prevent modification of external accounts (e.g., "@external/USD").
	// External accounts are automatically created by the system for asset boundaries.
	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return nil, pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	// Step 2: Construct partial update entity with only provided fields
	account := &mmodel.Account{
		Name:        uai.Name,
		Status:      uai.Status,
		EntityID:    uai.EntityID,
		SegmentID:   uai.SegmentID,
		PortfolioID: uai.PortfolioID,
		Metadata:    uai.Metadata,
	}

	// Step 3: Persist the update to PostgreSQL
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

	// Step 4: Update metadata in MongoDB using JSON Merge Patch semantics
	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String(), uai.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata", err)

		return nil, err
	}

	accountUpdated.Metadata = metadataUpdated

	return accountUpdated, nil
}
