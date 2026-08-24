// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// SegmentHandler struct contains a segment use case for managing segment related operations.
type SegmentHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createSegment/updateSegment/... methods below own the span, the service call
// and the success log. They take primitive args — parsed UUIDs, already-decoded
// payload, the query map — so nothing transport-shaped reaches them; the handlers in
// segment_handler.go pull those out of the request envelope. Every canonical error a
// core returns is rendered by its caller via http.HumaProblem, which fixes the code +
// HTTP status. Body decode+validation happens BEFORE these cores, in the handler, via
// http.DecodeAndValidate. Mirrors the asset exemplar (asset_core.go).

// createSegment owns the span + service call + success log for an already-decoded payload.
func (handler *SegmentHandler) createSegment(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_segment")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create a segment", payload)
	recordSafePayloadAttributes(span, payload)

	segment, err := handler.Command.CreateSegment(ctx, organizationID, ledgerID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create Segment on command", err)

		return nil, err
	}

	return segment, nil
}

// getAllSegments binds the query map imperatively via http.ValidateParameters so a
// bad query yields the canonical 400, then returns the assembled pagination
// envelope.
func (handler *SegmentHandler) getAllSegments(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_segments")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		segments, err := handler.Query.GetAllMetadataSegments(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve all Segments on query", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(segments)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	segments, err := handler.Query.GetAllSegments(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all Segments on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(segments)

	return pagination, nil
}

// getSegmentByID retrieves a single segment.
func (handler *SegmentHandler) getSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Segment, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_segment_by_id")
	defer span.End()

	segment, err := handler.Query.GetSegmentByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Segment on query", err)

		return nil, err
	}

	return segment, nil
}

// updateSegment owns the span + service call + success log for an already-decoded payload.
func (handler *SegmentHandler) updateSegment(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_segment")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update segment", payload)
	recordSafePayloadAttributes(span, payload)

	segment, err := handler.Command.UpdateSegmentByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update Segment on command", err)

		return nil, err
	}

	return segment, nil
}

// deleteSegment removes a segment.
func (handler *SegmentHandler) deleteSegment(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_segment_by_id")
	defer span.End()

	if err := handler.Command.DeleteSegmentByID(ctx, organizationID, ledgerID, id); err != nil {
		handleSpanByErrorClass(span, "Failed to remove Segment on command", err)

		return err
	}

	return nil
}

// countSegments returns the total segment count for the ledger.
func (handler *SegmentHandler) countSegments(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_segments")
	defer span.End()

	count, err := handler.Query.CountSegments(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count segments", err)

		return 0, err
	}

	return count, nil
}
