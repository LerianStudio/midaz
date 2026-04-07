// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// CreateOrganization creates a new organization and persists it in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_organization")
	defer span.End()

	status := coi.Status
	if status.Code == "" {
		status.Code = "ACTIVE"
	}

	if libCommons.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	if !coi.Address.IsEmpty() {
		if err := utils.ValidateCountryAddress(coi.Address.Country); err != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidCountryCode, constant.EntityOrganization)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate country address", err)

			return nil, err
		}
	}

	now := time.Now()

	organization := &mmodel.Organization{
		ParentOrganizationID: coi.ParentOrganizationID,
		LegalName:            coi.LegalName,
		DoingBusinessAs:      coi.DoingBusinessAs,
		LegalDocument:        coi.LegalDocument,
		Address:              coi.Address,
		Status:               status,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create organization on repository", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create organization", libLog.Err(err))

		return nil, err
	}

	// NOTE: The organization is already persisted at this point. If metadata creation
	// fails, the org exists in PostgreSQL without its metadata in MongoDB. This is a
	// known consistency gap that affects all entity creates. A proper fix requires
	// either a cross-store transaction or an async metadata creation with retries.
	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntityOrganization, org.ID, coi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create organization metadata", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create organization metadata, organization persisted without metadata",
			libLog.Err(err), libLog.String("organizationId", org.ID))

		return nil, err
	}

	org.Metadata = metadata

	return org, nil
}
