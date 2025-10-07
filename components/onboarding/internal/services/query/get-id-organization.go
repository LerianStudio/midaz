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
	"github.com/google/uuid"
)

// GetOrganizationByID retrieves a single organization by ID with metadata.
//
// This method implements the get organization query use case, which:
// 1. Fetches the organization from PostgreSQL by ID
// 2. Fetches associated metadata from MongoDB
// 3. Merges metadata into the organization object
// 4. Returns the enriched organization
//
// Query Features:
//   - Retrieves single entity by UUID
//   - Automatically enriches with metadata
//   - Excludes soft-deleted organizations
//
// Behavior:
//   - Returns error if organization not found
//   - Metadata is optional (organization returned even if metadata fetch fails)
//   - Soft-deleted organizations are not returned
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - id: UUID of the organization to retrieve
//
// Returns:
//   - *mmodel.Organization: Organization with metadata
//   - error: Business error if not found or query fails
//
// Possible Errors:
//   - ErrOrganizationIDNotFound: Organization doesn't exist or is deleted
//   - Database errors: Connection failures
//
// Example:
//
//	organization, err := useCase.GetOrganizationByID(ctx, orgID)
//	if err != nil {
//	    return nil, err
//	}
//	// Returns organization with metadata
//
// OpenTelemetry:
//   - Creates span "query.get_organization_by_id"
//   - Records errors as span events
func (uc *UseCase) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_organization_by_id")
	defer span.End()

	logger.Infof("Retrieving organization for id: %s", id.String())

	organization, err := uc.OrganizationRepo.Find(ctx, id)
	if err != nil {
		logger.Errorf("Error getting organization on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get organization on repo by id", err)

			logger.Warn("No organization found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get organization on repo by id", err)

		return nil, err
	}

	if organization != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb organization", err)

			logger.Warn("No metadata found")

			return nil, err
		}

		if metadata != nil {
			organization.Metadata = metadata.Data
		}
	}

	return organization, nil
}
