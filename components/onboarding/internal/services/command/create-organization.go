package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// CreateOrganization creates a new organization and persists it in the repository.
//
// Organizations are the top-level entities in the Midaz hierarchy and represent
// legal entities such as companies, institutions, or business units. They serve
// as the root container for all ledgers, accounts, and transactions.
//
// The function performs the following steps:
// 1. Validates and normalizes the organization status (defaults to ACTIVE)
// 2. Validates the country code in the address against ISO 3166-1 alpha-2 standard
// 3. Persists the organization to PostgreSQL
// 4. Stores custom metadata in MongoDB if provided
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - coi: The organization creation input containing all required fields
//
// Returns:
//   - *mmodel.Organization: The created organization with generated ID and metadata
//   - error: Business validation or persistence errors
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_organization")
	defer span.End()

	logger.Infof("Trying to create organization: %v", coi)

	// Step 1: Determine organization status, defaulting to ACTIVE if not specified.
	// This ensures all organizations start with a valid operational status.
	var status mmodel.Status
	if coi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&coi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = coi.Status
	}

	status.Description = coi.Status.Description

	// Step 2: Normalize parent organization reference.
	// If empty, explicitly set to nil to maintain database integrity.
	if libCommons.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	// Step 3: Validate the country code follows ISO 3166-1 alpha-2 standard (two-letter codes).
	// This is critical for compliance and data consistency across international operations.
	ctx, spanAddressValidation := tracer.Start(ctx, "command.create_organization.validate_address")

	if err := libCommons.ValidateCountryAddress(coi.Address.Country); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrInvalidCountryCode, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanAddressValidation, "Failed to validate country address", err)

		return nil, err
	}

	spanAddressValidation.End()

	// Step 4: Construct the organization entity with validated fields and timestamps
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

	// Step 5: Persist the organization to PostgreSQL
	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create organization on repository", err)

		logger.Errorf("Error creating organization: %v", err)

		return nil, err
	}

	// Step 6: Store custom metadata in MongoDB if provided.
	// Metadata allows flexible extension of organization attributes without schema changes.
	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create organization metadata", err)

		logger.Errorf("Error creating organization metadata: %v", err)

		return nil, err
	}

	org.Metadata = metadata

	return org, nil
}
