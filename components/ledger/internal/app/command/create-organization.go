package command

import (
	"context"
	c "github.com/LerianStudio/midaz/common/constant"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
)

// CreateOrganization creates a new organization persists data in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *o.CreateOrganizationInput) (*o.Organization, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create organization: %v", coi)

	var status o.Status
	if coi.Status.IsEmpty() {
		status = o.Status{
			Code: "ACTIVE",
		}
	} else {
		status = coi.Status
	}

	if common.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	if err := common.ValidateCountryAddress(coi.Address.Country); err != nil {
		return nil, c.ValidateBusinessError(err, reflect.TypeOf(o.Organization{}).Name())
	}

	organization := &o.Organization{
		ParentOrganizationID: coi.ParentOrganizationID,
		LegalName:            coi.LegalName,
		DoingBusinessAs:      coi.DoingBusinessAs,
		LegalDocument:        coi.LegalDocument,
		Address:              coi.Address,
		Status:               status,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		logger.Errorf("Error creating organization: %v", err)
		return nil, err
	}

	if coi.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, coi.Metadata); err != nil {
			return nil, err
		}

		meta := m.Metadata{
			EntityID:   org.ID,
			EntityName: reflect.TypeOf(o.Organization{}).Name(),
			Data:       coi.Metadata,
			CreatedAt:  organization.CreatedAt,
			UpdatedAt:  organization.UpdatedAt,
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(o.Organization{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating organization metadata: %v", err)
			return nil, err
		}

		org.Metadata = coi.Metadata
	}

	return org, nil
}
