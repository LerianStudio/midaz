// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAccountingSettings retrieves and parses accounting settings for a ledger.
// Returns default settings if:
//   - SettingsPort is nil (settings functionality not enabled)
//   - Settings fetch fails (graceful degradation)
//   - Settings are empty or missing accounting section
//
// This function never returns an error - it always returns valid settings.
// Errors are logged but do not propagate to callers.
func (uc *UseCase) GetAccountingSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) mmodel.AccountingSettings {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_accounting_settings")
	defer span.End()

	// If SettingsPort is not configured, return defaults (permissive)
	if uc.SettingsPort == nil {
		logger.Debugf("SettingsPort not configured, using default accounting settings for ledger: %s", ledgerID.String())

		return mmodel.DefaultAccountingSettings()
	}

	// Fetch settings from SettingsPort (which uses cache internally)
	settings, err := uc.SettingsPort.GetLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		// Log error but don't fail - use defaults for graceful degradation
		libOpentelemetry.HandleSpanError(&span, "Failed to get ledger settings, using defaults", err)

		// Error details captured in span; log only ledger ID to avoid exposing internal error messages
		logger.Warnf("Failed to get ledger settings for %s, using defaults", ledgerID.String())

		return mmodel.DefaultAccountingSettings()
	}

	// Parse settings into typed struct
	accountingSettings := mmodel.ParseAccountingSettings(settings)

	logger.Debugf("Retrieved accounting settings for ledger %s: validateAccountType=%v, validateRoutes=%v",
		ledgerID.String(), accountingSettings.ValidateAccountType, accountingSettings.ValidateRoutes)

	return accountingSettings
}
