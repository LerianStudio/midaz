// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"

	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// CountAssets returns the total count of assets for a specific ledger in an organization
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) CountAssets(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_assets")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Counting assets for organization: %s, ledger: %s", organizationID, ledgerID))

	count, err := uc.AssetRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error counting assets on repo: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("No assets found for organization: %s", organizationID.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count assets on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count assets on repo", err)

		return 0, err
	}

	return count, nil
}
