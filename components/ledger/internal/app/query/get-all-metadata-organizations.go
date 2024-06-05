package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"github.com/google/uuid"
)

// GetAllMetadataOrganizations fetch all Organizations from the repository
func (uc *UseCase) GetAllMetadataOrganizations(ctx context.Context, filter commonHTTP.QueryHeader) ([]*o.Organization, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving organizations")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(o.Organization{}).Name(), filter)
	if err != nil || metadata == nil {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(o.Organization{}).Name(),
			Message:    "Organizations by metadata was not found",
			Code:       "ORGANIZATION_NOT_FOUND",
			Err:        err,
		}
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

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Organization{}).Name(),
				Message:    "Organizations by metadata was not found",
				Code:       "ORGANIZATION_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	for i := range organizations {
		if data, ok := metadataMap[organizations[i].ID]; ok {
			organizations[i].Metadata = data
		}
	}

	return organizations, nil
}
