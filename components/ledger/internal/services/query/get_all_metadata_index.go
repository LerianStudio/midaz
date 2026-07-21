// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	// GetAllMetadataIndexes returns all metadata indexes, optionally filtered by entity name
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAllMetadataIndexes(ctx context.Context, filter http.QueryHeader) ([]*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_indexes")
	defer span.End()

	metadataIndexesResponse := make([]*mmodel.MetadataIndex, 0)

	entitiesToQuery := mmodel.GetValidMetadataIndexEntities()

	if filter.EntityName != nil && *filter.EntityName != "" {
		if !mmodel.IsValidMetadataIndexEntity(*filter.EntityName) {
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidEntityName, "MetadataIndex")
		}

		entitiesToQuery = []string{*filter.EntityName}
	}

	for _, entityName := range entitiesToQuery {
		metadataIndexes, err := uc.TransactionMetadataRepo.FindAllIndexes(ctx, entityName)
		if err != nil {
			logger.Log(ctx, libLog.LevelError, "Error getting metadata indexes",
				libLog.String("entity", entityName), libLog.Err(err))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata indexes on repo", err)

			return nil, err
		}

		// Repository already filters for metadata.* indexes, strips prefix, and includes stats
		metadataIndexesResponse = append(metadataIndexesResponse, metadataIndexes...)
	}

	return metadataIndexesResponse, nil
}
