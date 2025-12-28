package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// GetOrganizationByID fetch a new organization from the repository
func (uc *UseCase) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_organization_by_id")
	defer span.End()

	logger.Infof("Retrieving organization for id: %s", id.String())

	organization, err := uc.OrganizationRepo.Find(ctx, id)
	if err != nil {
		logger.Errorf("Error getting organization on repo by id: %v", err)

		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get organization on repo by id", err)

			logger.Warn("No organization found")

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get organization on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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
