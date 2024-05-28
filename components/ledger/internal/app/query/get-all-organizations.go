package query

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllOrganizations fetch all Organizations from the repository
func (uc *UseCase) GetAllOrganizations(ctx context.Context, limit int, token *string) (*o.Pagination, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving organizations")

	id := uuid.Nil
	if token != nil {
		id = uuid.MustParse(*token)
	}

	pagination, err := uc.OrganizationRepo.FindAll(ctx, limit, id)
	if err != nil {
		logger.Errorf("Error getting organizations on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Organization{}).Name(),
				Message:    "Organizations was not found",
				Code:       "ORGANIZATION_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	organizations := pagination.Organizations

	if organizations != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(o.Organization{}).Name(), bson.M{})
		if err != nil {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Organization{}).Name(),
				Message:    "Metadata was not found",
				Code:       "ORGANIZATION_NOT_FOUND",
				Err:        err,
			}
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

	pagination.Organizations = organizations

	return pagination, nil
}
