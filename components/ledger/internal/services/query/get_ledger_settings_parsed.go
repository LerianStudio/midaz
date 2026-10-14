// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// GetParsedLedgerSettings retrieves and parses ledger settings for a ledger.
// Returns an error if the settings cannot be fetched -- callers must not
// proceed without knowing whether route validation or account type validation
// is enabled, as skipping those checks could allow invalid transactions.
func (uc *UseCase) GetParsedLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (mmodel.LedgerSettings, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_settings")
	defer span.End()

	settings, err := uc.GetLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		// logged once by GetLedgerSettings; record on this span only
		spanattr.HandleSpanByErrorClass(span, "Failed to get ledger settings", err)

		return mmodel.LedgerSettings{}, err
	}

	ledgerSettings := mmodel.ParseLedgerSettings(settings)

	logger.Log(ctx, libLog.LevelDebug, "Retrieved ledger settings",
		libLog.String("ledgerId", ledgerID.String()),
		libLog.Bool("validateAccountType", ledgerSettings.Accounting.ValidateAccountType),
		libLog.Bool("validateRoutes", ledgerSettings.Accounting.ValidateRoutes))

	return ledgerSettings, nil
}
