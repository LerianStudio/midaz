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

// UpdateSegmentByID updates an existing segment in the repository.
//
// This method implements the update segment use case, which:
// 1. Updates the segment in PostgreSQL
// 2. Updates associated metadata in MongoDB using merge semantics
// 3. Returns the updated segment with merged metadata
//
// Business Rules:
//   - Only provided fields are updated (partial updates supported)
//   - Name can be updated
//   - Status can be updated
//   - Metadata is merged with existing
//
// Update Behavior:
//   - Empty strings in input are treated as "clear the field"
//   - Empty status means "don't update status"
//   - Metadata is merged with existing metadata (RFC 7396)
//
// Data Storage:
//   - Primary data: PostgreSQL (segments table)
//   - Metadata: MongoDB (merged with existing)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the segment to update
//   - upi: Update segment input with fields to update
//
// Returns:
//   - *mmodel.Segment: Updated segment with merged metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrSegmentIDNotFound: Segment doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.UpdateSegmentInput{
//	    Name:   "North America Region - Updated",
//	    Status: mmodel.Status{Code: "ACTIVE"},
//	}
//	segment, err := useCase.UpdateSegmentByID(ctx, orgID, ledgerID, segmentID, input)
//
// OpenTelemetry:
//   - Creates span "command.update_segment_by_id"
//   - Records errors as span events
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
