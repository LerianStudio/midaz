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
// This function performs a partial update of organization properties. Only the fields
// provided in the input will be updated; omitted fields remain unchanged.
//
// Validation rules enforced:
// - Parent organization ID cannot be the same as the organization ID (prevents circular references)
// - Country code must be valid ISO 3166-1 alpha-2 format if address is provided
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - id: The UUID of the organization to update
//   - uoi: The update input containing fields to modify
//
// Returns:
//   - *mmodel.Organization: The updated organization with refreshed metadata
//   - error: ErrParentIDSameID if circular reference, ErrOrganizationIDNotFound if not found
func (uc *UseCase) UpdateOrganizationByID(ctx context.Context, id uuid.UUID, uoi *mmodel.UpdateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_organization_by_id")
	defer span.End()

	logger.Infof("Trying to update organization: %v", uoi)

	// Step 1: Normalize parent organization reference to nil if empty
	if libCommons.IsNilOrEmpty(uoi.ParentOrganizationID) {
		uoi.ParentOrganizationID = nil
	}

	// Step 2: Prevent circular reference where organization is its own parent.
	// This is a critical validation to maintain organizational hierarchy integrity.
	if uoi.ParentOrganizationID != nil && *uoi.ParentOrganizationID == id.String() {
		err := pkg.ValidateBusinessError(constant.ErrParentIDSameID, "UpdateOrganizationByID")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "ID cannot be used as the parent ID.", err)

		logger.Errorf("Error ID cannot be used as the parent ID: %v", err)

		return nil, pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	// Step 3: Validate country code if address is being updated
	if !uoi.Address.IsEmpty() {
		if err := libCommons.ValidateCountryAddress(uoi.Address.Country); err != nil {
			err = pkg.ValidateBusinessError(err, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate address country", err)

			return nil, err
		}
	}

	// Step 4: Construct partial update entity with only provided fields
	organization := &mmodel.Organization{
		ParentOrganizationID: uoi.ParentOrganizationID,
		LegalName:            uoi.LegalName,
		DoingBusinessAs:      &uoi.DoingBusinessAs,
		Address:              uoi.Address,
		Status:               uoi.Status,
	}

	// Step 5: Persist the update to PostgreSQL
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

	// Step 6: Update metadata in MongoDB using JSON Merge Patch semantics
	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), id.String(), uoi.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	organizationUpdated.Metadata = metadataUpdated

	return organizationUpdated, nil
}
