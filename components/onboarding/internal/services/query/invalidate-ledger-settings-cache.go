// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/google/uuid"
)

// InvalidateLedgerSettingsCache removes the cached settings for a ledger so the next read fetches from the database.
// No-op when RedisRepo is nil. Cache failures are logged but not returned so callers are not failed by cache issues.
// Call this after any write path that changes ledger settings (e.g. UpdateLedgerSettings, ReplaceSettings).
func (uc *UseCase) InvalidateLedgerSettingsCache(ctx context.Context, organizationID, ledgerID uuid.UUID) {
	if uc.RedisRepo == nil {
		return
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.invalidate_ledger_settings_cache")
	defer span.End()

	cacheKey := BuildLedgerSettingsCacheKey(organizationID, ledgerID)
	if err := uc.RedisRepo.Del(ctx, cacheKey); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to invalidate ledger settings cache", err)

		logger.Errorf("Failed to invalidate ledger settings cache: %v", err)
	} else {
		logger.Debugf("Invalidated cache for ledger settings: %s", ledgerID.String())
	}
}
