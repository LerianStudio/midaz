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
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"reflect"
)

// GetAllOrganizations fetch all Organizations from the repository
func (uc *UseCase) GetAllOrganizations(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Organization, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_organizations")
	defer span.End()

	logger.Infof("Retrieving organizations")

	organizations, err := uc.OrganizationRepo.FindAll(ctx, filter.ToOffsetPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get organizations on repo", err)

		logger.Errorf("Error getting organizations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return nil, err
	}

	if organizations != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

			return nil, pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())
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
	}

	return organizations, nil
}
