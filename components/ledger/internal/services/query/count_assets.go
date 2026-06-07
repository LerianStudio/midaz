// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
)

// CountAssets returns the total count of assets for a specific ledger in an organization.
func (uc *UseCase) CountAssets(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_assets")
	defer span.End()

	count, err := uc.AssetRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAssetsFound, constant.EntityAsset)

			logger.Log(ctx, libLog.LevelWarn, "No assets found for organization")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count assets on repo", err)

			return 0, err
		}

		logger.Log(ctx, libLog.LevelError, "Error counting assets on repo", libLog.Err(err))

		libOpentelemetry.HandleSpanError(span, "Failed to count assets on repo", err)

		return 0, err
	}

	return count, nil
}
