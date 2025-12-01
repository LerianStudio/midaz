// Package command provides CQRS command handlers for the onboarding component.
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
// This function performs a partial update of an organization's mutable fields.
// It validates business rules, updates the organization record, and synchronizes
// associated metadata.
//
// # Updatable Fields
//
// The following fields can be updated:
//   - ParentOrganizationID: Change organization hierarchy
//   - LegalName: Update official name
//   - DoingBusinessAs: Update trade name
//   - Address: Update physical location
//   - Status: Change organization status
//   - Metadata: Update arbitrary key-value data
//
// Non-updatable fields (set at creation): ID, LegalDocument, CreatedAt
//
// # Self-Reference Validation
//
// An organization cannot be its own parent. Attempting to set
// ParentOrganizationID to the organization's own ID results in
// ErrParentIDSameID error.
//
// # Address Validation
//
// If address is updated, the country code is validated against ISO 3166-1 alpha-2.
// Only non-empty addresses trigger validation.
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.update_organization_by_id"
//  3. Normalize nil ParentOrganizationID (handle empty strings)
//  4. Validate ParentOrganizationID is not self-referential
//  5. If address provided, validate country code
//  6. Build partial organization update model
//  7. Update organization in PostgreSQL via repository
//  8. Handle not found error (ErrOrganizationIDNotFound)
//  9. Update associated metadata in MongoDB
//  10. Return updated organization with metadata
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - id: The UUID of the organization to update
//   - uoi: UpdateOrganizationInput containing fields to update
//
// # Returns
//
//   - *mmodel.Organization: The updated organization
//   - error: If validation fails, organization not found, or database fails
//
// # Error Scenarios
//
//   - ErrParentIDSameID: Attempted to set self as parent
//   - ErrInvalidCountryCode: Invalid country code in address
//   - ErrOrganizationIDNotFound: Organization with given ID not found
//   - Database connection failure
//   - Metadata update failure (MongoDB)
//
// # Observability
//
// Creates tracing span "command.update_organization_by_id" with error events.
// Logs operation progress, warnings for not found, errors for failures.
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
