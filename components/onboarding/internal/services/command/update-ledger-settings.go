// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// UpdateLedgerSettings updates the settings for a specific ledger using schema-aware deep merge.
// 1. Validates input settings against the LedgerSettings schema (rejects unknown fields, enforces types)
// 2. Atomically fetches existing settings, deep merges, and writes back using SELECT FOR UPDATE
// 3. Invalidates the cache after successful write
// Returns the merged settings after the update.
// Returns an error if the ledger does not exist or if validation fails.
func (uc *UseCase) UpdateLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_ledger_settings")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	logger.Infof("Updating settings for ledger: %s", ledgerID.String())

	// Validate input settings against schema before any DB operations
	if err := mmodel.ValidateSettings(settings); err != nil {
		logger.Errorf("Settings validation failed: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Settings validation failed", err)

		return nil, err
	}

	// Perform atomic read-modify-write using SELECT FOR UPDATE
	// This prevents lost updates under concurrent PATCH requests
	updatedSettings, err := uc.LedgerRepo.UpdateSettingsAtomic(ctx, organizationID, ledgerID,
		func(existing map[string]any) (map[string]any, error) {
			// Deep merge validated input with existing settings
			return mmodel.DeepMergeSettings(existing, settings), nil
		})
	if err != nil {
		logger.Errorf("Error updating ledger settings atomically: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger settings", err)

		return nil, err
	}

	// Ensure we never return nil, always return empty map
	if updatedSettings == nil {
		updatedSettings = make(map[string]any)
	}

	// Invalidate cache after successful write
	if uc.RedisRepo != nil {
		cacheKey := query.BuildLedgerSettingsCacheKey(organizationID, ledgerID)
		if err := uc.RedisRepo.Del(ctx, cacheKey); err != nil {
			logger.Warnf("Failed to invalidate ledger settings cache: %v", err)
		} else {
			logger.Debugf("Invalidated cache for ledger settings: %s", ledgerID.String())
		}
	}

	logger.Infof("Successfully updated settings for ledger: %s", ledgerID.String())

	return updatedSettings, nil
}
