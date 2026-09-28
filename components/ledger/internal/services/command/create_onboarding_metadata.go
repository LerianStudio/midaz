// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
)

// CreateOnboardingMetadata persists the given metadata in MongoDB for the specified entity.
// If metadata is nil, no document is created and (nil, nil) is returned.
func (uc *UseCase) CreateOnboardingMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_metadata")
	defer span.End()

	if metadata == nil {
		return nil, nil
	}

	now := time.Now()

	meta := mongodb.Metadata{
		EntityID:   entityID,
		EntityName: entityName,
		Data:       metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := uc.OnboardingMetadataRepo.Create(ctx, entityName, &meta); err != nil {
		recordCommandError(ctx, span, logger, "Failed to create metadata", err)

		return nil, err
	}

	return metadata, nil
}
