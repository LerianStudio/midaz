// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// DeleteLedgerByID deletes a ledger from the repository.
func (uc *UseCase) DeleteLedgerByID(ctx context.Context, organizationID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_ledger_by_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Remove ledger for id: %s", id.String()))

	if err := uc.LedgerRepo.Delete(ctx, organizationID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Ledger ID not found: %s", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete ledger on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete ledger on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error deleting ledger: %v", err))

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
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, uc.StreamingSource, events.LedgerDeletedDefinition.Key(),
		func(tenantID, source string) (libStreaming.Event, error) {
			return events.NewLedgerDeleted(id, organizationID, deletedAt).ToEvent(tenantID, source, deletedAt)
		})
}
