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

// GetAllMetadataSegments retrieves segments filtered by metadata criteria.
//
// This method implements a metadata-first query pattern for segments, useful when
// searching by custom attributes stored in MongoDB. Common use cases include
// filtering segments by external system identifiers, tags, or custom classifications.
//
// Query Strategy (Metadata-First):
//
// Unlike standard queries that start with PostgreSQL, this method:
//  1. Queries MongoDB first to find matching metadata documents
//  2. Extracts entity IDs from metadata results
//  3. Fetches full segment records from PostgreSQL by those IDs
//
// This approach is optimal when:
//   - Filtering by metadata fields not indexed in PostgreSQL
//   - Metadata filter is highly selective (few matches expected)
//   - Full segment data needed after metadata match
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Query Metadata from MongoDB
//	  - Apply metadata filter from query header
//	  - Return business error if no metadata matches
//	  - Build UUID slice and lookup map from results
//
//	Step 3: Fetch Segments from PostgreSQL
//	  - Query segments by extracted UUIDs
//	  - Scope to organization and ledger
//	  - Handle not-found scenarios
//
//	Step 4: Enrich Segments with Metadata
//	  - Assign metadata from lookup map to each segment
//	  - Match by entity ID
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope segments
//   - filter: Query parameters including metadata filter criteria
//
// Returns:
//   - []*mmodel.Segment: Segments with matching metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoSegmentsFound: No metadata matches or no segments for matched IDs
//   - Metadata query error: MongoDB unavailable or query failure
//   - Database error: PostgreSQL connection or query failure
//
// Note: Unlike paginated queries, this method returns all matching segments.
// Use with caution for large result sets.
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

		return nil, err
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

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get segments on repo by query params", err)

		return nil, err
	}

	for i := range segments {
		if data, ok := metadataMap[segments[i].ID]; ok {
			segments[i].Metadata = data
		}
	}

	return segments, nil
}
