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

// UpdateAccount updates an existing account's properties and metadata.
//
// Accounts are the fundamental units of the ledger system, representing entities
// that can hold balances and participate in transactions. This method allows
// updating various account properties while enforcing protection rules for
// system-managed external accounts.
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "command.update_account"
//
//	Step 2: Account Validation
//	  - Find existing account by ID within organization and ledger scope
//	  - If account not found: Return error from repository
//	  - If account is "external" type: Return ErrForbiddenExternalAccountManipulation
//	    (External accounts are system-managed and cannot be modified)
//
//	Step 3: Input Mapping
//	  - Map UpdateAccountInput to Account model
//	  - Updateable fields: Name, Status, EntityID, SegmentID, PortfolioID, Metadata, Blocked
//
//	Step 4: PostgreSQL Update
//	  - Call AccountRepo.Update with organization and ledger scope
//	  - If account not found during update: Return ErrAccountIDNotFound business error
//	  - If other error: Return wrapped error with span event
//
//	Step 5: Metadata Update
//	  - Call UpdateMetadata for MongoDB metadata merge
//	  - If metadata update fails: Return error
//
//	Step 6: Response Assembly
//	  - Attach updated metadata to account entity
//	  - Return complete updated account
//
// External Account Protection:
//
// External accounts are automatically created during asset creation to serve as
// the counterparty for external transactions. These accounts:
//   - Have type="external" and cannot be modified
//   - Attempting to update returns ErrForbiddenExternalAccountManipulation
//
// Updateable Fields:
//
//   - Name: Display name of the account
//   - Status: Account status (e.g., ACTIVE, INACTIVE, BLOCKED)
//   - EntityID: Reference to an external entity (customer, vendor, etc.)
//   - SegmentID: Logical grouping within the ledger
//   - PortfolioID: Portfolio association for account aggregation
//   - Blocked: Sending/receiving block flags
//   - Metadata: Arbitrary key-value metadata
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the account
//   - portfolioID: Optional portfolio UUID (nil if account has no portfolio)
//   - id: UUID of the account to update
//   - uai: Update input containing optional fields to update
//
// Returns:
//   - *mmodel.Account: Updated account with merged metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrForbiddenExternalAccountManipulation: Cannot update external accounts
//   - ErrAccountIDNotFound: Account does not exist
//   - Database connection failure
//   - MongoDB metadata update failure
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
		Blocked:     uai.Blocked,
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
