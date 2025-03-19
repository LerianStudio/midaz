package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// UpdateSegmentByID update a segment from the repository by given id.
func (uc *UseCase) UpdateSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdateSegmentInput) (*mmodel.Segment, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a new segment operation with telemetry for update
	op := uc.Telemetry.NewSegmentOperation("update", id.String())

	// Add important attributes for telemetry
	op.WithAttributes(
		attribute.String("segment_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	if upi.Name != "" {
		op.WithAttribute("segment_name", upi.Name)
	}

	// Record system metric
	op.RecordSystemicMetric(ctx)

	// Start trace span for this operation
	ctx = op.StartTrace(ctx)

	defer func() {
		// End span will be done by op.End() at the end of the function
	}()

	logger.Infof("Trying to update segment: %v", upi)

	segment := &mmodel.Segment{
		Name:   upi.Name,
		Status: upi.Status,
	}

	segmentUpdated, err := uc.SegmentRepo.Update(ctx, organizationID, ledgerID, id, segment)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update segment on repo by id", err)

		logger.Errorf("Error updating segment on repo by id: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())
		}

		return nil, err
	}

	// Add segment info to telemetry
	op.WithAttribute("segment_name", segmentUpdated.Name)

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update metadata on repo by id", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_metadata_error", err)

		return nil, err
	}

	segmentUpdated.Metadata = metadataUpdated

	// Mark operation as successful
	op.End(ctx, "success")

	return segmentUpdated, nil
}
