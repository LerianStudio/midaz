// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"

	// CreateMetadataIndex creates a new metadata index.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) CreateMetadataIndex(ctx context.Context, entityName string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_metadata_index")
	defer span.End()

	existingIndexes, err := uc.TransactionMetadataRepo.FindAllIndexes(ctx, entityName)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check existing indexes", err)

		logger.Log(ctx, libLog.LevelError, "Failed to check existing indexes", libLog.Err(err))

		return nil, err
	}

	// FindAllIndexes returns MetadataKey without the "metadata." prefix
	for _, idx := range existingIndexes {
		if idx.MetadataKey == input.MetadataKey {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Metadata index already exists", nil)

			logger.Log(ctx, libLog.LevelWarn, "Metadata index already exists", libLog.String("metadata_key", input.MetadataKey))

			return nil, pkg.ValidateBusinessError(constant.ErrMetadataIndexAlreadyExists, "MetadataIndex", strings.ToLower(input.MetadataKey))
		}
	}

	metadataIndex, err := uc.TransactionMetadataRepo.CreateIndex(ctx, entityName, input)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create metadata index", err)

		logger.Log(ctx, libLog.LevelError, "Failed to create metadata index", libLog.Err(err))

		return nil, err
	}

	// Set the entity name since the repo returns with collection name
	metadataIndex.EntityName = entityName

	return metadataIndex, nil
}
