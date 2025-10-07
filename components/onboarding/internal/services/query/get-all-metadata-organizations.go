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

// GetAllMetadataOrganizations retrieves organizations filtered by metadata criteria.
//
// This is a metadata-first query that:
// 1. Queries MongoDB for organizations matching metadata filters
// 2. Extracts entity IDs from metadata results
// 3. Fetches corresponding organizations from PostgreSQL
// 4. Merges metadata into organization objects
//
// Use Case: Searching organizations by custom metadata fields (e.g., industry, region)
//
// Query Flow: MongoDB → PostgreSQL (filter by metadata first, then fetch entities)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - filter: Query parameters including metadata filters (e.g., metadata.industry=Finance)
//
// Returns:
//   - []*mmodel.Organization: Array of organizations matching metadata criteria
//   - error: Business error if query fails
//
// Example:
//
//	filter := http.QueryHeader{
//	    Metadata: &bson.M{"industry": "Financial Services"},
//	}
//	orgs, err := useCase.GetAllMetadataOrganizations(ctx, filter)
//
// OpenTelemetry: Creates span "query.get_all_metadata_organizations"
func (uc *UseCase) GetAllMetadataOrganizations(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_organizations")
	defer span.End()

	logger.Infof("Retrieving organizations")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	organizations, err := uc.OrganizationRepo.ListByIDs(ctx, uuids)
	if err != nil {
		logger.Errorf("Error getting organizations on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get organizations on repo", err)

			logger.Warn("No organizations found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get organizations on repo", err)

		return nil, err
	}

	for i := range organizations {
		if data, ok := metadataMap[organizations[i].ID]; ok {
			organizations[i].Metadata = data
		}
	}

	return organizations, nil
}
