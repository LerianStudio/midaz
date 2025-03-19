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

// CreateOrganization creates a new organization and persists data in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	organizationID := pkg.GenerateUUIDv7().String()
	op := uc.Telemetry.NewOrganizationOperation("create", organizationID)

	op.WithAttributes(
		attribute.String("organization_name", coi.LegalName),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

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
	} else {
		op.WithAttribute("parent_organization_id", *coi.ParentOrganizationID)
	}

	addressValidationOp := uc.Telemetry.NewEntityOperation("address", "validate", organizationID)
	addressValidationOp.WithAttribute("country", coi.Address.Country)
	addressValidationCtx := addressValidationOp.StartTrace(ctx)

	if err := pkg.ValidateCountryAddress(coi.Address.Country); err != nil {
		mopentelemetry.HandleSpanError(&addressValidationOp.span, "Failed to validate country address", err)
		addressValidationOp.WithAttribute("error_detail", "invalid_country_address")
		addressValidationOp.RecordError(addressValidationCtx, "validation_error", err)
		addressValidationOp.End(addressValidationCtx, "error")

		op.WithAttribute("error_detail", "invalid_country_address")
		op.RecordError(ctx, "validation_error", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	addressValidationOp.End(addressValidationCtx, "success")

	organization := &mmodel.Organization{
		ID:                   organizationID,
		ParentOrganizationID: coi.ParentOrganizationID,
		LegalName:            coi.LegalName,
		DoingBusinessAs:      coi.DoingBusinessAs,
		LegalDocument:        coi.LegalDocument,
		Address:              coi.Address,
		Status:               status,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	err := mopentelemetry.SetSpanAttributesFromStruct(&op.span, "organization_repository_input", organization)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to convert organization repository input to JSON string", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "span_attributes_error", err)

		return nil, err
	}

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create organization on repository", err)
		logger.Errorf("Error creating organization: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "creation_error", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create organization metadata", err)
		logger.Errorf("Error creating organization metadata: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "metadata_error", err)

		return nil, err
	}

	org.Metadata = metadata

	op.End(ctx, "success")

	return org, nil
}
