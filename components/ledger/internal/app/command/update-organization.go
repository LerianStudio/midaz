package command

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

// UpdateOrganizationByID update an organization from the repository.
func (uc *UseCase) UpdateOrganizationByID(ctx context.Context, id string, uoi *o.UpdateOrganizationInput) (*o.Organization, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update organization: %v", uoi)

	if common.IsNilOrEmpty(uoi.ParentOrganizationID) {
		uoi.ParentOrganizationID = nil
	}

	if !uoi.Address.IsEmpty() {
		if err := common.ValidateCountryAddress(uoi.Address.Country); err != nil {
			return nil, err
		}
	}

	organization := &o.Organization{
		ParentOrganizationID: uoi.ParentOrganizationID,
		LegalName:            uoi.LegalName,
		DoingBusinessAs:      uoi.DoingBusinessAs,
		Address:              uoi.Address,
		Status:               uoi.Status,
	}

	organizationUpdated, err := uc.OrganizationRepo.Update(ctx, uuid.MustParse(id), organization)
	if err != nil {
		logger.Errorf("Error updating organization on repo by id: %v", err)

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

	if len(uoi.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, uoi.Metadata); err != nil {
			return nil, err
		}

		if err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(o.Organization{}).Name(), id, uoi.Metadata); err != nil {
			return nil, err
		}

		organizationUpdated.Metadata = uoi.Metadata
	}

	return organizationUpdated, nil
}
