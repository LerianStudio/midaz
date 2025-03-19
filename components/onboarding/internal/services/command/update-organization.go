package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// UpdateOrganizationByID update an organization from the repository.
func (uc *UseCase) UpdateOrganizationByID(ctx context.Context, id uuid.UUID, uoi *mmodel.UpdateOrganizationInput) (*mmodel.Organization, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	op := uc.Telemetry.NewOrganizationOperation("update", id.String())

	op.WithAttributes(
		attribute.String("organization_id", id.String()),
	)

	if uoi.LegalName != "" {
		op.WithAttribute("organization_name", uoi.LegalName)
	}

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Trying to update organization: %v", uoi)

	if pkg.IsNilOrEmpty(uoi.ParentOrganizationID) {
		uoi.ParentOrganizationID = nil
	} else {
		op.WithAttribute("parent_organization_id", *uoi.ParentOrganizationID)
	}

	if uoi.ParentOrganizationID != nil && *uoi.ParentOrganizationID == id.String() {
		err := pkg.ValidateBusinessError(constant.ErrParentIDSameID, "UpdateOrganizationByID")
		mopentelemetry.HandleSpanError(&op.span, "ID cannot be used as the parent ID.", err)
		logger.Errorf("Error ID cannot be used as the parent ID: %v", err)
		op.WithAttribute("error_detail", "parent_id_same_as_id")
		op.RecordError(ctx, "validation_error", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	if !uoi.Address.IsEmpty() {
		addressValidationOp := uc.Telemetry.NewEntityOperation("address", "validate", id.String())
		addressValidationOp.WithAttribute("country", uoi.Address.Country)
		addressValidationCtx := addressValidationOp.StartTrace(ctx)

		if err := pkg.ValidateCountryAddress(uoi.Address.Country); err != nil {
			mopentelemetry.HandleSpanError(&addressValidationOp.span, "Failed to validate address country", err)
			addressValidationOp.WithAttribute("error_detail", "invalid_country_address")
			addressValidationOp.RecordError(addressValidationCtx, "validation_error", err)
			addressValidationOp.End(addressValidationCtx, "error")

			op.WithAttribute("error_detail", "invalid_country_address")
			op.RecordError(ctx, "validation_error", err)

			return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		addressValidationOp.End(addressValidationCtx, "success")
	}

	organization := &mmodel.Organization{
		ParentOrganizationID: uoi.ParentOrganizationID,
		LegalName:            uoi.LegalName,
		DoingBusinessAs:      &uoi.DoingBusinessAs,
		Address:              uoi.Address,
		Status:               uoi.Status,
	}

	organizationUpdated, err := uc.OrganizationRepo.Update(ctx, id, organization)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update organization on repo by id", err)
		logger.Errorf("Error updating organization on repo by id: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), id.String(), uoi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update metadata on repo by id", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_metadata_error", err)

		return nil, err
	}

	organizationUpdated.Metadata = metadataUpdated

	op.End(ctx, "success")

	return organizationUpdated, nil
}
