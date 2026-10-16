// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// GetParsedLedgerSettings retrieves and parses ledger settings for a ledger.
// Returns an error if the settings cannot be fetched -- callers must not
// proceed without knowing whether route validation or account type validation
// is enabled, as skipping those checks could allow invalid transactions.
func (uc *UseCase) GetParsedLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (mmodel.LedgerSettings, error) {
	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)

	// No span of its own: GetLedgerSettings opens the single query.get_ledger_settings
	// span per read and already logs and records this error there, so a wrapper span
	// would only nest the same name inside itself.
	settings, err := uc.GetLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		return mmodel.LedgerSettings{}, err
	}

	ledgerSettings := mmodel.ParseLedgerSettings(settings)

	logger.Log(ctx, libLog.LevelDebug, "Retrieved ledger settings",
		libLog.String("ledgerId", ledgerID.String()),
		libLog.Bool("validateAccountType", ledgerSettings.Accounting.ValidateAccountType),
		libLog.Bool("validateRoutes", ledgerSettings.Accounting.ValidateRoutes))

	return ledgerSettings, nil
}
