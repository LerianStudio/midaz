package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"github.com/google/uuid"
)

// GetOrganizationByID fetch a new organization from the repository
func (uc *UseCase) GetOrganizationByID(ctx context.Context, id string) (*o.Organization, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving organization for id: %s", id)

	organization, err := uc.OrganizationRepo.Find(ctx, uuid.MustParse(id))
	if err != nil {
		logger.Errorf("Error getting organization on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Organization{}).Name(),
				Message:    fmt.Sprintf("Organization with id %s was not found", id),
				Code:       "ORGANIZATION_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if organization != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(o.Organization{}).Name(), id)
		if err != nil {
			logger.Errorf("Error get metadata on mongodb organization: %v", err)
			return nil, err
		}

		if metadata != nil {
			organization.Metadata = metadata.Data
		}
	}

	return organization, nil
}
