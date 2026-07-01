// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"

	// GetAssetRateByExternalID gets data in the repository.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAssetRateByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*assetrate.AssetRate, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_external_id")
	defer span.End()

	assetRate, err := uc.AssetRateRepo.FindByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get asset rate by external id on repository", err)

		logger.Log(ctx, libLog.LevelError, "Error getting asset rate", libLog.Err(err))

		return nil, err
	}

	if assetRate != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, constant.EntityAssetRate, assetRate.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get metadata on mongodb asset rate", err)

			logger.Log(ctx, libLog.LevelError, "Error get metadata on mongodb asset rate", libLog.Err(err))

			return nil, err
		}

		if metadata != nil {
			assetRate.Metadata = metadata.Data
		}
	}

	return assetRate, nil
}
