package query

import (
	"context"
	"fmt"
	"strings"

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

			return nil, fmt.Errorf("failed to get metadata indexes for entity %s: %w", entityName, err)
		}

		for _, idx := range metadataIndexes {
			metadataKey := idx.MetadataKey

			if metadataKey == "" || metadataKey == "_id" {
				continue
			}

			if !strings.HasPrefix(metadataKey, "metadata.") {
				continue
			}

			metadataKey = strings.TrimPrefix(metadataKey, "metadata.")

			metadataIndexesResponse = append(metadataIndexesResponse, &mmodel.MetadataIndex{
				IndexName:   fmt.Sprintf("metadata.%s_1", metadataKey),
				EntityName:  entityName,
				MetadataKey: metadataKey,
				Unique:      idx.Unique,
				Sparse:      idx.Sparse,
				CreatedAt:   idx.CreatedAt,
			})
		}
	}

	return metadataIndexesResponse, nil
}
