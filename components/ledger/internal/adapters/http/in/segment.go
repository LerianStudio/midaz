// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// SegmentHandler struct contains a segment use case for managing segment related operations.
type SegmentHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createSegment/updateSegment/... methods below own the span, the service call
// and the success log. They take primitive args (parsed UUIDs, already-decoded
// payload, the query map) so BOTH transports feed them: the Fiber wrappers pull
// those from *fiber.Ctx (Locals + the WithBody-decoded payload + c.Queries) and the
// Huma handlers (segment_handler_huma.go) pull them from the request envelope. Every
// canonical error the cores return is rendered by the caller — http.WithError on the
// Fiber path, http.HumaProblem on the Huma path — so code + HTTP status are identical
// across transports. Body decode+validation happens BEFORE these cores (Fiber via the
// WithBody decorator, Huma via http.DecodeAndValidate), both feeding the SAME
// validated payload here. Mirrors the asset exemplar (asset.go).

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

// getAllSegments binds the query map imperatively (http.ValidateParameters — the SAME
// binder the Fiber path used) so a bad query yields the canonical 400, then returns
// the assembled pagination envelope.
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

// --- Fiber wrappers (thin) ----------------------------------------------------
//
// These stay so the legacy Fiber unit/integration tests keep exercising the handler
// methods directly; each pulls the transport inputs from *fiber.Ctx (Locals set by
// ParseUUIDPathParameters, the WithBody-decoded payload as `i`) and delegates to the
// shared core. NOTE: the LIVE segment routes become Huma via segment_handler_huma.go +
// RegisterSegmentRoutesToApp; these Fiber wrappers keep the inline routes compiling
// until integration wires the mount.

// CreateSegment is a method that creates segment information.
func (handler *SegmentHandler) CreateSegment(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	segment, err := handler.createSegment(ctx, organizationID, ledgerID, i.(*mmodel.CreateSegmentInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, segment)
}

// GetAllSegments is a method that retrieves all Segments.
func (handler *SegmentHandler) GetAllSegments(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllSegments(ctx, organizationID, ledgerID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}

// GetSegmentByID is a method that retrieves Segment information by a given id.
func (handler *SegmentHandler) GetSegmentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	segment, err := handler.getSegmentByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, segment)
}

// UpdateSegment is a method that updates Segment information.
func (handler *SegmentHandler) UpdateSegment(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	segment, err := handler.updateSegment(ctx, organizationID, ledgerID, id, i.(*mmodel.UpdateSegmentInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, segment)
}

// DeleteSegmentByID is a method that removes Segment information by a given ids.
func (handler *SegmentHandler) DeleteSegmentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	if err := handler.deleteSegment(ctx, organizationID, ledgerID, id); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// CountSegments is a method that counts all segments for a given organization and ledger.
func (handler *SegmentHandler) CountSegments(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	count, err := handler.countSegments(ctx, organizationID, ledgerID)
	if err != nil {
		return http.WithError(c, err)
	}

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
