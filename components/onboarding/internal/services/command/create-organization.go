package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// CreateOrganization creates a new organization persists data in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_organization")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	if err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.create_organization_input", coi); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert create organization input to JSON string", err)
	}

	logger.Infof("Trying to create organization: %v", coi)

	var status mmodel.Status
	if coi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&coi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = coi.Status
	}

	status.Description = coi.Status.Description

	if libCommons.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	ctx, spanAddressValidation := tracer.Start(ctx, "command.create_organization.validate_address")

	if err := libCommons.ValidateCountryAddress(coi.Address.Country); err != nil {
		libOpentelemetry.HandleSpanError(&spanAddressValidation, "Failed to validate country address", err)

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidCountryCode, reflect.TypeOf(mmodel.Organization{}).Name())
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

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.repository_input", organization)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert organization repository input to JSON string", err)
	}

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create organization on repository", err)

		logger.Errorf("Error creating organization: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create organization metadata", err)

		logger.Errorf("Error creating organization metadata: %v", err)

		return nil, err
	}

	org.Metadata = metadata

	return org, nil
}
