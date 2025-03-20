package command

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"reflect"
)

// UpdateSegmentByID update a segment from the repository by given id.
func (uc *UseCase) UpdateSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdateSegmentInput) (*mmodel.Segment, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_segment_by_id")
	defer span.End()

	logger.Infof("Trying to update segment: %v", upi)

	segment := &mmodel.Segment{
		Name:   upi.Name,
		Status: upi.Status,
	}

	segmentUpdated, err := uc.SegmentRepo.Update(ctx, organizationID, ledgerID, id, segment)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update segment on repo by id", err)

		logger.Errorf("Error updating segment on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	segmentUpdated.Metadata = metadataUpdated

	return segmentUpdated, nil
}
