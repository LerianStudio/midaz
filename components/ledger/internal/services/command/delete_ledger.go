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
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// DeleteLedgerByID deletes a ledger from the repository.
func (uc *UseCase) DeleteLedgerByID(ctx context.Context, organizationID, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_ledger_by_id")
	defer span.End()

	if err := uc.LedgerRepo.Delete(ctx, organizationID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, constant.EntityLedger)

			logger.Log(ctx, libLog.LevelWarn, "Ledger ID not found", libLog.String("ledger_id", id.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete ledger on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete ledger on repo by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to delete ledger", libLog.Err(err))

		return err
	}

	deletedAt := time.Now()
	uc.emitLedgerDeletedEvent(ctx, span, logger, id.String(), organizationID.String(), deletedAt)

	return nil
}

// emitLedgerDeletedEvent publishes the ledger.deleted event for a
// successfully soft-deleted ledger. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
func (uc *UseCase) emitLedgerDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, organizationID string, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.LedgerDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewLedgerDeleted(id, organizationID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
