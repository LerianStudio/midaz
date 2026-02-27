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
// 2. Fetches existing settings from the database
// 3. Deep merges validated input with existing settings (preserves nested properties not in input)
// 4. Writes the complete merged result back to the database
// Invalidates the cache after successful write.
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

	// Validate input settings against schema
	if err := mmodel.ValidateSettings(settings); err != nil {
		logger.Errorf("Settings validation failed: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Settings validation failed", err)

		return nil, err
	}

	// Fetch existing settings from database
	existingSettings, err := uc.LedgerRepo.GetSettings(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Errorf("Error fetching existing ledger settings: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to fetch existing ledger settings", err)

		return nil, err
	}

	// Deep merge validated input with existing settings
	mergedSettings := mmodel.DeepMergeSettings(existingSettings, settings)

	// Write complete merged result to database (replace, not merge)
	updatedSettings, err := uc.LedgerRepo.ReplaceSettings(ctx, organizationID, ledgerID, mergedSettings)
	if err != nil {
		logger.Errorf("Error replacing ledger settings: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to replace ledger settings", err)

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
