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

// GetAllOrganizations retrieves a paginated list of organizations with their metadata.
//
// This query handler fetches organizations from PostgreSQL and enriches them with
// custom metadata from MongoDB. It uses offset-based pagination controlled by the
// filter parameters (page, limit, sort order).
//
// The function performs a batch metadata fetch for efficiency, retrieving metadata
// for all organizations in a single MongoDB query and then mapping it back to each
// organization entity.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - filter: Pagination and filtering parameters (page, limit, sort_order, date range)
//
// Returns:
//   - []*mmodel.Organization: Paginated list of organizations with enriched metadata
//   - error: ErrNoOrganizationsFound if none exist, or repository errors
func (uc *UseCase) GetAllOrganizations(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_organizations")
	defer span.End()

	logger.Infof("Retrieving organizations")

	// Step 1: Fetch paginated organizations from PostgreSQL
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

	// Step 2: Extract organization IDs for batch metadata lookup
	organizationIDs := make([]string, len(organizations))
	for i, o := range organizations {
		organizationIDs[i] = o.ID
	}

	// Step 3: Batch fetch metadata from MongoDB for all organizations
	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), organizationIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
	}

	// Step 4: Build metadata map for efficient lookup by entity ID
	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	// Step 5: Enrich each organization with its metadata
	for i := range organizations {
		if data, ok := metadataMap[organizations[i].ID]; ok {
			organizations[i].Metadata = data
		}
	}

	return organizations, nil
}
