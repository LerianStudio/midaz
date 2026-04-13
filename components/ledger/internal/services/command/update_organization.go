// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// UpdateOrganizationByID applies a partial update to the organization identified by id.
// Only non-nil/non-zero fields in uoi are persisted; omitted fields remain unchanged.
// It validates that parentOrganizationID is not self-referencing and that the address
// country code is valid (when an address is provided). Metadata is updated separately
// via MongoDB after the organization record is persisted.
func (uc *UseCase) UpdateOrganizationByID(ctx context.Context, id uuid.UUID, uoi *mmodel.UpdateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_organization_by_id")
	defer span.End()

	if libCommons.IsNilOrEmpty(uoi.ParentOrganizationID) {
		uoi.ParentOrganizationID = nil
	}

	if uoi.ParentOrganizationID != nil && *uoi.ParentOrganizationID == id.String() {
		err := pkg.ValidateBusinessError(constant.ErrParentIDSameID, constant.EntityOrganization)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "ID cannot be used as the parent ID", err)
		logger.Log(ctx, libLog.LevelWarn, "Parent organization ID cannot be the same as the organization ID", libLog.Err(err))

		return nil, err
	}

	if !uoi.Address.IsEmpty() {
		if err := utils.ValidateCountryAddress(uoi.Address.Country); err != nil {
			err = pkg.ValidateBusinessError(err, constant.EntityOrganization)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate address country", err)

			return nil, err
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
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, constant.EntityOrganization)
			logger.Log(ctx, libLog.LevelWarn, "Organization not found", libLog.Err(err))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Organization not found", err)

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to update organization", libLog.Err(err))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update organization", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntityOrganization, id.String(), uoi.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to update organization metadata", libLog.Err(err))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update organization metadata", err)

		return nil, err
	}

	organizationUpdated.Metadata = metadataUpdated

	return organizationUpdated, nil
}
