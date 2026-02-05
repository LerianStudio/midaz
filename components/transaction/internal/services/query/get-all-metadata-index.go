// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// GetAllMetadataIndexes returns all metadata indexes, optionally filtered by entity name
func (uc *UseCase) GetAllMetadataIndexes(ctx context.Context, filter http.QueryHeader) ([]*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_indexes")
	defer span.End()

	logger.Infof("Initializing the get all metadata indexes operation: %v", filter)

	metadataIndexesResponse := make([]*mmodel.MetadataIndex, 0)

	entitiesToQuery := mmodel.GetValidMetadataIndexEntities()

	if filter.EntityName != nil && *filter.EntityName != "" {
		if !mmodel.IsValidMetadataIndexEntity(*filter.EntityName) {
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidEntityName, "MetadataIndex")
		}

		entitiesToQuery = []string{*filter.EntityName}
	}

	for _, entityName := range entitiesToQuery {
		metadataIndexes, err := uc.MetadataRepo.FindAllIndexes(ctx, entityName)
		if err != nil {
			logger.Errorf("Error getting metadata indexes for entity %s: %v", entityName, err)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata indexes on repo", err)

			return nil, err
		}

		// Repository already filters for metadata.* indexes, strips prefix, and includes stats
		metadataIndexesResponse = append(metadataIndexesResponse, metadataIndexes...)
	}

	return metadataIndexesResponse, nil
}
