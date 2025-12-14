package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// UpdateOrganizationByID update an organization from the repository.
func (uc *UseCase) UpdateOrganizationByID(ctx context.Context, id uuid.UUID, uoi *mmodel.UpdateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_organization_by_id")
	defer span.End()

	logger.Infof("Trying to update organization: %v", uoi)

	if libCommons.IsNilOrEmpty(uoi.ParentOrganizationID) {
		uoi.ParentOrganizationID = nil
	}

	if uoi.ParentOrganizationID != nil && *uoi.ParentOrganizationID == id.String() {
		err := pkg.ValidateBusinessError(constant.ErrParentIDSameID, "UpdateOrganizationByID")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "ID cannot be used as the parent ID.", err)

		logger.Errorf("Error ID cannot be used as the parent ID: %v", err)

		return nil, fmt.Errorf("validation failed: %w", pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name()))
	}

	if !uoi.Address.IsEmpty() {
		if err := utils.ValidateCountryAddress(uoi.Address.Country); err != nil {
			err = pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate address country", err)

			return nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	organization := &mmodel.Organization{
		ParentOrganizationID: uoi.ParentOrganizationID,
		LegalName:            uoi.LegalName,
		DoingBusinessAs:      uoi.DoingBusinessAs,
		Address:              uoi.Address,
		Status:               uoi.Status,
	}

	organizationUpdated, err := uc.OrganizationRepo.Update(ctx, id, organization)
	if err != nil {
		logger.Errorf("Error updating organization on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			logger.Warnf("Organization ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update organization on repo by id", err)

			return nil, fmt.Errorf("validation failed: %w", err)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update organization on repo by id", err)

		return nil, fmt.Errorf("validation failed: %w", err)
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), id.String(), uoi.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, fmt.Errorf("operation failed: %w", err)
	}

	organizationUpdated.Metadata = metadataUpdated

	return organizationUpdated, nil
}
