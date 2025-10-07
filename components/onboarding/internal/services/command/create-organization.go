// Package command implements write operations (commands) for the onboarding service.
// This file contains the CreateOrganization command implementation.
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
// This method implements the create organization use case, which:
// 1. Validates the country code in the address (ISO 3166-1 alpha-2)
// 2. Sets default status to ACTIVE if not provided
// 3. Creates the organization in PostgreSQL
// 4. Creates associated metadata in MongoDB
// 5. Returns the complete organization with metadata
//
// Business Rules:
//   - Country code must be valid ISO 3166-1 alpha-2 format (2 letters)
//   - Status defaults to ACTIVE if not provided or empty
//   - Parent organization ID is optional (for hierarchical organizations)
//   - Legal name and legal document are required (validated at HTTP layer)
//
// Data Storage:
//   - Primary data: PostgreSQL (organizations table)
//   - Metadata: MongoDB (flexible key-value storage)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - coi: Create organization input with all required and optional fields
//
// Returns:
//   - *mmodel.Organization: Created organization with metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrInvalidCountryCode: Country code is not valid ISO 3166-1 alpha-2
//   - ErrParentOrganizationIDNotFound: Parent organization does not exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.CreateOrganizationInput{
//	    LegalName:     "Acme Corp",
//	    LegalDocument: "12345678901234",
//	    Address: mmodel.Address{
//	        Country: "US",
//	        // ... other fields
//	    },
//	}
//	org, err := useCase.CreateOrganization(ctx, input)
//	if err != nil {
//	    return nil, err
//	}
//
// OpenTelemetry:
//   - Creates span "command.create_organization"
//   - Creates sub-span "command.create_organization.validate_address"
//   - Records errors as span events
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
