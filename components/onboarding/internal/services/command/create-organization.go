package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"
)

// CreateOrganization creates a new organization persists data in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.create_organization")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "organization", "create",
		attribute.String("organization_name", coi.LegalName))

	logger.Infof("Trying to create organization: %v", coi)

	var status mmodel.Status
	if coi.Status.IsEmpty() || pkg.IsNilOrEmpty(&coi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = coi.Status
	}

	status.Description = coi.Status.Description

	if pkg.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	ctx, spanAddressValidation := tracer.Start(ctx, "command.create_organization.validate_address")

	if err := pkg.ValidateCountryAddress(coi.Address.Country); err != nil {
		mopentelemetry.HandleSpanError(&spanAddressValidation, "Failed to validate country address", err)

		// Record error
		uc.recordOnboardingError(ctx, "organization", "validation_error",
			attribute.String("organization_name", coi.LegalName),
			attribute.String("error_detail", "invalid_country_address"))

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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

		// Record error
		uc.recordOnboardingError(ctx, "organization", "span_attributes_error",
			attribute.String("organization_name", coi.LegalName),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create organization on repository", err)

		logger.Errorf("Error creating organization: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "organization", "creation_error",
			attribute.String("organization_name", coi.LegalName),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create organization metadata", err)

		logger.Errorf("Error creating organization metadata: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "organization", "metadata_error",
			attribute.String("organization_id", org.ID),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	org.Metadata = metadata

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "organization", "create", "success",
		attribute.String("organization_id", org.ID),
		attribute.String("organization_name", org.LegalName))

	return org, nil
}
