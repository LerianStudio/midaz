// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// DefaultSettingsCacheTTL is the default TTL for cached ledger settings (5 minutes).
// Can be overridden via UseCase.SettingsCacheTTL or SETTINGS_CACHE_TTL env var.
const DefaultSettingsCacheTTL = 5 * time.Minute

// LedgerSettingsCacheKeyPrefix is the prefix for ledger settings cache keys.
// Full key format: ledger_settings:{organizationID}:{ledgerID}
// Organization ID is required to ensure proper tenant isolation in multi-tenant deployments.
const LedgerSettingsCacheKeyPrefix = "ledger_settings"

// getSettingsCacheTTL returns the configured cache TTL or the default if not set.
func (uc *UseCase) getSettingsCacheTTL() time.Duration {
	if uc.SettingsCacheTTL > 0 {
		return uc.SettingsCacheTTL
	}

	return DefaultSettingsCacheTTL
}

// BuildLedgerSettingsCacheKey builds the cache key for ledger settings.
func BuildLedgerSettingsCacheKey(organizationID, ledgerID uuid.UUID) string {
	return fmt.Sprintf("%s:%s:%s", LedgerSettingsCacheKeyPrefix, organizationID.String(), ledgerID.String())
}

// GetLedgerSettings retrieves the settings for a specific ledger.
// Uses cache-aside pattern: checks cache first, falls back to database on miss.
// Returns an empty map if no settings are defined (not an error).
// Returns an error if the ledger does not exist.
func (uc *UseCase) GetLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_settings")
	defer span.End()

	logger.Debugf("Retrieving settings for ledger: %s", ledgerID.String())

	cacheKey := BuildLedgerSettingsCacheKey(organizationID, ledgerID)

	// Try to get from cache first
	if uc.RedisRepo != nil {
		cached, err := uc.RedisRepo.Get(ctx, cacheKey)
		if err != nil {
			logger.Warnf("Cache error, falling back to database: %v", err)
		} else if cached != "" {
			// Cache hit - unmarshal and return
			var settings map[string]any
			if err := json.Unmarshal([]byte(cached), &settings); err != nil {
				logger.Warnf("Failed to unmarshal cached settings, falling back to database: %v", err)
			} else {
				logger.Debugf("Cache hit for ledger settings: %s", ledgerID.String())

				// Merge with defaults to ensure complete settings object
				mergedSettings := mmodel.MergeSettingsWithDefaults(settings)

				return mergedSettings, nil
			}
		}
	}

	// Cache miss or error - get from database
	settings, err := uc.LedgerRepo.GetSettings(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Errorf("Error getting ledger settings: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get ledger settings", err)

		return nil, err
	}

	// Merge with defaults to ensure complete settings object
	settings = mmodel.MergeSettingsWithDefaults(settings)

	// Populate cache for future reads
	if uc.RedisRepo != nil {
		cacheCtx, cacheSpan := tracer.Start(ctx, "cache.set_ledger_settings")
		defer cacheSpan.End()

		settingsJSON, err := json.Marshal(settings)
		if err != nil {
			logger.Warnf("Failed to marshal settings for cache: %v", err)
		} else {
			if err := uc.RedisRepo.Set(cacheCtx, cacheKey, string(settingsJSON), uc.getSettingsCacheTTL()); err != nil {
				libOpentelemetry.HandleSpanError(&cacheSpan, "Failed to cache ledger settings", err)

				logger.Warnf("Failed to cache ledger settings: %v", err)
			} else {
				logger.Debugf("Cached ledger settings: %s", ledgerID.String())
			}
		}
	}

	logger.Debugf("Successfully retrieved settings for ledger: %s", ledgerID.String())

	return settings, nil
}
