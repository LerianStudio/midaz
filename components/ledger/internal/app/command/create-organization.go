package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
)

// CreateOrganization creates a new organization persists data in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *o.CreateOrganizationInput) (*o.Organization, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create organization: %v", coi)

	var status o.Status
	if coi.Status.IsEmpty() || common.IsNilOrEmpty(&coi.Status.Code) {
		status = o.Status{
			Code: "ACTIVE",
		}
	} else {
		status = coi.Status
	}

	status.Description = coi.Status.Description

	if common.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	if err := common.ValidateCountryAddress(coi.Address.Country); err != nil {
		return nil, common.ValidateBusinessError(err, reflect.TypeOf(o.Organization{}).Name())
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

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(o.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		logger.Errorf("Error creating organization metadata: %v", err)
		return nil, err
	}

	org.Metadata = metadata

	return org, nil
}
