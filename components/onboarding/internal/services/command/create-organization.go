// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for creating a new organization.
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

// CreateOrganization creates a new organization in the repository.
//
// This use case is responsible for:
// 1. Validating the country code in the address against the ISO 3166-1 alpha-2 standard.
// 2. Setting a default status of "ACTIVE" if none is provided.
// 3. Persisting the organization in the PostgreSQL database.
// 4. Storing any associated metadata in MongoDB.
// 5. Returning the newly created organization, including its metadata.
//
// Business Rules:
//   - The country code in the address must be a valid ISO 3166-1 alpha-2 code.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - coi: The input data for creating the organization.
//
// Returns:
//   - *mmodel.Organization: The created organization, complete with its metadata.
//   - error: An error if the creation fails due to a business rule violation or a database error.
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
