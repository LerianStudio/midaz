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
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
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

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Updating settings for ledger: %s", ledgerID.String()))

	// Validate input settings against schema before any DB operations
	if err := mmodel.ValidateSettings(settings); err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Settings validation failed: %v", err))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Settings validation failed", err)

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
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating ledger settings atomically: %v", err))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update ledger settings", err)

		return nil, err
	}

	// Ensure we never return nil, always return empty map
	if updatedSettings == nil {
		updatedSettings = make(map[string]any)
	}

	// Invalidate cache after successful write
	uc.invalidateSettingsCache(ctx, organizationID, ledgerID)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully updated settings for ledger: %s", ledgerID.String()))

	return updatedSettings, nil
}

// invalidateSettingsCache removes the cached ledger settings so the next read fetches from the database.
// RedisRepo is a required dependency at runtime; the nil guard exists solely to uphold this function's
// resilience contract: cache issues must never fail a successful write operation.
func (uc *UseCase) invalidateSettingsCache(ctx context.Context, organizationID, ledgerID uuid.UUID) {
	if uc.OnboardingRedisRepo == nil {
		return
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.invalidate_settings_cache")
	defer span.End()

	cacheKey := utils.LedgerSettingsInternalKey(organizationID, ledgerID)
	if err := uc.OnboardingRedisRepo.Del(ctx, cacheKey); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to invalidate ledger settings cache", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to invalidate ledger settings cache: %v", err))
	}
}
