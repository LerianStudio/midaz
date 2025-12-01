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

// GetAllLedgers retrieves all ledgers for an organization with pagination and metadata.
//
// This query fetches ledgers from PostgreSQL scoped to a specific organization
// and enriches each with its associated metadata from MongoDB. It supports
// offset-based pagination via the filter parameter.
//
// Multi-Tenancy:
//
// The organizationID parameter enforces tenant isolation at the query level.
// Only ledgers belonging to the specified organization are returned, preventing
// cross-tenant data access.
//
// Query Process:
//
//	Step 1: Context Extraction
//	  - Extract logger and tracer from context
//	  - Start tracing span "query.get_all_ledgers"
//
//	Step 2: PostgreSQL Query
//	  - Call LedgerRepo.FindAll with organization scope and pagination
//	  - Handle ErrDatabaseItemNotFound as "no ledgers found"
//	  - Return early if result is empty (valid empty response)
//
//	Step 3: Metadata Enrichment
//	  - Collect all ledger IDs
//	  - Batch fetch metadata from MongoDB by entity IDs
//	  - Build ID-to-metadata map for efficient lookup
//
//	Step 4: Result Assembly
//	  - Attach metadata to each ledger
//	  - Return enriched ledger list
//
// Parameters:
//   - ctx: Context with observability (logger, tracer, metrics)
//   - organizationID: Organization UUID for tenant isolation
//   - filter: Query parameters including pagination (limit, offset)
//
// Returns:
//   - []*mmodel.Ledger: List of ledgers with metadata
//   - error: Business error (ErrNoLedgersFound) or infrastructure error
//
// Error Scenarios:
//   - ErrNoLedgersFound: No ledgers exist for this organization
//   - Database errors: PostgreSQL connection or query failures
//   - Metadata errors: MongoDB query failures (returns error, not partial result)
func (uc *UseCase) GetAllLedgers(ctx context.Context, organizationID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_ledgers")
	defer span.End()

	logger.Infof("Retrieving ledgers")

	ledgers, err := uc.LedgerRepo.FindAll(ctx, organizationID, filter.ToOffsetPagination())
	if err != nil {
		logger.Errorf("Error getting ledgers on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			logger.Warn("No ledgers found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get ledgers on repo", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get ledgers on repo", err)

		return nil, err
	}

	if len(ledgers) == 0 {
		return ledgers, nil
	}

	ledgerIDs := make([]string, len(ledgers))
	for i, l := range ledgers {
		ledgerIDs[i] = l.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), ledgerIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		return nil, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range ledgers {
		if data, ok := metadataMap[ledgers[i].ID]; ok {
			ledgers[i].Metadata = data
		}
	}

	return ledgers, nil
}
