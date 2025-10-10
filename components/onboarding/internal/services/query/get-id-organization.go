// Package query implements read operations (queries) for the onboarding service.
// This file contains the query for retrieving an organization by its ID.
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

// GetOrganizationByID retrieves a single organization by its ID, enriched with metadata.
//
// This use case fetches an organization from the PostgreSQL database and its
// corresponding metadata from MongoDB, then merges them into a single response.
// Soft-deleted organizations are excluded from the result.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - id: The UUID of the organization to retrieve.
//
// Returns:
//   - *mmodel.Organization: The organization with its metadata, or nil if not found.
//   - error: An error if the organization is not found or if the query fails.
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
			// FIXME: This error handling is incorrect. It returns an ErrOrganizationIDNotFound,
			// but the error is related to fetching metadata, not the organization itself.
			// The function should either return the organization without metadata or a
			// more appropriate metadata-specific error.
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
