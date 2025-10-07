// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

package query

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
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllSegments retrieves a paginated list of segments with metadata.
//
// Fetches segments from PostgreSQL with pagination, then enriches with MongoDB metadata.
// Returns empty array if no segments found (not an error). Excludes soft-deleted segments.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - filter: Query parameters (pagination, sorting, date range)
//
// Returns:
//   - []*mmodel.Segment: Array of segments with metadata
//   - error: Business error if query fails
//
// OpenTelemetry: Creates span "query.get_all_segments"
func (uc *UseCase) GetAllSegments(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_segments")
	defer span.End()

	logger.Infof("Retrieving segments")

	segments, err := uc.SegmentRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		logger.Errorf("Error getting segments on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoSegmentsFound, reflect.TypeOf(mmodel.Segment{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get segments on repo", err)

			logger.Warn("No segments found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get segments on repo", err)

		return nil, err
	}

	if len(segments) == 0 {
		return segments, nil
	}

	segmentIDs := make([]string, len(segments))
	for i, s := range segments {
		segmentIDs[i] = s.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), segmentIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoSegmentsFound, reflect.TypeOf(mmodel.Segment{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
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

	return segments, nil
}
