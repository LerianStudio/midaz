// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

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
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdateSegmentByID updates a segment from the repository by the given ID.
func (uc *UseCase) UpdateSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_segment_by_id")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if upi.Name != "" {
		segmentFound, err := uc.SegmentRepo.Find(ctx, organizationID, ledgerID, id)
		if err != nil {
			if errors.Is(err, services.ErrDatabaseItemNotFound) {
				err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, constant.EntitySegment)

				logger.Log(ctx, libLog.LevelWarn, "Segment not found", libLog.Err(err), libLog.String("segment_id", id.String()))
			} else {
				logger.Log(ctx, libLog.LevelError, "Failed to find segment", libLog.Err(err), libLog.String("segment_id", id.String()))
			}

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find segment on repo by id", err)

			return nil, err
		}

		if segmentFound != nil && segmentFound.Name != upi.Name {
			if _, err := uc.SegmentRepo.ExistsByName(ctx, organizationID, ledgerID, upi.Name); err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check segment name existence", err)
				logger.Log(ctx, libLog.LevelWarn, "Segment name is not available", libLog.Err(err), libLog.String("segment_id", id.String()))

				return nil, err
			}
		}
	}

	segment := &mmodel.Segment{
		Name:   upi.Name,
		Status: upi.Status,
	}

	segmentUpdated, err := uc.SegmentRepo.Update(ctx, organizationID, ledgerID, id, segment)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, constant.EntitySegment)
			logger.Log(ctx, libLog.LevelWarn, "Segment not found", libLog.Err(err), libLog.String("segment_id", id.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update segment on repo by id", err)

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to update segment", libLog.Err(err), libLog.String("segment_id", id.String()))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update segment on repo by id", err)

		return nil, err
	}

	uc.emitSegmentUpdatedEvent(ctx, span, logger, segmentUpdated)

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntitySegment, id.String(), upi.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to update segment metadata", libLog.Err(err), libLog.String("segment_id", id.String()))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	segmentUpdated.Metadata = metadataUpdated

	return segmentUpdated, nil
}

// emitSegmentUpdatedEvent publishes the segment.updated event for a
// successfully persisted update. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked between the SegmentRepo.Update success branch and the
// metadata-write call in UpdateSegmentByID, so a downstream Mongo
// failure cannot mask the event.
//
// Caller invariant: s must be the value returned by SegmentRepo.Update
// (post-commit), not the input struct. Specifically s.ID, s.UpdatedAt
// and the persisted Name/Status must reflect the row state.
//
// Wire-format mapping lives in pkg/streaming/events/segment_updated.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitSegmentUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, s *mmodel.Segment) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.SegmentUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewSegmentUpdated(s).ToEmitRequest(tenantID, s.UpdatedAt)
		})
}
