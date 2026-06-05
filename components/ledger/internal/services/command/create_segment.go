// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// CreateSegment creates a new segment and persists it in the repository.
func (uc *UseCase) CreateSegment(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_segment")
	defer span.End()

	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	segmentID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate segment ID", err)
		logger.Log(ctx, libLog.LevelError, "Failed to generate segment ID", libLog.Err(err))

		return nil, err
	}

	now := time.Now()
	segment := &mmodel.Segment{
		ID:             segmentID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if _, err = uc.SegmentRepo.ExistsByName(ctx, organizationID, ledgerID, cpi.Name); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check segment name existence", err)
		logger.Log(ctx, libLog.LevelError, "Failed to check segment name existence", libLog.Err(err))

		return nil, err
	}

	seg, err := uc.SegmentRepo.Create(ctx, segment)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create segment", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create segment", libLog.Err(err))

		return nil, err
	}

	uc.emitSegmentCreatedEvent(ctx, span, logger, seg)

	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntitySegment, seg.ID, cpi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create segment metadata", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create segment metadata", libLog.Err(err))

		return nil, err
	}

	seg.Metadata = metadata

	return seg, nil
}

// emitSegmentCreatedEvent publishes the segment.created event for a
// successfully persisted segment. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after SegmentRepo.Create succeeds and
// before CreateOnboardingMetadata runs, so a downstream Mongo failure
// cannot mask the event.
//
// Wire-format mapping lives in pkg/streaming/events/segment_created.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitSegmentCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, s *mmodel.Segment) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.SegmentCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewSegmentCreated(s).ToEmitRequest(tenantID, s.CreatedAt)
		})
}
