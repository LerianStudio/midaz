package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
)

// CreateOrganization creates a new organization persists data in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_organization")
	defer span.End()

	logger.Infof("Trying to create organization: %v", coi)

	var status mmodel.Status
	if coi.Status.IsEmpty() || common.IsNilOrEmpty(&coi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = coi.Status
	}

	status.Description = coi.Status.Description

	if common.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	ctx, spanAddressValidation := tracer.Start(ctx, "command.create_organization.validate_address")

	if err := common.ValidateCountryAddress(coi.Address.Country); err != nil {
		mopentelemetry.HandleSpanError(&spanAddressValidation, "Failed to validate country address", err)

		return nil, common.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	spanAddressValidation.End()

	organization := &mmodel.Organization{
		ParentOrganizationID: coi.ParentOrganizationID,
		LegalName:            coi.LegalName,
		DoingBusinessAs:      coi.DoingBusinessAs,
		LegalDocument:        coi.LegalDocument,
		Address:              coi.Address,
		Status:               status,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "organization_repository_input", organization)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert organization repository input to JSON string", err)

		return nil, err
	}

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create organization on repository", err)

		logger.Errorf("Error creating organization: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create organization metadata", err)

		logger.Errorf("Error creating organization metadata: %v", err)

		return nil, err
	}

	org.Metadata = metadata

	return org, nil
}
