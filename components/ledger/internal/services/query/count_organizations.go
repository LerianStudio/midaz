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

	// CountOrganizations returns the total count of organizations
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) CountOrganizations(ctx context.Context) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_organizations")
	defer span.End()

	count, err := uc.OrganizationRepo.Count(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error counting organizations on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, constant.EntityOrganization)

			logger.Log(ctx, libLog.LevelWarn, "No organizations found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count organizations on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count organizations on repo", err)

		return 0, err
	}

	return count, nil
}
