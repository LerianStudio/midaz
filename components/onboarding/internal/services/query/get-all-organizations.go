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
)

// GetAllOrganizations retrieves a paginated list of organizations with metadata.
//
// This method implements the list organizations query use case, which:
// 1. Fetches organizations from PostgreSQL with pagination
// 2. Fetches metadata for all organizations from MongoDB
// 3. Merges metadata into organization objects
// 4. Returns enriched organizations
//
// Query Features:
//   - Pagination: Supports limit and page parameters
//   - Sorting: Supports sort order (asc/desc)
//   - Date filtering: Supports start_date and end_date
//   - Metadata enrichment: Automatically fetches and merges metadata
//
// Behavior:
//   - Returns empty array if no organizations found (not an error)
//   - Metadata is optional (organizations without metadata are still returned)
//   - Soft-deleted organizations are excluded (WHERE deleted_at IS NULL)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - filter: Query parameters including pagination, sorting, and date range
//
// Returns:
//   - []*mmodel.Organization: Array of organizations with metadata
//   - error: Business error if query fails
//
// Possible Errors:
//   - ErrNoOrganizationsFound: No organizations match the query (only on database error)
//   - Database errors: Connection failures
//
// Example:
//
//	filter := http.QueryHeader{
//	    Limit:     50,
//	    Page:      1,
//	    SortOrder: "desc",
//	}
//	organizations, err := useCase.GetAllOrganizations(ctx, filter)
//	if err != nil {
//	    return nil, err
//	}
//	// Returns organizations with metadata merged
//
// OpenTelemetry:
//   - Creates span "query.get_all_organizations"
//   - Records errors as span events
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
