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

// UpdateSegmentByID updates an existing segment's properties and metadata.
//
// Segments are logical groupings of accounts within a ledger, used for
// organizational and reporting purposes (e.g., "retail", "corporate", "treasury").
// This method allows updating the name, status, and metadata of an existing segment.
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "command.update_segment_by_id"
//
//	Step 2: Input Mapping
//	  - Map UpdateSegmentInput to Segment model
//	  - Only name and status fields are updateable via this method
//
//	Step 3: PostgreSQL Update
//	  - Call SegmentRepo.Update with organization and ledger scope
//	  - If segment not found: Return ErrSegmentIDNotFound business error
//	  - If other error: Return wrapped error with span event
//
//	Step 4: Metadata Update
//	  - Call UpdateMetadata for MongoDB metadata merge
//	  - If metadata update fails: Return error
//
//	Step 5: Response Assembly
//	  - Attach updated metadata to segment entity
//	  - Return complete updated segment
//
// Business Rules:
//
//   - Segment must exist within the specified organization and ledger
//   - Name updates are allowed (uniqueness enforced by repository)
//   - Status can be updated (e.g., ACTIVE -> INACTIVE)
//   - Metadata follows merge semantics (see UpdateMetadata)
//   - Accounts referencing this segment are not affected by status changes
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the segment
//   - id: UUID of the segment to update
//   - upi: Update input containing optional name, status, and metadata
//
// Returns:
//   - *mmodel.Segment: Updated segment with merged metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrSegmentIDNotFound: Segment does not exist
//   - Database connection failure
//   - MongoDB metadata update failure
func (uc *UseCase) UpdateSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_segment_by_id")
	defer span.End()

	logger.Infof("Trying to update segment: %v", upi)

	segment := &mmodel.Segment{
		Name:   upi.Name,
		Status: upi.Status,
	}

	segmentUpdated, err := uc.SegmentRepo.Update(ctx, organizationID, ledgerID, id, segment)
	if err != nil {
		logger.Errorf("Error updating segment on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())

			logger.Warnf("Segment ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update segment on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update segment on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	segmentUpdated.Metadata = metadataUpdated

	return segmentUpdated, nil
}
