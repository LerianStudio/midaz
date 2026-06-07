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

// DeleteSegmentByID deletes a segment from the repository by IDs.
func (uc *UseCase) DeleteSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_segment_by_id")
	defer span.End()

	if err := uc.SegmentRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, constant.EntitySegment)

			logger.Log(ctx, libLog.LevelWarn, "Segment ID not found", libLog.Err(err), libLog.String("segment_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete segment on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete segment on repo by id", err)

		logger.Log(ctx, libLog.LevelError, "Failed to delete segment", libLog.Err(err), libLog.String("segment_id", id.String()))

		return err
	}

	uc.emitSegmentDeletedEvent(ctx, span, logger, id.String(), organizationID.String(), ledgerID.String(), time.Now())

	return nil
}

// emitSegmentDeletedEvent publishes the segment.deleted event for a
// successfully soft-deleted segment. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after SegmentRepo.Delete succeeds.
// SegmentRepo.Delete does not return the post-delete record, so the
// payload sources identity from the use-case parameters (which match
// the request path) and stamps deletedAt with the wall-clock instant
// captured by the caller. The PG deleted_at column is set by the same
// wall clock at row-update time, so the values are effectively identical
// up to clock skew.
//
// Wire-format mapping lives in pkg/streaming/events/segment_deleted.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitSegmentDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, organizationID, ledgerID string, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.SegmentDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewSegmentDeleted(id, organizationID, ledgerID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
