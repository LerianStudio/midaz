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

// GetAllPortfolio retrieves all portfolios for a ledger with metadata enrichment.
//
// Portfolios group related accounts for reporting, management, and access control.
// This method fetches all portfolios with their associated metadata, providing a
// complete view of account groupings within the ledger.
//
// Domain Context:
//
// Portfolios serve multiple purposes:
//   - Group accounts by customer (e.g., "Customer A Accounts")
//   - Organize accounts by product (e.g., "Savings Products")
//   - Define access control boundaries
//   - Enable aggregated reporting and analytics
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Fetch Portfolios from PostgreSQL
//	  - Query all portfolios for the organization/ledger
//	  - Apply offset-based pagination
//	  - Handle not-found with business error
//
//	Step 3: Collect Portfolio IDs
//	  - Build slice for bulk metadata lookup
//	  - Return empty slice early if no portfolios found
//
//	Step 4: Fetch Metadata from MongoDB
//	  - Bulk query metadata by portfolio IDs
//	  - Build lookup map indexed by entity ID
//
//	Step 5: Enrich Portfolios with Metadata
//	  - Assign metadata from lookup map
//	  - Portfolios without metadata retain nil
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope portfolios
//   - filter: Query parameters with offset pagination
//
// Returns:
//   - []*mmodel.Portfolio: Portfolios with metadata, empty slice if none found
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoPortfoliosFound: No portfolios exist for ledger
//   - Metadata error: MongoDB query failure
//   - Database error: PostgreSQL connection or query failure
//
// Pagination:
//
// This method uses offset-based pagination (Limit/Page) rather than cursor-based.
// For large portfolio lists, consider using cursor pagination for better performance.
func (uc *UseCase) GetAllPortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_portfolio")
	defer span.End()

	logger.Infof("Retrieving portfolios")

	portfolios, err := uc.PortfolioRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		logger.Errorf("Error getting portfolios on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get portfolios on repo", err)

			logger.Warn("No portfolios found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get portfolios on repo", err)

		return nil, err
	}

	if len(portfolios) == 0 {
		return portfolios, nil
	}

	portfolioIDs := make([]string, len(portfolios))
	for i, p := range portfolios {
		portfolioIDs[i] = p.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), portfolioIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range portfolios {
		if data, ok := metadataMap[portfolios[i].ID]; ok {
			portfolios[i].Metadata = data
		}
	}

	return portfolios, nil
}
