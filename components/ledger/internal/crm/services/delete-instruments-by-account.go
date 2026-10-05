// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// DeleteInstrumentsByAccount soft-deletes every live instrument linked to the
// account in the ledger and returns how many this call deleted. Each instrument
// goes through DeleteInstrumentByID, so it gets the same event and metric as a
// manual soft delete. An instrument deleted concurrently is skipped without
// counting; any other failure aborts and propagates.
func (uc *UseCase) DeleteInstrumentsByAccount(ctx context.Context, organizationID string, ledgerID, accountID uuid.UUID) (int, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_instruments_by_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	deleted := 0

	defer func() {
		span.SetAttributes(attribute.Int("app.instruments_deleted", deleted))
	}()

	refs, err := uc.InstrumentRepo.FindLiveRefsByAccount(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		recordSpanError(span, "Failed to find live instruments by account", err)

		return deleted, err
	}

	for _, ref := range refs {
		err := uc.DeleteInstrumentByID(ctx, organizationID, ref.HolderID, ref.ID, false)
		if err == nil {
			deleted++

			logger.Log(ctx, libLog.LevelInfo, "Instrument soft-deleted by account cascade",
				libLog.String("instrument_id", ref.ID.String()),
				libLog.String("action", "cascade_soft_delete"))

			continue
		}

		if isInstrumentNotFound(err) {
			logger.Log(ctx, libLog.LevelDebug, "Instrument already deleted, skipping",
				libLog.String("instrument_id", ref.ID.String()))

			continue
		}

		recordSpanError(span, "Failed to delete instrument linked to account", err)

		return deleted, err
	}

	return deleted, nil
}

func isInstrumentNotFound(err error) bool {
	var notFound pkg.EntityNotFoundError

	return errors.As(err, &notFound) && notFound.Code == constant.ErrInstrumentNotFound.Error()
}
