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
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllAssets fetches all assets from the repository.
func (uc *UseCase) GetAllAssets(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Asset, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_assets")
	defer span.End()

	assets, err := uc.AssetRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting assets on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, constant.EntityAsset)

			logger.Log(ctx, libLog.LevelWarn, "No assets found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get assets on repo", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get assets on repo", err)

		return nil, err
	}

	if len(assets) == 0 {
		return assets, nil
	}

	assetIDs := make([]string, len(assets))
	for i, a := range assets {
		assetIDs[i] = a.ID
	}

	metadata, err := uc.OnboardingMetadataRepo.FindByEntityIDs(ctx, constant.EntityAsset, assetIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, constant.EntityAsset)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on repo", err)

		return nil, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for idx := range assets {
		if data, ok := metadataMap[assets[idx].ID]; ok {
			assets[idx].Metadata = data
		}
	}

	return assets, nil
}
