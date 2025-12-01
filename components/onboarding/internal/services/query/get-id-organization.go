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

// GetOrganizationByID retrieves a single organization by its unique identifier.
//
// Organizations are the root entities in the multi-tenant hierarchy. This method
// fetches the complete organization configuration including associated metadata,
// useful for organization management and tenant configuration interfaces.
//
// Domain Context:
//
// An organization represents:
//   - A tenant in the multi-tenant system
//   - The top-level container for ledgers, accounts, and transactions
//   - Legal entity information (name, address, tax IDs)
//   - Custom configuration via metadata
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//	  - Log the organization ID being retrieved
//
//	Step 2: Fetch Organization from PostgreSQL
//	  - Query by organization ID
//	  - Handle not-found with business error
//	  - Handle other errors as infrastructure errors
//
//	Step 3: Fetch Metadata from MongoDB (if organization found)
//	  - Query metadata document by organization ID
//	  - Assign metadata to organization if present
//	  - Handle metadata errors as infrastructure errors
//
// Parameters:
//   - ctx: Request context with tracing information
//   - id: Organization UUID to retrieve
//
// Returns:
//   - *mmodel.Organization: Complete organization with metadata
//   - error: Business error (not found) or infrastructure error
//
// Error Scenarios:
//   - ErrOrganizationIDNotFound: Organization does not exist
//   - Database error: PostgreSQL connection or query failure
//   - Metadata error: MongoDB query failure
//
// Security Note:
//
// This method does not perform tenant isolation checks. The caller is responsible
// for ensuring the requesting user has permission to access the organization.
// In most cases, this is validated at the API gateway or middleware layer.
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
