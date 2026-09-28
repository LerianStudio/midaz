// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// LedgerRef identifies a ledger in its owning organization. Cross-ledger callers
// carry both IDs because different organizations are supported by the contract.
type LedgerRef struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
}

// GetCrossLedgerPolicy loads and validates the cross-ledger opt-in for every
// distinct ledger reference. A ledger must explicitly enable the feature before
// a later transaction flow may use it.
func (uc *UseCase) GetCrossLedgerPolicy(ctx context.Context, ledgerRefs []LedgerRef) (map[LedgerRef]mmodel.CrossLedgerSettings, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_cross_ledger_policy")
	defer span.End()

	span.SetAttributes(attribute.Int("app.request.ledger_count", len(ledgerRefs)))

	policies := make(map[LedgerRef]mmodel.CrossLedgerSettings, len(ledgerRefs))
	for _, ledgerRef := range ledgerRefs {
		if _, alreadyResolved := policies[ledgerRef]; alreadyResolved {
			continue
		}

		if err := ctx.Err(); err != nil {
			libOpentelemetry.HandleSpanError(span, "Context cancelled before cross-ledger policy lookup", err)

			return nil, err
		}

		settings, err := uc.GetParsedLedgerSettings(ctx, ledgerRef.OrganizationID, ledgerRef.LedgerID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings for cross-ledger policy", err)
			logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings for cross-ledger policy", libLog.String("ledgerId", ledgerRef.LedgerID.String()), libLog.Err(err))

			return nil, err
		}

		if !settings.CrossLedger.Enabled {
			err := pkg.ValidateBusinessError(constant.ErrCrossLedgerNotEnabled, constant.EntityLedger, ledgerRef.LedgerID.String())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Ledger is not enabled for cross-ledger transactions", err)
			logger.Log(ctx, libLog.LevelWarn, "Ledger is not enabled for cross-ledger transactions", libLog.String("ledgerId", ledgerRef.LedgerID.String()), libLog.Err(err))

			return nil, err
		}

		policies[ledgerRef] = settings.CrossLedger
	}

	return policies, nil
}
