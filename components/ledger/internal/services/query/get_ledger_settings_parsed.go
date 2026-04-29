// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetParsedLedgerSettings retrieves and parses ledger settings for a ledger.
// Returns an error if the settings cannot be fetched -- callers must not
// proceed without knowing whether route validation or account type validation
// is enabled, as skipping those checks could allow invalid transactions.
func (uc *UseCase) GetParsedLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (mmodel.LedgerSettings, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_settings")
	defer span.End()

	settings, err := uc.GetLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.String("ledgerId", ledgerID.String()), libLog.Err(err))

		return mmodel.LedgerSettings{}, err
	}

	ledgerSettings := mmodel.ParseLedgerSettings(settings)

	logger.Log(ctx, libLog.LevelDebug, "Retrieved ledger settings",
		libLog.String("ledgerId", ledgerID.String()),
		libLog.Bool("validateAccountType", ledgerSettings.Accounting.ValidateAccountType),
		libLog.Bool("validateRoutes", ledgerSettings.Accounting.ValidateRoutes))

	return ledgerSettings, nil
}
