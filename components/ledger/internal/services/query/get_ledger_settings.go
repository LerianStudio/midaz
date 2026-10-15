// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// SettingsCacheTTL is the TTL for cached ledger settings (5 minutes).
const SettingsCacheTTL = 5 * time.Minute

// GetLedgerSettings retrieves the raw settings map for a specific ledger.
// The returned map reflects exactly what is stored in the database -- no default
// injection. Callers should use ParseLedgerSettings to convert to the typed
// LedgerSettings struct, which applies defaults for any missing fields.
//
// Uses a cache-aside pattern: checks Redis first, falls back to the database on
// miss, and populates the cache after a successful DB read. Cache errors are
// non-blocking -- only the DB fetch is required to succeed.
func (uc *UseCase) GetLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_settings")
	defer span.End()

	cacheKey := utils.LedgerSettingsInternalKey(organizationID, ledgerID)

	// Cache read (best-effort)
	if settings, ok := uc.readSettingsFromCache(ctx, span, logger, cacheKey, ledgerID); ok {
		return settings, nil
	}

	// DB fallback
	settings, err := uc.LedgerRepo.GetSettings(ctx, organizationID, ledgerID)
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get ledger settings", err)

		return nil, err
	}

	// Cache write (best-effort)
	uc.writeSettingsToCache(ctx, span, logger, cacheKey, settings, ledgerID)

	return settings, nil
}

// readSettingsFromCache attempts to read ledger settings from Redis.
// Returns (settings, true) on a cache hit, or (nil, false) on miss or error.
// Errors are logged but never propagated -- a cache failure simply means the
// caller falls through to the database. The cache read opens no span of its own:
// it is one Redis command the driver already traces, so its failure is recorded on
// the caller's query.get_ledger_settings span.
func (uc *UseCase) readSettingsFromCache(ctx context.Context, span trace.Span, logger libLog.Logger, cacheKey string, ledgerID uuid.UUID) (map[string]any, bool) {
	if uc.OnboardingRedisRepo == nil {
		return nil, false
	}

	cached, err := uc.OnboardingRedisRepo.Get(ctx, cacheKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Cache read error", err)
		logger.Log(ctx, libLog.LevelWarn, "Cache read error, falling back to database", libLog.Err(err))

		return nil, false
	}

	if cached == "" {
		return nil, false
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(cached), &settings); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal cached settings", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to unmarshal cached settings, falling back to database", libLog.Err(err))

		return nil, false
	}

	logger.Log(ctx, libLog.LevelDebug, "Cache hit for ledger settings", libLog.String("ledgerId", ledgerID.String()))

	return settings, true
}

// writeSettingsToCache stores ledger settings in Redis for future reads.
// Errors are logged but never propagated -- a cache write failure does not
// affect the transaction flow. Like the read, the write opens no span of its own
// and records its failure on the caller's query.get_ledger_settings span.
func (uc *UseCase) writeSettingsToCache(ctx context.Context, span trace.Span, logger libLog.Logger, cacheKey string, settings map[string]any, ledgerID uuid.UUID) {
	if uc.OnboardingRedisRepo == nil {
		return
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal settings for cache", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to marshal settings for cache", libLog.Err(err))

		return
	}

	if err := uc.OnboardingRedisRepo.Set(ctx, cacheKey, string(settingsJSON), SettingsCacheTTL); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to cache ledger settings", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to cache ledger settings", libLog.Err(err))

		return
	}

	logger.Log(ctx, libLog.LevelDebug, "Cached ledger settings", libLog.String("ledgerId", ledgerID.String()))
}
