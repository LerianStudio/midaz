package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// GetSegmentByID get a Segment from the repository by given id.
func (uc *UseCase) GetSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_segment_by_id")
	defer span.End()

	logger.Infof("Retrieving segment for id: %s", id.String())

	segment, err := uc.SegmentRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Error getting segment on repo by id: %v", err)

		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get segment on repo by id", err)

			logger.Warn("No segment found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get segment on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Segment{}).Name())
	}

	if segment != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb segment", err)

			logger.Warn("No metadata found")

			return nil, err
		}

		if metadata != nil {
			segment.Metadata = metadata.Data
		}
	}

	return segment, nil
}
