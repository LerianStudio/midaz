// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
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

	// FindAllIndexes returns MetadataKey without the "metadata." prefix
	for _, idx := range existingIndexes {
		if idx.MetadataKey == input.MetadataKey {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Metadata index already exists", nil)

			logger.Errorf("Metadata index already exists for key: %s", input.MetadataKey)

			return nil, pkg.ValidateBusinessError(constant.ErrMetadataIndexAlreadyExists, "MetadataIndex", strings.ToLower(input.MetadataKey))
		}
	}

	metadataIndex, err := uc.MetadataRepo.CreateIndex(ctx, entityName, input)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create metadata index", err)

		logger.Errorf("Failed to create metadata index: %v", err)

		return nil, err
	}

	// Set the entity name since the repo returns with collection name
	metadataIndex.EntityName = entityName

	return metadataIndex, nil
}
