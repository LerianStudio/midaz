package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	"github.com/google/uuid"
)

// GetOrganizationByID fetch a new organization from the repository
func (uc *UseCase) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_organization_by_id")
	defer span.End()

	logger.Infof("Retrieving organization for id: %s", id.String())

	organization, err := uc.OrganizationRepo.Find(ctx, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get organization on repo by id", err)

		logger.Errorf("Error getting organization on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return nil, err
	}

	if organization != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), id.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb organization", err)

			logger.Errorf("Error get metadata on mongodb organization: %v", err)

			return nil, err
		}

		if metadata != nil {
			organization.Metadata = metadata.Data
		}
	}

	return organization, nil
}
