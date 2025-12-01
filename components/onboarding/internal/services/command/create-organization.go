// Package command provides CQRS command handlers for the onboarding component.
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

// CreateOrganization creates a new organization and persists it to the repository.
//
// Organizations are the top-level tenant entities in the Midaz ledger system.
// They represent a company, business unit, or logical grouping that owns ledgers,
// accounts, and other financial resources.
//
// # Organization Hierarchy
//
// Organizations support a parent-child hierarchy:
//   - ParentOrganizationID: Optional reference to a parent organization
//   - Enables multi-level organizational structures
//   - Parent must exist before creating child organizations
//
// # Status Management
//
// If no status is provided, organizations default to "ACTIVE".
// Supported statuses typically include:
//   - ACTIVE: Organization is operational
//   - INACTIVE: Organization is disabled
//   - SUSPENDED: Organization is temporarily disabled
//
// # Address Validation
//
// Organization addresses require valid ISO 3166-1 alpha-2 country codes.
// Invalid country codes result in ErrInvalidCountryCode.
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.create_organization"
//  3. Set default status to "ACTIVE" if not provided
//  4. Normalize nil ParentOrganizationID (handle empty strings)
//  5. Validate country code in address (ISO 3166-1 alpha-2)
//  6. Build organization model with timestamps
//  7. Persist organization to PostgreSQL via repository
//  8. Create associated metadata in MongoDB (if provided)
//  9. Return created organization with metadata
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - coi: CreateOrganizationInput containing organization details
//
// # Input Fields
//
//   - ParentOrganizationID: Optional parent organization reference
//   - LegalName: Official registered name of the organization
//   - DoingBusinessAs: Optional trade name (DBA)
//   - LegalDocument: Business registration number (CNPJ, EIN, etc.)
//   - Address: Physical address with country validation
//   - Status: Organization status (defaults to ACTIVE)
//   - Metadata: Optional key-value pairs for additional data
//
// # Returns
//
//   - *mmodel.Organization: The created organization with generated ID
//   - error: If validation fails or database operations fail
//
// # Error Scenarios
//
//   - ErrInvalidCountryCode: Invalid country code in address
//   - Database connection failure during Create
//   - Metadata creation failure (MongoDB)
//
// # Observability
//
// Creates parent span "command.create_organization" and child span
// for address validation. Error events recorded on failures.
//
// # Example
//
//	input := &mmodel.CreateOrganizationInput{
//	    LegalName:     "Acme Corporation",
//	    DoingBusinessAs: "Acme",
//	    LegalDocument: "12345678000199",
//	    Address: mmodel.Address{
//	        Country: "BR",
//	        State:   "SP",
//	        City:    "Sao Paulo",
//	    },
//	}
//	org, err := uc.CreateOrganization(ctx, input)
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_organization")
	defer span.End()

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
		err := pkg.ValidateBusinessError(constant.ErrInvalidCountryCode, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanAddressValidation, "Failed to validate country address", err)

		return nil, err
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

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create organization on repository", err)

		logger.Errorf("Error creating organization: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create organization metadata", err)

		logger.Errorf("Error creating organization metadata: %v", err)

		return nil, err
	}

	org.Metadata = metadata

	return org, nil
}
