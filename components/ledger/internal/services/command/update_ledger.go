// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdateLedgerByID updates a ledger from the repository.
func (uc *UseCase) UpdateLedgerByID(ctx context.Context, organizationID, id uuid.UUID, uli *mmodel.UpdateLedgerInput) (_ *mmodel.Ledger, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_ledger_by_id")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_ledger", start, err)
	}()

	ledger := &mmodel.Ledger{
		Name:   uli.Name,
		Status: uli.Status,
	}

	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, organizationID, id, ledger)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, constant.EntityLedger)

			logger.Log(ctx, libLog.LevelWarn, "Ledger ID not found", libLog.String("ledger_id", id.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update ledger on repo by id", err)

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to update ledger", libLog.Err(err))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update ledger on repo by id", err)

		return nil, err
	}

	uc.emitLedgerUpdatedEvent(ctx, span, logger, ledgerUpdated)

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntityLedger, id.String(), uli.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to update ledger metadata", libLog.Err(err))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo", err)

		return nil, err
	}

	ledgerUpdated.Metadata = metadataUpdated

	return ledgerUpdated, nil
}

// emitLedgerUpdatedEvent publishes the ledger.updated event for a
// successfully persisted update. IMPORTANT posture: build and emit failures
// are span-recorded and logged at Warn, never returned.
func (uc *UseCase) emitLedgerUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, led *mmodel.Ledger) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.LedgerUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewLedgerUpdated(led).ToEmitRequest(tenantID, led.UpdatedAt)
		})
}
