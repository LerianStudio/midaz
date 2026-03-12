// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"

	// CountOrganizations returns the total count of organizations
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) CountOrganizations(ctx context.Context) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_organizations")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Counting organizations")

	count, err := uc.OrganizationRepo.Count(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error counting organizations on repo: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())

			logger.Log(ctx, libLog.LevelWarn, "No organizations found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count organizations on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count organizations on repo", err)

		return 0, err
	}

	return count, nil
}
