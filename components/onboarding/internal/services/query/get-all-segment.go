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

// GetAllSegments retrieves all segments for a ledger with metadata enrichment.
//
// Segments organize accounts into logical groups within a ledger. This method
// fetches all segments with their associated metadata, providing a complete
// view of the ledger's organizational structure.
//
// Domain Context:
//
// Segments serve multiple purposes:
//   - Group accounts by business unit (e.g., "Sales", "Operations")
//   - Organize accounts by function (e.g., "Revenue", "Expenses")
//   - Enable hierarchical chart of accounts structures
//   - Support reporting and aggregation boundaries
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Fetch Segments from PostgreSQL
//	  - Query all segments for the organization/ledger
//	  - Apply offset-based pagination
//	  - Handle not-found with business error
//
//	Step 3: Collect Segment IDs
//	  - Build slice for bulk metadata lookup
//	  - Return empty slice early if no segments found
//
//	Step 4: Fetch Metadata from MongoDB
//	  - Bulk query metadata by segment IDs
//	  - Build lookup map indexed by entity ID
//
//	Step 5: Enrich Segments with Metadata
//	  - Assign metadata from lookup map
//	  - Segments without metadata retain nil
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope segments
//   - filter: Query parameters with offset pagination
//
// Returns:
//   - []*mmodel.Segment: Segments with metadata, empty slice if none found
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoSegmentsFound: No segments exist for ledger
//   - Metadata error: MongoDB query failure
//   - Database error: PostgreSQL connection or query failure
//
// Pagination:
//
// This method uses offset-based pagination (Limit/Page) rather than cursor-based.
// For large segment lists, consider using cursor pagination for better performance.
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
