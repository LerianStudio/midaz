package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"reflect"
)

// GetOrganizationByID fetch a new organization from the repository
func (uc *UseCase) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_organization_by_id")
	defer span.End()

	logger.Infof("Retrieving organization for id: %s", id.String())

	organization, err := uc.OrganizationRepo.Find(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get organization on repo by id", err)

		logger.Errorf("Error getting organization on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return nil, err
	}

	if organization != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), id.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb organization", err)

			logger.Errorf("Error get metadata on mongodb organization: %v", err)

			return nil, err
		}

		if metadata != nil {
			organization.Metadata = metadata.Data
		}
	}

	return organization, nil
}
