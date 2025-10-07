// Package command implements write operations (commands) for the onboarding service.
// This file contains the UpdateOrganizationByID command implementation.
package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateOrganizationByID updates an existing organization in the repository.
//
// This method implements the update organization use case, which:
// 1. Validates that parent organization ID is not the same as organization ID
// 2. Validates country code if address is provided (ISO 3166-1 alpha-2)
// 3. Updates the organization in PostgreSQL
// 4. Updates associated metadata in MongoDB
// 5. Returns the updated organization with metadata
//
// Business Rules:
//   - An organization cannot be its own parent
//   - Country code must be valid ISO 3166-1 alpha-2 format if provided
//   - Only provided fields are updated (partial updates supported)
//   - Parent organization must exist if provided
//   - Legal document cannot be updated (immutable field, enforced at HTTP layer)
//
// Update Behavior:
//   - Empty strings in input are treated as "clear the field"
//   - Nil pointers in input mean "don't update this field"
//   - Empty status means "don't update status"
//   - Empty address means "don't update address"
//
// Data Storage:
//   - Primary data: PostgreSQL (organizations table)
//   - Metadata: MongoDB (metadata is replaced, not merged)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - id: UUID of the organization to update
//   - uoi: Update organization input with fields to update
//
// Returns:
//   - *mmodel.Organization: Updated organization with metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrOrganizationIDNotFound: Organization doesn't exist
//   - ErrParentIDSameID: Attempting to set organization as its own parent
//   - ErrInvalidCountryCode: Country code is not valid ISO 3166-1 alpha-2
//   - ErrParentOrganizationIDNotFound: Parent organization doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.UpdateOrganizationInput{
//	    LegalName: "Acme Corporation Ltd.",
//	    Status:    mmodel.Status{Code: "ACTIVE"},
//	}
//	org, err := useCase.UpdateOrganizationByID(ctx, orgID, input)
//
// OpenTelemetry:
//   - Creates span "command.update_organization_by_id"
//   - Records errors as span events
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

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	if !uoi.Address.IsEmpty() {
		if err := libCommons.ValidateCountryAddress(uoi.Address.Country); err != nil {
			err = pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate address country", err)

			return nil, err
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
		logger.Errorf("Error updating organization on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			logger.Warnf("Organization ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update organization on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update organization on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), id.String(), uoi.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	organizationUpdated.Metadata = metadataUpdated

	return organizationUpdated, nil
}
