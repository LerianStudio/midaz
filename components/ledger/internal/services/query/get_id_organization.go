// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"

	// GetOrganizationByID fetch a new organization from the repository
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_organization_by_id")
	defer span.End()

	organization, err := uc.OrganizationRepo.Find(ctx, id)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting organization on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, constant.EntityOrganization)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get organization on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, "No organization found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get organization on repo by id", err)

		return nil, err
	}

	if organization != nil {
		metadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, constant.EntityOrganization, id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, constant.EntityOrganization)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb organization", err)

			logger.Log(ctx, libLog.LevelWarn, "No metadata found")

			return nil, err
		}

		if metadata != nil {
			organization.Metadata = metadata.Data
		}
	}

	return organization, nil
}
