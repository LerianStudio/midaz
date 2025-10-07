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

// DeleteSegmentByID soft-deletes a segment from the repository.
//
// This method implements the delete segment use case, which performs a soft delete
// by setting the DeletedAt timestamp. The segment record remains in the database
// but is excluded from normal queries.
//
// Business Rules:
//   - Segment must exist and not be already deleted
//   - Segment should not have active accounts referencing it
//   - Soft delete is idempotent (deleting already deleted segment returns error)
//
// Soft Deletion:
//   - Sets DeletedAt timestamp to current time
//   - Segment remains in database for audit purposes
//   - Excluded from list and get operations (WHERE deleted_at IS NULL)
//   - Can be used for historical reporting
//   - Cannot be undeleted (no restore operation)
//
// Cascade Behavior:
//   - Accounts referencing this segment are NOT automatically updated
//   - Consider updating accounts to remove segment reference before deletion
//   - Foreign key constraints may prevent deletion if accounts reference it
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the segment to delete
//
// Returns:
//   - error: Business error if segment not found, database error if deletion fails
//
// Possible Errors:
//   - ErrSegmentIDNotFound: Segment doesn't exist or already deleted
//   - Database errors: Foreign key violations, connection failures
//
// Example:
//
//	err := useCase.DeleteSegmentByID(ctx, orgID, ledgerID, segmentID)
//	if err != nil {
//	    return err
//	}
//	// Segment is soft-deleted
//
// OpenTelemetry:
//   - Creates span "command.delete_segment_by_id"
//   - Records errors as span events
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
