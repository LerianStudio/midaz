// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataAssets fetches all assets from the repository.
func (uc *UseCase) GetAllMetadataAssets(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_assets")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Retrieving assets")

	metadata, err := uc.OnboardingMetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get metadata on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error getting metadata on repo")

		return nil, err
	}

	if metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No metadata found", err)

		logger.Log(ctx, libLog.LevelWarn, "No metadata found")

		return nil, err
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for idx, meta := range metadata {
		uuids[idx] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	assets, err := uc.AssetRepo.ListByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get assets on repo", err)

			logger.Log(ctx, libLog.LevelWarn, "No assets found")

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Error getting assets on repo")

		libOpentelemetry.HandleSpanError(span, "Failed to get assets on repo", err)

		return nil, err
	}

	for idx := range assets {
		if data, ok := metadataMap[assets[idx].ID]; ok {
			assets[idx].Metadata = data
		}
	}

	return assets, nil
}
