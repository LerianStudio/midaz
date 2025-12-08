package query

import (
	"context"
	"fmt"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataIndexes returns all metadata indexes, optionally filtered by entity name
func (uc *UseCase) GetAllMetadataIndexes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_indexes")
	defer span.End()

	logger.Infof("Initializing the get all metadata indexes operation: %v", filter)

	var metadataIndexesResponse []*mmodel.MetadataIndex

	entitiesToQuery := mmodel.GetValidMetadataIndexEntities()
	if filter.EntityName != nil && *filter.EntityName != "" {
		entitiesToQuery = []string{*filter.EntityName}
	}

	for _, entityName := range entitiesToQuery {
		metadataIndexes, err := uc.MetadataRepo.FindAllIndexes(ctx, entityName, filter)
		if err != nil {
			logger.Errorf("Error getting metadata indexes for entity %s: %v", entityName, err)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata indexes on repo", err)

			return nil, err
		}

		for _, idx := range metadataIndexes {
			if !strings.HasPrefix(idx.MetadataKey, "metadata.") && idx.MetadataKey != "" {
				metadataIndexesResponse = append(metadataIndexesResponse, &mmodel.MetadataIndex{
					IndexName:   fmt.Sprintf("metadata.%s_1", idx.MetadataKey),
					EntityName:  entityName,
					MetadataKey: idx.MetadataKey,
					Unique:      idx.Unique,
					Sparse:      idx.Sparse,
				})
			}
		}
	}

	return metadataIndexesResponse, nil
}
