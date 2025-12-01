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

// DeleteSegmentByID deletes a segment from the repository.
//
// Segments are logical groupings of accounts within a ledger. This method
// removes a segment by its ID after validating it exists within the specified
// organization and ledger scope.
//
// Deletion Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "command.delete_segment_by_id"
//	  - Log deletion attempt with segment ID
//
//	Step 2: Segment Deletion
//	  - Call SegmentRepo.Delete with organization and ledger scope
//	  - If segment not found: Return ErrSegmentIDNotFound business error
//	  - If other error: Return wrapped error with span event
//
//	Step 3: Return Success
//	  - Return nil error on successful deletion
//
// Referential Integrity:
//
// The database may enforce referential integrity constraints. If accounts
// reference this segment, the deletion may fail at the database level.
// Callers should handle potential constraint violation errors.
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the segment
//   - id: UUID of the segment to delete
//
// Returns:
//   - error: Business or infrastructure error, nil on success
//
// Error Scenarios:
//   - ErrSegmentIDNotFound: Segment does not exist
//   - Database connection failure
//   - Referential integrity constraint violation (if accounts reference segment)
func (uc *UseCase) DeleteSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_segment_by_id")
	defer span.End()

	logger.Infof("Remove segment for id: %s", id.String())

	if err := uc.SegmentRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())

			logger.Warnf("Segment ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete segment on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete segment on repo by id", err)

		logger.Errorf("Error deleting segment: %v", err)

		return err
	}

	return nil
}
