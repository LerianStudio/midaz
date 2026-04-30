// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
)

func (uc *UseCase) UpdateOnboardingMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_metadata")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to update metadata for %s", entityName))

	metadataToUpdate := metadata

	if metadataToUpdate != nil {
		existingMetadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, entityName, entityID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb", err)

			logger.Log(ctx, libLog.LevelError, "Error getting metadata on mongodb")

			return nil, err
		}

		if existingMetadata != nil {
			metadataToUpdate = libCommons.MergeMaps(metadata, existingMetadata.Data)
		}
	} else {
		metadataToUpdate = map[string]any{}
	}

	if err := uc.OnboardingMetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on mongodb", err)

		return nil, err
	}

	return metadataToUpdate, nil
}
