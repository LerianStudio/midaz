package command

import (
	"context"
	"errors"
	"reflect"
	"time"

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
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.update_organization_by_id")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "organization", "update",
		attribute.String("organization_id", id.String()))

	logger.Infof("Trying to update organization: %v", uoi)

	if pkg.IsNilOrEmpty(uoi.ParentOrganizationID) {
		uoi.ParentOrganizationID = nil
	}

	if uoi.ParentOrganizationID != nil && *uoi.ParentOrganizationID == id.String() {
		err := pkg.ValidateBusinessError(constant.ErrParentIDSameID, "UpdateOrganizationByID")

		mopentelemetry.HandleSpanError(&span, "ID cannot be used as the parent ID.", err)

		logger.Errorf("Error ID cannot be used as the parent ID: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "organization", "validation_error",
			attribute.String("organization_id", id.String()),
			attribute.String("error_detail", "parent_id_same_as_id"))

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	if !uoi.Address.IsEmpty() {
		if err := pkg.ValidateCountryAddress(uoi.Address.Country); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to validate address country", err)

			// Record error
			uc.recordOnboardingError(ctx, "organization", "validation_error",
				attribute.String("organization_id", id.String()),
				attribute.String("error_detail", "invalid_country_address"))

			return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
		}
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
		mopentelemetry.HandleSpanError(&span, "Failed to update organization on repo by id", err)

		logger.Errorf("Error updating organization on repo by id: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "organization", "update_error",
			attribute.String("organization_id", id.String()),
			attribute.String("error_detail", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), id.String(), uoi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		// Record error
		uc.recordOnboardingError(ctx, "organization", "update_metadata_error",
			attribute.String("organization_id", id.String()),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	organizationUpdated.Metadata = metadataUpdated

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "organization", "update", "success",
		attribute.String("organization_id", id.String()),
		attribute.String("organization_name", organizationUpdated.LegalName))

	return organizationUpdated, nil
}
