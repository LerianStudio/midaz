// Package query provides CQRS query use cases for the onboarding bounded context.
//
// This package implements the read-side of CQRS pattern, providing query operations
// for retrieving entity data with full observability support:
//   - Organization queries (list, find by ID)
//   - Ledger queries (list by organization)
//   - Account queries (list, find by ID, including soft-deleted)
//   - Asset queries (list by ledger)
//   - Portfolio queries (list by ledger)
//
// Query Pattern:
//
// All queries follow a consistent pattern:
//  1. Extract observability context (logger, tracer)
//  2. Start tracing span for the operation
//  3. Retrieve primary data from PostgreSQL
//  4. Enrich with metadata from MongoDB
//  5. Return combined result with proper error handling
//
// Metadata Enrichment:
//
// Entities are stored in PostgreSQL (structured data) while flexible metadata
// lives in MongoDB. Queries automatically join both sources to return complete
// entity representations.
//
// Related Packages:
//   - command: Write-side use cases (create, update, delete)
//   - postgres adapters: PostgreSQL repository implementations
//   - mongodb adapters: MongoDB metadata repository
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
)

// GetAllOrganizations retrieves all organizations with pagination and metadata enrichment.
//
// This query fetches organizations from PostgreSQL and enriches each with its
// associated metadata from MongoDB. It supports offset-based pagination via
// the filter parameter.
//
// Query Process:
//
//	Step 1: Context Extraction
//	  - Extract logger and tracer from context
//	  - Start tracing span "query.get_all_organizations"
//
//	Step 2: PostgreSQL Query
//	  - Call OrganizationRepo.FindAll with pagination
//	  - Handle ErrDatabaseItemNotFound as "no organizations found"
//	  - Return early if result is empty (valid empty response)
//
//	Step 3: Metadata Enrichment
//	  - Collect all organization IDs
//	  - Batch fetch metadata from MongoDB by entity IDs
//	  - Build ID-to-metadata map for efficient lookup
//
//	Step 4: Result Assembly
//	  - Attach metadata to each organization
//	  - Return enriched organization list
//
// Parameters:
//   - ctx: Context with observability (logger, tracer, metrics)
//   - filter: Query parameters including pagination (limit, offset)
//
// Returns:
//   - []*mmodel.Organization: List of organizations with metadata
//   - error: Business error (ErrNoOrganizationsFound) or infrastructure error
//
// Error Scenarios:
//   - ErrNoOrganizationsFound: No organizations exist or match filter
//   - Database errors: PostgreSQL connection or query failures
//   - Metadata errors: MongoDB query failures (returns error, not partial result)
func (uc *UseCase) GetAllOrganizations(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_organizations")
	defer span.End()

	logger.Infof("Retrieving organizations")

	organizations, err := uc.OrganizationRepo.FindAll(ctx, filter.ToOffsetPagination())
	if err != nil {
		logger.Errorf("Error getting organizations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get organizations on repo", err)

			logger.Warn("No organizations found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get organizations on repo", err)

		return nil, err
	}

	if len(organizations) == 0 {
		return organizations, nil
	}

	organizationIDs := make([]string, len(organizations))
	for i, o := range organizations {
		organizationIDs[i] = o.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), organizationIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range organizations {
		if data, ok := metadataMap[organizations[i].ID]; ok {
			organizations[i].Metadata = data
		}
	}

	return organizations, nil
}
