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

// UpdateLedgerByID updates an existing ledger's properties and metadata.
//
// This method handles partial updates to a ledger entity, allowing modification
// of name, status, and metadata while preserving other properties. The update
// is performed atomically across PostgreSQL (ledger data) and MongoDB (metadata).
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "command.update_ledger_by_id"
//
//	Step 2: Input Mapping
//	  - Map UpdateLedgerInput fields to Ledger model
//	  - Only non-nil fields from input are considered for update
//
//	Step 3: PostgreSQL Update
//	  - Call LedgerRepo.Update with organization scope
//	  - If ledger not found: Return ErrLedgerIDNotFound business error
//	  - If other error: Return wrapped error with span event
//
//	Step 4: Metadata Update
//	  - Call UpdateMetadata for MongoDB metadata merge
//	  - If metadata update fails: Return error (ledger update not rolled back)
//
//	Step 5: Response Assembly
//	  - Attach updated metadata to ledger entity
//	  - Return complete updated ledger
//
// Business Rules:
//
//   - Ledger must exist within the specified organization
//   - Name updates do not require uniqueness validation (handled by repo)
//   - Status transitions are not validated (any status is allowed)
//   - Metadata follows merge semantics (see UpdateMetadata)
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - id: UUID of the ledger to update
//   - uli: Update input containing optional name, status, and metadata
//
// Returns:
//   - *mmodel.Ledger: Updated ledger with merged metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrLedgerIDNotFound: Ledger does not exist in organization
//   - Database connection failure
//   - MongoDB metadata update failure
func (uc *UseCase) UpdateLedgerByID(ctx context.Context, organizationID, id uuid.UUID, uli *mmodel.UpdateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_ledger_by_id")
	defer span.End()

	logger.Infof("Trying to update ledger: %v", uli)

	ledger := &mmodel.Ledger{
		Name:           uli.Name,
		OrganizationID: organizationID.String(),
		Status:         uli.Status,
	}

	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, organizationID, id, ledger)
	if err != nil {
		logger.Errorf("Error updating ledger on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			logger.Warnf("Ledger ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), id.String(), uli.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo", err)

		return nil, err
	}

	ledgerUpdated.Metadata = metadataUpdated

	return ledgerUpdated, nil
}
