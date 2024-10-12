package query

import (
	"context"
	"errors"
	c "github.com/LerianStudio/midaz/common/constant"
	"reflect"

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
		return nil, c.ValidateBusinessError(c.NoOrganizationsFoundBusinessError, reflect.TypeOf(o.Organization{}).Name())
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
			return nil, c.ValidateBusinessError(c.NoOrganizationsFoundBusinessError, reflect.TypeOf(o.Organization{}).Name())
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
