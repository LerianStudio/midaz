// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// GetAllAssetRatesByAssetCode returns all asset rates by asset codes.
func (uc *UseCase) GetAllAssetRatesByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, filter http.QueryHeader) ([]*assetrate.AssetRate, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_asset_codes")
	defer span.End()

	if err := utils.ValidateCode(fromAssetCode); err != nil {
		err := pkg.ValidateBusinessError(err, constant.EntityAssetRate)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate 'from' asset code", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	for _, toAssetCode := range filter.ToAssetCodes {
		if err := utils.ValidateCode(toAssetCode); err != nil {
			err := pkg.ValidateBusinessError(err, constant.EntityAssetRate)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate 'to' asset codes", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	assetRates, cur, err := uc.AssetRateRepo.FindAllByAssetCodes(ctx, organizationID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination())
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get asset rate by asset codes on repository", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if assetRates != nil {
		metadata, err := uc.TransactionMetadataRepo.FindList(ctx, constant.EntityAssetRate, filter)
		if err != nil {
			recordQueryError(ctx, span, logger, "Failed to get metadata on mongodb asset rate", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range assetRates {
			if data, ok := metadataMap[assetRates[i].ID]; ok {
				assetRates[i].Metadata = data
			}
		}
	}

	return assetRates, cur, nil
}
