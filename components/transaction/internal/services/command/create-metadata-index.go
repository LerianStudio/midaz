package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// CreateMetadataIndex creates a new metadata index.
func (uc *UseCase) CreateMetadataIndex(ctx context.Context, entityName string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_metadata_index")
	defer span.End()

	logger.Infof("Initializing the create metadata index operation: entityName=%s, input=%v", entityName, input)

	existingIndexes, err := uc.MetadataRepo.FindAllIndexes(ctx, entityName)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check existing indexes", err)

		logger.Errorf("Failed to check existing indexes: %v", err)

		return nil, err
	}

	expectedIndexKey := fmt.Sprintf("metadata.%s", input.MetadataKey)
	for _, idx := range existingIndexes {
		if idx.MetadataKey == expectedIndexKey {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Metadata index already exists", nil)

			logger.Errorf("Metadata index already exists for key: %s", input.MetadataKey)

			return nil, pkg.ValidateBusinessError(constant.ErrMetadataIndexAlreadyExists, "MetadataIndex", strings.ToLower(input.MetadataKey))
		}
	}

	sparse := true
	if input.Sparse != nil {
		sparse = *input.Sparse
	}

	metadataIndex, err := uc.MetadataRepo.CreateIndex(ctx, entityName, &mongodb.MetadataIndex{
		EntityName:  entityName,
		MetadataKey: input.MetadataKey,
		Unique:      input.Unique,
		Sparse:      sparse,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create metadata index", err)

		logger.Errorf("Failed to create metadata index: %v", err)

		return nil, err
	}

	result := &mmodel.MetadataIndex{
		IndexName:   fmt.Sprintf("metadata.%s_1", metadataIndex.MetadataKey),
		EntityName:  metadataIndex.EntityName,
		MetadataKey: metadataIndex.MetadataKey,
		Unique:      metadataIndex.Unique,
		Sparse:      metadataIndex.Sparse,
		CreatedAt:   time.Now(),
	}

	return result, nil
}
