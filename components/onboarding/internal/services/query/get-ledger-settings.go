// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
)

// GetLedgerSettings retrieves the settings for a specific ledger.
// Returns an empty map if no settings are defined (not an error).
// Returns an error if the ledger does not exist.
func (uc *UseCase) GetLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_settings")
	defer span.End()

	logger.Infof("Retrieving settings for ledger: %s", ledgerID.String())

	settings, err := uc.LedgerRepo.GetSettings(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Errorf("Error getting ledger settings: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get ledger settings", err)

		return nil, err
	}

	// Ensure we never return nil, always return empty map
	if settings == nil {
		settings = make(map[string]any)
	}

	logger.Infof("Successfully retrieved settings for ledger: %s", ledgerID.String())

	return settings, nil
}
