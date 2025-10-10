// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for deleting a segment.
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
// This use case performs a soft delete by setting the DeletedAt timestamp.
// The segment record is preserved for auditing but is excluded from normal queries.
// Accounts referencing the segment are not automatically updated.
//
// Business Rules:
//   - The segment must exist and not have been previously deleted.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the segment to be deleted.
//
// Returns:
//   - error: An error if the segment is not found or if the deletion fails.
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
