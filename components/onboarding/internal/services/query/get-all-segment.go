package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"reflect"
)

// GetAllSegments fetch all Segment from the repository
func (uc *UseCase) GetAllSegments(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Segment, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_segments")
	defer span.End()

	logger.Infof("Retrieving segments")

	segments, err := uc.SegmentRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get segments on repo", err)

		logger.Errorf("Error getting segments on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoSegmentsFound, reflect.TypeOf(mmodel.Segment{}).Name())
		}

		return nil, err
	}

	if segments != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

			return nil, pkg.ValidateBusinessError(constant.ErrNoSegmentsFound, reflect.TypeOf(mmodel.Segment{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range segments {
			if data, ok := metadataMap[segments[i].ID]; ok {
				segments[i].Metadata = data
			}
		}
	}

	return segments, nil
}
