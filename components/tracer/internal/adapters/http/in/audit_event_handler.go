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

// ListAuditEvents godoc
//
//	@Summary		List audit events
//	@Description	Lists audit events with filters and cursor-based pagination. SOX/GLBA compliance endpoint.
//	@ID				listAuditEvents
//	@Tags			audit
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			start_date		query		string	false	"Start date (RFC3339 format)"	Format(date-time)
//	@Param			end_date		query		string	false	"End date (RFC3339 format)"		Format(date-time)
//	@Param			event_type		query		string	false	"Filter by event type"	Enums(TRANSACTION_VALIDATED, RULE_CREATED, RULE_UPDATED, RULE_ACTIVATED, RULE_DEACTIVATED, RULE_DRAFTED, RULE_DELETED, LIMIT_CREATED, LIMIT_UPDATED, LIMIT_ACTIVATED, LIMIT_DEACTIVATED, LIMIT_DRAFTED, LIMIT_DELETED)
//	@Param			action			query		string	false	"Filter by action"	Enums(VALIDATE, CREATE, UPDATE, DELETE, ACTIVATE, DEACTIVATE, DRAFT)
//	@Param			result			query		string	false	"Filter by result"	Enums(SUCCESS, FAILED, ALLOW, DENY, REVIEW)
//	@Param			resource_type	query		string	false	"Filter by resource type"	Enums(transaction, rule, limit)
//	@Param			resource_id		query		string	false	"Filter by resource ID"
//	@Param			actor_type		query		string	false	"Filter by actor type"	Enums(user, system)
//	@Param			actor_id		query		string	false	"Filter by actor ID"
//	@Param			account_id		query		string	false	"Filter by account ID (UUID)"	Format(uuid)
//	@Param			segment_id		query		string	false	"Filter by segment ID (UUID)"	Format(uuid)
//	@Param			portfolio_id	query		string	false	"Filter by portfolio ID (UUID)"	Format(uuid)
//	@Param			transaction_type	query		string	false	"Filter by transaction type"	Enums(CARD, WIRE, PIX, CRYPTO)
//	@Param			matched_rule_id	query		string	false	"Filter by matched rule ID (UUID)"	Format(uuid)
//	@Param			limit		    query		int		false	"Max items per page (1-1000, default: 100)"	minimum(1)	maximum(1000)
//	@Param			cursor  		query		string	false	"Pagination token (empty for first page)"
//	@Param			sort_by			query		string	false	"Sort field"	Enums(created_at, event_type)
//	@Param			sort_order		query		string	false	"Sort direction"	Enums(ASC, DESC)
//	@Success		200				{object}	ListAuditEventsResponse	"Audit events listed successfully"
//	@Failure		400				{object}	api.ErrorResponse	"Invalid parameters"
//	@Failure		401				{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		500				{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/audit-events [get]
func (h *AuditEventHandler) ListAuditEvents(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.audit_event.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse query parameters into input struct
	var input ListAuditEventsInput

	if err := c.QueryParser(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse query parameters", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityAuditEvent, "filters"))
	}

	// Validate before applying defaults to ensure fail-fast behavior
	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		logger.With(
			libLog.String("operation", "handler.audit_event.list"),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to validate request parameters")

		return http.WithError(c, err)
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

		return http.WithError(c, err)
	}

	result, err := h.service.ListAuditEvents(ctx, filters)
	if err != nil {
		return handleAuditEventServiceError(c, span, err)
	}

	// Convert to response
	response := toListAuditEventsResponse(result)

	logger.With(
		libLog.String("operation", "handler.audit_event.list"),
		libLog.Int("list.count", len(response.AuditEvents)),
		libLog.Bool("list.has_more", response.HasMore),
	).Log(ctx, libLog.LevelDebug, "Audit events listed")

	return http.OK(c, response)
}

// GetAuditEvent godoc
//
//	@Summary		Get an audit event by ID
//	@Description	Retrieves a single audit event by its unique identifier. SOX/GLBA compliance endpoint.
//	@ID				getAuditEvent
//	@Tags			audit
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Event ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.AuditEvent		"Audit event retrieved successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid event ID"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Audit event not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/audit-events/{id} [get]
func (h *AuditEventHandler) GetAuditEvent(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.audit_event.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse event ID from path
	idParam := c.Params("id")

	eventID, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid event ID", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityAuditEvent, "id"))
	}

	result, err := h.service.GetAuditEvent(ctx, eventID)
	if err != nil {
		return handleAuditEventServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.audit_event.get"),
		libLog.String("audit_event.id", result.EventID.String()),
		libLog.Any("audit_event.type", result.EventType),
	).Log(ctx, libLog.LevelDebug, "Audit event retrieved")

	return http.OK(c, result)
}

// VerifyHashChain godoc
//
//	@Summary		Verify audit event hash chain integrity
//	@Description	Verifies the integrity of the audit event hash chain up to a specific event. Detects tampering attempts. SOX/GLBA compliance endpoint.
//	@ID				verifyAuditEvent
//	@Tags			audit
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Event ID to verify up to (UUID)"	Format(uuid)
//	@Success		200			{object}	model.HashChainVerificationResult	"Hash chain verification completed"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid event ID"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Event not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/audit-events/{id}/verify [get]
func (h *AuditEventHandler) VerifyHashChain(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.audit_event.verify_chain")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse event ID from path parameter
	eventIDParam := c.Params("id")

	eventID, err := uuid.Parse(eventIDParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid event ID", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityAuditEvent, "id"))
	}

	result, err := h.service.VerifyHashChain(ctx, eventID)
	if err != nil {
		return handleAuditEventServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.audit_event.verify_chain"),
		libLog.Bool("is_valid", result.IsValid),
		libLog.Any("total_checked", result.TotalChecked),
	).Log(ctx, libLog.LevelDebug, "Hash chain verification completed")

	return http.OK(c, result)
}

// handleAuditEventServiceError converts service errors to appropriate HTTP responses.
func handleAuditEventServiceError(c *fiber.Ctx, span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrAuditEventNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Audit event not found", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrAuditEventNotFound, constant.EntityAuditEvent))
	case errors.Is(err, constant.ErrInvalidAuditEventFilters):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid filters", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidAuditEventFilters, constant.EntityAuditEvent))
	case errors.Is(err, constant.ErrInvalidCursor):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidCursor, constant.EntityAuditEvent))
	case errors.Is(err, constant.ErrInvalidSortColumn):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidSortColumn, constant.EntityAuditEvent))
	default:
		libOpentelemetry.HandleSpanError(span, "Operation failed", err)
		return http.WithError(c, pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "The server encountered an unexpected error. Please try again later or contact support."})
	}
}
