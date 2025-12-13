package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataSegments fetch all Segments from the repository
func (uc *UseCase) GetAllMetadataSegments(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_segments")
	defer span.End()

	logger.Infof("Retrieving segments")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoSegmentsFound, reflect.TypeOf(mmodel.Segment{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo by query params", err)

		logger.Warn("No metadata found")

		return nil, fmt.Errorf("validation failed: %w", err)
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	segments, err := uc.SegmentRepo.FindByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		logger.Errorf("Error getting segments on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoSegmentsFound, reflect.TypeOf(mmodel.Segment{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get segments on repo by query params", err)

			logger.Warn("No segments found")

			return nil, fmt.Errorf("validation failed: %w", err)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get segments on repo by query params", err)

		return nil, fmt.Errorf("validation failed: %w", err)
	}

	for i := range segments {
		if data, ok := metadataMap[segments[i].ID]; ok {
			segments[i].Metadata = data
		}
	}

	return segments, nil
}
