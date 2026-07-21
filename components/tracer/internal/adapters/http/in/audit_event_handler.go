// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=audit_event_handler.go -destination=audit_event_service_mock.go -package=in

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// AuditEventService defines the interface for audit event query operations.
// Interface defined locally per Ring pattern.
type AuditEventService interface {
	GetAuditEvent(ctx context.Context, eventID uuid.UUID) (*model.AuditEvent, error)
	ListAuditEvents(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error)
	VerifyHashChain(ctx context.Context, eventID uuid.UUID) (*model.HashChainVerificationResult, error)
}

// AuditEventHandler handles HTTP requests for audit event operations.
type AuditEventHandler struct {
	service AuditEventService
}

// NewAuditEventHandler creates a new audit event handler.
func NewAuditEventHandler(service AuditEventService) *AuditEventHandler {
	return &AuditEventHandler{
		service: service,
	}
}

func (h *AuditEventHandler) ListAuditEvents(c *fiber.Ctx) error {
	// Fiber binds the query with QueryParser; the shared core owns the rest.
	response, err := h.listAuditEvents(c.UserContext(), c.QueryParser)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, response)
}

// listAuditEvents is the transport-agnostic core of the list-audit-events
// operation shared by the Fiber method (ListAuditEvents) and the Huma func
// (ListAuditEventsHuma). See rule_handler.go:listRules for the split rationale.
//
// Query BINDING is the only transport-specific step and stays at the edge: the
// caller passes a bind func that populates *ListAuditEventsInput from its own
// query source (Fiber's c.QueryParser, or the Huma func's imperative
// string->typed copy). A bind error is canonicalized to ErrInvalidQueryParameter
// (0082) — the SAME code the Fiber QueryParser-failure path produced. Everything
// after binding (Validate -> SetDefaults -> filters -> service -> response) is
// shared, and every error is canonicalized before it returns, so both transports
// emit field/status/code-identical results.
func (h *AuditEventHandler) listAuditEvents(ctx context.Context, bind func(any) error) (*ListAuditEventsResponse, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.audit_event.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	var input ListAuditEventsInput
	if err := bind(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse query parameters", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityAuditEvent, "filters")
	}

	// Validate before applying defaults to ensure fail-fast behavior
	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		logger.With(
			libLog.String("operation", "handler.audit_event.list"),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to validate request parameters")

		return nil, err
	}

	// Apply defaults after validation
	input.SetDefaults()

	// Convert to service filters
	filters, err := toAuditEventFilters(&input)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid filters", err)
		logger.With(
			libLog.String("operation", "handler.audit_event.list"),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to convert filter parameters")

		return nil, err
	}

	result, err := h.service.ListAuditEvents(ctx, filters)
	if err != nil {
		return nil, classifyAuditEventError(span, err)
	}

	// Convert to response
	response := toListAuditEventsResponse(result)

	logger.With(
		libLog.String("operation", "handler.audit_event.list"),
		libLog.Int("list.count", len(response.AuditEvents)),
		libLog.Bool("list.has_more", response.HasMore),
	).Log(ctx, libLog.LevelDebug, "Audit events listed")

	return response, nil
}

func (h *AuditEventHandler) GetAuditEvent(c *fiber.Ctx) error {
	result, err := h.getAuditEvent(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// getAuditEvent is the transport-agnostic core of the get-audit-event operation
// shared by the Fiber method (GetAuditEvent) and the Huma func (GetAuditEventHuma).
// It owns the span + id parse + service call + success log, and canonicalizes
// every error before returning so both transports render the identical body.
func (h *AuditEventHandler) getAuditEvent(ctx context.Context, idParam string) (*model.AuditEvent, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.audit_event.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	eventID, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid event ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityAuditEvent, "id")
	}

	result, err := h.service.GetAuditEvent(ctx, eventID)
	if err != nil {
		return nil, classifyAuditEventError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.audit_event.get"),
		libLog.String("audit_event.id", result.EventID.String()),
		libLog.Any("audit_event.type", result.EventType),
	).Log(ctx, libLog.LevelDebug, "Audit event retrieved")

	return result, nil
}

func (h *AuditEventHandler) VerifyHashChain(c *fiber.Ctx) error {
	result, err := h.verifyHashChain(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// verifyHashChain is the transport-agnostic core of the verify-hash-chain
// operation shared by the Fiber method (VerifyHashChain) and the Huma func
// (VerifyHashChainHuma). It owns the span + id parse + service call + success
// log, and canonicalizes every error before returning.
func (h *AuditEventHandler) verifyHashChain(ctx context.Context, idParam string) (*model.HashChainVerificationResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.audit_event.verify_chain")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	eventID, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid event ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityAuditEvent, "id")
	}

	result, err := h.service.VerifyHashChain(ctx, eventID)
	if err != nil {
		return nil, classifyAuditEventError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.audit_event.verify_chain"),
		libLog.Bool("is_valid", result.IsValid),
		libLog.Any("total_checked", result.TotalChecked),
	).Log(ctx, libLog.LevelDebug, "Hash chain verification completed")

	return result, nil
}

// classifyAuditEventError maps a raw service error to its canonical Midaz error,
// attributing the span, WITHOUT rendering. It is the single classification the
// Fiber wrappers (which render via http.WithError) and the Huma path
// (humaProblem -> *http.Detail) both consume, so both transports emit
// field/status/code/type-identical envelopes. The errors.Is cascade and the
// canonical mapping are preserved verbatim from the pre-Huma handler
// (handleAuditEventServiceError), minus the render.
func classifyAuditEventError(span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrAuditEventNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Audit event not found", err)
		return pkg.ValidateBusinessError(constant.ErrAuditEventNotFound, constant.EntityAuditEvent)
	case errors.Is(err, constant.ErrInvalidAuditEventFilters):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid filters", err)

		return pkg.ValidateBusinessError(constant.ErrInvalidAuditEventFilters, constant.EntityAuditEvent)
	case errors.Is(err, constant.ErrInvalidCursor):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)

		return pkg.ValidateBusinessError(constant.ErrInvalidCursor, constant.EntityAuditEvent)
	case errors.Is(err, constant.ErrInvalidSortColumn):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)

		return pkg.ValidateBusinessError(constant.ErrInvalidSortColumn, constant.EntityAuditEvent)
	default:
		libOpentelemetry.HandleSpanError(span, "Operation failed", err)
		return pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "The server encountered an unexpected error. Please try again later or contact support."}
	}
}
