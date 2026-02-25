// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// UpdateLedgerSettings updates the settings for a specific ledger using JSONB merge semantics.
// The new settings are merged with existing settings (not replaced).
// Invalidates the cache after successful write.
// Returns the merged settings after the update.
// Returns an error if the ledger does not exist.
func (uc *UseCase) UpdateLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_ledger_settings")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	logger.Infof("Updating settings for ledger: %s", ledgerID.String())

	updatedSettings, err := uc.LedgerRepo.UpdateSettings(ctx, organizationID, ledgerID, settings)
	if err != nil {
		logger.Errorf("Error updating ledger settings: %v", err)

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
