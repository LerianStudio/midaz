package command

import (
	"context"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
)

// CreateOrganization creates a new organization persists data in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *o.CreateOrganizationInput) (*o.Organization, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_organization")
	defer span.End()

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

	ctx, spanAddressValidation := tracer.Start(ctx, "command.create_organization.validate_address")

	if err := common.ValidateCountryAddress(coi.Address.Country); err != nil {
		mopentelemetry.HandleSpanError(&spanAddressValidation, "Failed to validate country address", err)

		return nil, common.ValidateBusinessError(err, reflect.TypeOf(o.Organization{}).Name())
	}

	spanAddressValidation.End()

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

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(o.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create organization metadata", err)

		logger.Errorf("Error creating organization metadata: %v", err)

		return nil, err
	}

	org.Metadata = metadata

	return org, nil
}
