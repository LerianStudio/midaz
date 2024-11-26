package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
)

// GetAllOrganizations fetch all Organizations from the repository
func (uc *UseCase) GetAllOrganizations(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Organization, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving organizations")

	organizations, err := uc.OrganizationRepo.FindAll(ctx, filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting organizations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return nil, err
	}

	if organizations != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), filter)
		if err != nil {
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
